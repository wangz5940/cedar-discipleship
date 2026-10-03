package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"agp/backend/internal/asset"
	"agp/backend/internal/learning"
)

type resourceDownloadTestRepo struct {
	learning.Repository
	settings map[uint64]map[string]any
}

func (r *resourceDownloadTestRepo) LearningConfig(_ context.Context, groupID uint64) (map[string]any, error) {
	return r.settings[groupID], nil
}

func (r *resourceDownloadTestRepo) SaveResourceDownloadEnabled(_ context.Context, groupID uint64, enabled bool) error {
	r.settings[groupID] = map[string]any{"resource_download_enabled": enabled}
	return nil
}

func TestResourceDownloadPolicyKeepsReadingAndPlayback(t *testing.T) {
	path := t.TempDir() + "/lesson.mp4"
	if err := os.WriteFile(path, []byte("0123456789"), 0600); err != nil {
		t.Fatal(err)
	}
	a := &app{
		secret: []byte("test-secret"),
		learning: learning.NewService(&resourceDownloadTestRepo{settings: map[uint64]map[string]any{
			1: {"resource_download_enabled": false},
		}}),
		assets: asset.NewService(&downloadAssetRepo{asset: asset.Asset{ID: 16, GroupID: 1,
			OriginalName: "lesson.mp4", StoragePath: "team-agp-resources/objects/test/lesson.mp4", MimeType: "video/mp4"}},
			&downloadAssetStorage{path: path}, ""),
	}
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
		want    int
	}{
		{"download", a.handleDownloadAsset, http.StatusForbidden},
		{"content", a.handleAssetContent, http.StatusPartialContent},
		{"playback", a.handleAssetPlayback, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/assets/16/"+tc.name, nil)
			r.SetPathValue("id", "16")
			r.Header.Set("Range", "bytes=0-3")
			r = r.WithContext(context.WithValue(r.Context(), currentUserKey, currentUser{ID: 1, CurrentGroupID: 1}))
			w := httptest.NewRecorder()
			tc.handler(w, r)
			if w.Code != tc.want {
				t.Fatalf("status=%d body=%s", w.Code, w.Body)
			}
			if tc.name == "content" && (!strings.HasPrefix(w.Header().Get("Content-Disposition"), "inline") || w.Body.String() != "0123") {
				t.Fatalf("reading response headers=%v body=%s", w.Header(), w.Body)
			}
		})
	}
}

func TestResourceDownloadSettingsRequireAdministrator(t *testing.T) {
	for _, u := range []currentUser{
		{ID: 1, CurrentGroupID: 1, IsSuperAdmin: true},
		{ID: 2, CurrentGroupID: 1, Roles: []string{roleGroupAdmin}},
		{ID: 3, CurrentGroupID: 1},
	} {
		repo := &resourceDownloadTestRepo{settings: map[uint64]map[string]any{}}
		a := &app{learning: learning.NewService(repo)}
		r := httptest.NewRequest(http.MethodPut, "/api/admin/resource-download", strings.NewReader(`{"enabled":false}`))
		r = r.WithContext(context.WithValue(r.Context(), currentUserKey, u))
		w := httptest.NewRecorder()
		a.requireRole(roleGroupAdmin, a.handleResourceDownloadSettings)(w, r)
		allowed := u.IsSuperAdmin || hasRole(u.Roles, roleGroupAdmin)
		if allowed && (w.Code != http.StatusOK || learning.ResourceDownloadsAllowed(repo.settings[1])) {
			t.Fatalf("administrator could not disable downloads: status=%d body=%s", w.Code, w.Body)
		}
		if !allowed && (w.Code != http.StatusForbidden || len(repo.settings) != 0) {
			t.Fatalf("member changed downloads: status=%d settings=%v", w.Code, repo.settings)
		}
	}
}
