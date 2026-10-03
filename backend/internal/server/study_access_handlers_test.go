package server

import (
	assetdomain "agp/backend/internal/asset"
	"agp/backend/internal/studymemory"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type learningTestRepository struct {
	keys     map[uint64]string
	handouts map[[2]uint64]*studymemory.Handout
}

func (r *learningTestRepository) Access(_ context.Context, id uint64, key string) (bool, error) {
	return key != "" && r.keys[id] == key, nil
}
func (r *learningTestRepository) Unlock(_ context.Context, id uint64, key string) error {
	if r.keys == nil {
		r.keys = map[uint64]string{}
	}
	r.keys[id] = key
	return nil
}
func (r *learningTestRepository) Revoke(_ context.Context, id uint64) error {
	delete(r.keys, id)
	return nil
}
func (r *learningTestRepository) Handout(_ context.Context, g, m uint64) (*studymemory.Handout, error) {
	return r.handouts[[2]uint64{g, m}], nil
}
func (r *learningTestRepository) SaveHandout(_ context.Context, g, m uint64, h *studymemory.Handout) error {
	if r.handouts == nil {
		r.handouts = map[[2]uint64]*studymemory.Handout{}
	}
	r.handouts[[2]uint64{g, m}] = h
	return nil
}

type handoutAssetRepository struct {
	renameAssetRepository
	bindings map[[2]uint64]assetdomain.Asset
}

func (r *handoutAssetRepository) FindByID(_ context.Context, g, id uint64) (*assetdomain.Asset, error) {
	item, ok := r.bindings[[2]uint64{g, id}]
	if !ok {
		return nil, errors.New("not_found")
	}
	return &item, nil
}
func TestStudyAccessKeyAndAccountIsolation(t *testing.T) {
	hash := sha256.Sum256([]byte("test-only-key"))
	repo := &learningTestRepository{}
	a := &app{studyLearning: repo, ovcmKeyHash: hex.EncodeToString(hash[:])}
	for _, tc := range []struct {
		body string
		code int
	}{{`{"key":"wrong"}`, 403}, {`{"key":"test-only-key"}`, 200}} {
		w := httptest.NewRecorder()
		a.handleToggleStudyAccess(w, studyRequest(11, 1, tc.body))
		if w.Code != tc.code {
			t.Fatalf("status %d: %s", w.Code, w.Body)
		}
	}
	w := httptest.NewRecorder()
	a.handleToggleStudyAccess(w, studyRequest(11, 1, `{"key":"wrong"}`))
	if w.Code != 403 || repo.keys[11] == "" {
		t.Fatal("wrong key changed unlocked state")
	}
	w = httptest.NewRecorder()
	a.handleToggleStudyAccess(w, studyRequest(11, 1, `{"key":"test-only-key"}`))
	if w.Code != 200 || w.Body.String() != `{"unlocked":false}`+"\n" || repo.keys[11] != "" {
		t.Fatal("second valid key did not revoke access", w.Body)
	}
	w = httptest.NewRecorder()
	a.handleToggleStudyAccess(w, studyRequest(11, 1, `{"key":"test-only-key"}`))
	if w.Code != 200 || w.Body.String() != `{"unlocked":true}`+"\n" {
		t.Fatal("third valid key did not restore access", w.Body)
	}
	for _, tc := range []struct {
		id   uint64
		want string
	}{{11, `{"unlocked":true}`}, {22, `{"unlocked":false}`}} {
		w := httptest.NewRecorder()
		a.handleStudyAccess(w, studyRequest(tc.id, 2, ""))
		if w.Body.String() != tc.want+"\n" {
			t.Fatal(w.Body)
		}
	}
	a.ovcmKeyHash = "different-key"
	w = httptest.NewRecorder()
	a.handleStudyAccess(w, studyRequest(11, 1, ""))
	if w.Body.String() != `{"unlocked":false}`+"\n" {
		t.Fatal("old key survived rotation")
	}
	w = httptest.NewRecorder()
	a.auth(a.handleToggleStudyAccess)(w, httptest.NewRequest("POST", "/api/study-access", nil))
	if w.Code != 401 {
		t.Fatal("anonymous unlock allowed")
	}
}
func TestHandoutPermissionsAndGroupIsolation(t *testing.T) {
	repo := &learningTestRepository{}
	assets := &handoutAssetRepository{bindings: map[[2]uint64]assetdomain.Asset{
		{1, 9}: {ID: 9, MimeType: "audio/mpeg"}, {1, 10}: {ID: 10, MimeType: "application/pdf"}, {2, 9}: {ID: 9, MimeType: "audio/mpeg"}, {2, 20}: {ID: 20, MimeType: "application/pdf"},
	}}
	a := &app{studyLearning: repo, assets: assetdomain.NewService(assets, nil, "")}
	run := func(group uint64, body string, admin bool) *httptest.ResponseRecorder {
		r := studyRequest(11, group, body)
		r.SetPathValue("id", "9")
		u := mustUser(r)
		if admin {
			u.Roles = []string{roleGroupAdmin}
		}
		r = r.WithContext(context.WithValue(r.Context(), currentUserKey, u))
		w := httptest.NewRecorder()
		a.requireRole(roleGroupAdmin, a.handleMediaHandout)(w, r)
		return w
	}
	valid := `{"groupId":1,"handout":{"assetId":10,"pages":[{"page":1,"time":0},{"page":2,"time":90}]}}`
	if w := run(1, valid, false); w.Code != 403 {
		t.Fatal("member edited shared handout")
	}
	if w := run(1, valid, true); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if h, _ := repo.Handout(context.Background(), 2, 9); h != nil {
		t.Fatal("handout crossed group boundary")
	}
	for _, body := range []string{`{"groupId":1,"handout":{"assetId":20,"pages":[]}}`, `{"groupId":1,"handout":{"assetId":10,"pages":[{"page":0,"time":0}]}}`, `{"groupId":2,"handout":null}`} {
		if w := run(1, body, true); w.Code == 200 {
			t.Fatalf("invalid accepted: %s", body)
		}
	}
	if w := run(1, `{"groupId":1,"handout":null}`, true); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if h, _ := repo.Handout(context.Background(), 1, 9); h != nil {
		t.Fatal("unlink retained configuration")
	}
	r := studyRequest(11, 1, "")
	r.Method = http.MethodGet
	r.SetPathValue("id", "9")
	w := httptest.NewRecorder()
	a.handleMediaHandout(w, r)
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
}
