package server

import (
	"agp/backend/internal/studymemory"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type studyMemoryTestRepository struct {
	users map[uint64]studymemory.Snapshot
}

func (r *studyMemoryTestRepository) Load(_ context.Context, id uint64) (studymemory.Snapshot, error) {
	if r.users == nil {
		r.users = map[uint64]studymemory.Snapshot{}
	}
	if _, ok := r.users[id]; !ok {
		r.users[id] = studymemory.Snapshot{Progress: map[string]studymemory.Progress{}, Favorites: map[string]studymemory.Favorite{}}
	}
	return r.users[id], nil
}
func (r *studyMemoryTestRepository) SaveProgress(ctx context.Context, id uint64, key string, p studymemory.Progress) error {
	s, _ := r.Load(ctx, id)
	s.Progress[key] = p
	return nil
}
func (r *studyMemoryTestRepository) SetFavorite(ctx context.Context, id uint64, key string, f *studymemory.Favorite) error {
	s, _ := r.Load(ctx, id)
	if f == nil {
		delete(s.Favorites, key)
	} else {
		s.Favorites[key] = *f
	}
	return nil
}
func studyRequest(id, groupID uint64, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPut, "/api/study-memory", strings.NewReader(body))
	return r.WithContext(context.WithValue(r.Context(), currentUserKey, currentUser{ID: id, CurrentGroupID: groupID}))
}
func TestStudyMemoryAccountIsolationAndCrossGroupSync(t *testing.T) {
	repo := &studyMemoryTestRepository{}
	a := &app{studyMemory: repo}
	for _, id := range []uint64{11, 22} {
		w := httptest.NewRecorder()
		a.handleStudyProgress(w, studyRequest(id, 1, `{"key":"ovcm:rte/04","time":1182,"duration":6000,"completed":false,"user_id":999}`))
		if w.Code != http.StatusOK {
			t.Fatalf("save status %d: %s", w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	a.handleStudyFavorite(w, studyRequest(11, 1, `{"key":"ovcm:rte/04","favorite":true,"item":{"key":"ovcm:rte/04","kind":"ovcm","courseId":"rte","lessonId":"04","type":"audio","title":"第四课"}}`))
	if w.Code != http.StatusOK {
		t.Fatal(w.Body.String())
	}
	read := func(id, group uint64) studymemory.Snapshot {
		w := httptest.NewRecorder()
		a.handleStudyMemory(w, studyRequest(id, group, ""))
		if w.Code != http.StatusOK {
			t.Fatal(w.Body.String())
		}
		var data studymemory.Snapshot
		if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
			t.Fatal(err)
		}
		return data
	}
	if got := read(11, 2); got.Progress["ovcm:rte/04"].Time != 1182 || len(got.Favorites) != 1 {
		t.Fatalf("same account across groups: %+v", got)
	}
	if len(read(22, 1).Favorites) != 0 || len(read(33, 1).Progress) != 0 || len(read(999, 1).Progress) != 0 {
		t.Fatal("data crossed account boundary")
	}
	w = httptest.NewRecorder()
	a.handleStudyFavorite(w, studyRequest(22, 1, `{"key":"ovcm:rte/04","favorite":false}`))
	if len(read(11, 1).Favorites) != 1 {
		t.Fatal("another user removed owner favorite")
	}
	w = httptest.NewRecorder()
	a.handleStudyFavorite(w, studyRequest(11, 2, `{"key":"ovcm:rte/04","favorite":false}`))
	if len(read(11, 1).Favorites) != 0 || read(11, 1).Progress["ovcm:rte/04"].Time != 1182 {
		t.Fatal("cancel must preserve progress")
	}
}
func TestStudyMemoryRejectsInvalidInputAndAnonymousRequests(t *testing.T) {
	a := &app{studyMemory: &studyMemoryTestRepository{}}
	for _, body := range []string{`{"key":"","time":0}`, `{"key":"audio","time":-1}`, `{"key":"audio","time":0,"duration":-1}`} {
		w := httptest.NewRecorder()
		a.handleStudyProgress(w, studyRequest(11, 1, body))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("accepted %s", body)
		}
	}
	for _, body := range []string{`{"key":"a","favorite":true}`, `{"key":"a","favorite":true,"item":{"key":"other","kind":"ovcm","title":"课时","type":"audio","courseId":"rte","lessonId":"04"}}`, `{"key":"a","favorite":true,"item":{"key":"a","kind":"media","title":"课时","type":"video","url":"javascript:alert(1)"}}`} {
		w := httptest.NewRecorder()
		a.handleStudyFavorite(w, studyRequest(11, 1, body))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("accepted %s", body)
		}
	}
	mux := http.NewServeMux()
	a.routes(mux)
	for _, path := range []string{"/api/study-memory", "/api/study-memory/progress", "/api/study-memory/favorites"} {
		method := http.MethodPut
		if path == "/api/study-memory" {
			method = http.MethodGet
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous %s: %d", path, w.Code)
		}
	}
}
