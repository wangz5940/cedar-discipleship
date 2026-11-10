package ovcm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

const testData = `const c={courseData:[{id:"old",title:"旧课程",coverImage:"cover/old.svg",lessons:[{id:"old-01",title:"第一课",audioUrl:"audio/old/old-01.mp3",duration:"1:20",slide:{count:1,timestamps:["0:05"]},segments:[{label:"一",startTime:"0:05",endTime:"1:10"}]}]}]};const a={audioUrlMap:{"old-01":"resolved/audio"}};export{c as c,a as a};`
const testService = `const A="https://ov4.oss-cn-hongkong.aliyuncs.com/",h="https://media.example/55/";import("./data-mock-test.js");`

func TestDecodePreservesIdentityAndResolvesResources(t *testing.T) {
	data, err := decode([]byte(testData), []byte(testService))
	if err != nil {
		t.Fatal(err)
	}
	var courses []map[string]any
	if err := json.Unmarshal(data, &courses); err != nil {
		t.Fatal(err)
	}
	lesson := courses[0]["lessons"].([]any)[0].(map[string]any)
	if courses[0]["id"] != "old" || lesson["id"] != "old-01" || lesson["audioUrl"] != "https://media.example/55/resolved/audio.mp3?f=old-01.mp3" || lesson["duration"] != float64(80) {
		t.Fatalf("unexpected lesson: %v", lesson)
	}
	slide := lesson["slides"].([]any)[0].(map[string]any)
	if slide["time"] != float64(5) || slide["url"] != "https://ov4.oss-cn-hongkong.aliyuncs.com/svg/old/old-01-01.svg" {
		t.Fatalf("unexpected slide: %v", slide)
	}
	if _, err := decode([]byte(strings.Replace(testData, `id:"old-01"`, `id:""`, 1)), []byte(testService)); err == nil {
		t.Fatal("accepted missing lesson identity")
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRefreshRetainsSnapshotAcrossFailureAndRestart(t *testing.T) {
	root := t.TempDir()
	c := New(root)
	fail := false
	data := testData
	c.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if fail {
			return nil, errors.New("source unavailable")
		}
		body := data
		switch {
		case r.URL.String() == Source:
			body = `<link href="https://ov5.oss-cn-shenzhen.aliyuncs.com/assets/mock-blog-test.js">`
		case strings.Contains(r.URL.Path, "mock-blog-"):
			body = testService
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	first, ok := c.Snapshot()
	if !ok || first.Stale {
		t.Fatal("successful snapshot missing")
	}
	data = strings.Replace(testData, "resolved/audio", "updated/audio", 1)
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	first, _ = c.Snapshot()
	if !strings.Contains(string(first.Courses), "updated/audio") || !strings.Contains(string(first.Courses), `"id":"old-01"`) {
		t.Fatal("source update lost identity or kept old URL")
	}
	fail = true
	if err := c.Refresh(context.Background()); err == nil {
		t.Fatal("expected refresh failure")
	}
	stale, ok := c.Snapshot()
	if !ok || !stale.Stale || string(stale.Courses) != string(first.Courses) {
		t.Fatal("failure replaced catalogue")
	}
	restored, ok := New(root).Snapshot()
	if !ok || !restored.Stale || string(restored.Courses) != string(first.Courses) {
		t.Fatal("restart lost catalogue")
	}
}

func TestFetchRejectsOtherOrigins(t *testing.T) {
	c := New(t.TempDir())
	for _, address := range []string{"http://ovcm.net/tx2026/", "https://ovcm.net.evil.test/", "https://127.0.0.1/", "https://ovcm.net:444/"} {
		if _, err := c.fetch(context.Background(), address); err == nil {
			t.Fatalf("accepted %s", address)
		}
	}
}

func TestLiveCatalogue(t *testing.T) {
	if os.Getenv("OVCM_LIVE_TEST") != "1" {
		t.Skip("explicit live-source verification only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
	defer cancel()
	c := New(t.TempDir())
	if err := c.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	snapshot, ok := c.Snapshot()
	if !ok {
		t.Fatal("missing live catalogue")
	}
	var courses []map[string]any
	if err := json.Unmarshal(snapshot.Courses, &courses); err != nil {
		t.Fatal(err)
	}
	lessons := 0
	for _, course := range courses {
		lessons += len(course["lessons"].([]any))
	}
	t.Logf("live catalogue: %d courses, %d lessons", len(courses), lessons)
}
