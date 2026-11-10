package ovcm

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
)

const Source = "https://ovcm.net/tx2026/"

//go:embed normalize.js
var normalize string

type Snapshot struct {
	Courses   json.RawMessage `json:"courses"`
	SyncedAt  time.Time       `json:"synced_at"`
	CheckedAt time.Time       `json:"checked_at"`
	Stale     bool            `json:"stale"`
}

type Catalogue struct {
	mu       sync.RWMutex
	snapshot Snapshot
	path     string
	client   *http.Client
}

func New(root string) *Catalogue {
	c := &Catalogue{path: filepath.Join(root, ".ovcm", "catalogue.json"), client: &http.Client{Timeout: 20 * time.Second}}
	c.client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !allowed(req.URL) {
			return errors.New("unsupported OVCM redirect")
		}
		return nil
	}
	if content, err := os.ReadFile(c.path); err == nil {
		var snapshot Snapshot
		if json.Unmarshal(content, &snapshot) == nil && validate(snapshot.Courses) == nil {
			snapshot.Stale = true
			c.snapshot = snapshot
		}
	}
	return c
}

func (c *Catalogue) Snapshot() (Snapshot, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.snapshot, len(c.snapshot.Courses) > 0
}

// Run checks the published entry every minute. A failed refresh never replaces
// the last complete catalogue, including across backend restarts.
func (c *Catalogue) Run(ctx context.Context) {
	for {
		_ = c.Refresh(ctx)
		timer := time.NewTimer(time.Minute)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

var (
	serviceLink = regexp.MustCompile(`https://ov5\.oss-cn-shenzhen\.aliyuncs\.com/assets/mock-blog-[A-Za-z0-9_-]+\.js`)
	dataLink    = regexp.MustCompile(`\./data-mock-[A-Za-z0-9_-]+\.js`)
	exports     = regexp.MustCompile(`export\s*\{([^}]+)\};?\s*$`)
	alias       = regexp.MustCompile(`^([A-Za-z_$][\w$]*)\s+as\s+([A-Za-z_$][\w$]*)$`)
	assetBase   = regexp.MustCompile(`https://ov[0-9]+\.oss-[a-z0-9-]+\.aliyuncs\.com/"`)
	mediaBase   = regexp.MustCompile(`https://[a-z0-9.]+/(?:[0-9]+/)+"`)
)

func allowed(u *url.URL) bool {
	return u.Scheme == "https" && u.User == nil && u.Port() == "" && (u.Hostname() == "ovcm.net" || u.Hostname() == "ov5.oss-cn-shenzhen.aliyuncs.com")
}

func (c *Catalogue) fetch(ctx context.Context, address string) ([]byte, error) {
	u, err := url.Parse(address)
	if err != nil || !allowed(u) {
		return nil, errors.New("unsupported OVCM URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OVCM HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20+1))
	if err != nil {
		return nil, err
	}
	if len(body) > 4<<20 {
		return nil, errors.New("OVCM response too large")
	}
	return body, nil
}

func (c *Catalogue) Refresh(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			c.mu.Lock()
			c.snapshot.Stale = true
			c.mu.Unlock()
		}
	}()
	page, err := c.fetch(ctx, Source)
	if err != nil {
		return err
	}
	serviceURL := serviceLink.FindString(string(page))
	if serviceURL == "" {
		return errors.New("OVCM service entry missing")
	}
	service, err := c.fetch(ctx, serviceURL)
	if err != nil {
		return err
	}
	dataPath := dataLink.FindString(string(service))
	if dataPath == "" {
		return errors.New("OVCM data entry missing")
	}
	base, _ := url.Parse(serviceURL)
	relative, _ := url.Parse(dataPath)
	data, err := c.fetch(ctx, base.ResolveReference(relative).String())
	if err != nil {
		return err
	}
	courses, err := decode(data, service)
	if err != nil {
		return err
	}
	snapshot := Snapshot{Courses: courses, SyncedAt: time.Now().UTC(), CheckedAt: time.Now().UTC()}
	content, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(c.path), 0750); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(c.path), ".catalogue-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, err = file.Write(content)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(file.Name(), c.path); err != nil {
		return err
	}
	c.mu.Lock()
	c.snapshot = snapshot
	c.mu.Unlock()
	return nil
}

func decode(data, service []byte) (json.RawMessage, error) {
	match := exports.FindSubmatch(data)
	if match == nil {
		return nil, errors.New("OVCM data exports missing")
	}
	var names []string
	for _, item := range strings.Split(string(match[1]), ",") {
		parts := alias.FindStringSubmatch(strings.TrimSpace(item))
		if parts == nil {
			return nil, errors.New("unsupported OVCM data export")
		}
		names = append(names, parts[2]+":"+parts[1])
	}
	assets := strings.TrimSuffix(assetBase.FindString(string(service)), `"`)
	media := strings.TrimSuffix(mediaBase.FindString(string(service)), `"`)
	// The preload helper also contains an asset origin. The course CDN is the
	// last matching origin before the media base in the published service.
	if index := strings.Index(string(service), media); media != "" && index >= 0 {
		matches := assetBase.FindAllString(string(service[:index]), -1)
		if len(matches) > 0 {
			assets = strings.TrimSuffix(matches[len(matches)-1], `"`)
		}
	}
	if assets == "" || media == "" {
		return nil, errors.New("OVCM media origins missing")
	}
	vm := goja.New()
	_ = vm.Set("assetBase", assets)
	_ = vm.Set("mediaBase", media)
	timer := time.AfterFunc(2*time.Second, func() { vm.Interrupt("OVCM data execution timed out") })
	defer timer.Stop()
	script := exports.ReplaceAllString(string(data), "") + "\nconst modules={" + strings.Join(names, ",") + "};\n" + normalize
	value, err := vm.RunString(script)
	if err != nil {
		return nil, fmt.Errorf("decode OVCM data: %w", err)
	}
	result := json.RawMessage(value.String())
	if err := validate(result); err != nil {
		return nil, err
	}
	return result, nil
}

func validate(data json.RawMessage) error {
	var courses []struct {
		ID      string `json:"id"`
		Title   string `json:"title"`
		Lessons []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"lessons"`
	}
	if len(data) > 16<<20 || json.Unmarshal(data, &courses) != nil || len(courses) == 0 {
		return errors.New("invalid OVCM catalogue")
	}
	ids := map[string]bool{}
	for _, course := range courses {
		if course.ID == "" || course.Title == "" || ids[course.ID] {
			return errors.New("invalid OVCM course identity")
		}
		ids[course.ID] = true
		lessons := map[string]bool{}
		for _, lesson := range course.Lessons {
			if lesson.ID == "" || lesson.Title == "" || lessons[lesson.ID] {
				return errors.New("invalid OVCM lesson identity")
			}
			lessons[lesson.ID] = true
		}
	}
	return nil
}
