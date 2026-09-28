package server

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	assetdomain "agp/backend/internal/asset"
	auditdomain "agp/backend/internal/audit"
)

func TestAdminUploadAssetMultipartStorage(t *testing.T) {
	tests := []struct {
		name           string
		uploadSize     int64
		wantDiskBacked bool
	}{
		{name: "small file remains in memory", uploadSize: 1024},
		{name: "large file is spooled to disk", uploadSize: assetUploadMemoryThreshold + 1, wantDiskBacked: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body bytes.Buffer
			form := multipart.NewWriter(&body)
			part, err := form.CreateFormFile("file", "upload.pdf")
			if err != nil {
				t.Fatalf("create file part: %v", err)
			}
			if _, err := io.CopyN(part, zeroReader{}, tt.uploadSize); err != nil {
				t.Fatalf("write file part: %v", err)
			}
			if err := form.Close(); err != nil {
				t.Fatalf("close multipart form: %v", err)
			}

			repo := &uploadAssetRepo{}
			storage := &uploadAssetStorage{}
			a := &app{
				assets: assetdomain.NewService(repo, storage, ""),
				audits: auditdomain.NewService(&uploadAuditRepo{}),
			}
			request := httptest.NewRequest(http.MethodPost, "/api/admin/assets/upload", &body)
			request.Header.Set("Content-Type", form.FormDataContentType())
			request = request.WithContext(context.WithValue(request.Context(), currentUserKey, currentUser{
				ID:             1,
				CurrentGroupID: 1,
			}))
			recorder := httptest.NewRecorder()

			a.handleAdminUploadAsset(recorder, request)

			if recorder.Code != http.StatusCreated {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusCreated)
			}
			if storage.diskBacked != tt.wantDiskBacked {
				t.Fatalf("disk-backed = %t, want %t", storage.diskBacked, tt.wantDiskBacked)
			}
			if storage.size != tt.uploadSize {
				t.Fatalf("stored size = %d, want %d", storage.size, tt.uploadSize)
			}
			if storage.tempPath != "" {
				if _, err := os.Stat(storage.tempPath); !os.IsNotExist(err) {
					t.Fatalf("multipart temp file still exists after upload: %v", err)
				}
			}
		})
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

type uploadAssetRepo struct{}

func (*uploadAssetRepo) FindByID(context.Context, uint64, uint64) (*assetdomain.Asset, error) {
	return nil, os.ErrNotExist
}

func (*uploadAssetRepo) List(context.Context, uint64, int) ([]assetdomain.Asset, error) {
	return nil, nil
}

func (*uploadAssetRepo) Create(context.Context, *assetdomain.Asset, uint64) (uint64, error) {
	return 1, nil
}

func (*uploadAssetRepo) Delete(context.Context, uint64, uint64) error {
	return nil
}

func (*uploadAssetRepo) GroupCode(context.Context, uint64) (string, error) {
	return "test-group", nil
}

type uploadAssetStorage struct {
	diskBacked bool
	size       int64
	tempPath   string
}

func (s *uploadAssetStorage) Save(_ context.Context, _, _ string, src io.Reader) (*assetdomain.StoredObject, error) {
	if file, ok := src.(*os.File); ok {
		s.diskBacked = true
		s.tempPath = file.Name()
	}
	size, err := io.Copy(io.Discard, src)
	if err != nil {
		return nil, err
	}
	s.size = size
	return &assetdomain.StoredObject{
		StoragePath:    "test-group/objects/test/large.pdf",
		FileSize:       uint64(size),
		ChecksumSHA256: "checksum",
	}, nil
}

func (*uploadAssetStorage) Resolve(context.Context, string) (*assetdomain.ResolvedObject, error) {
	return nil, os.ErrNotExist
}

func (*uploadAssetStorage) Delete(context.Context, string) error {
	return nil
}

type uploadAuditRepo struct{}

func (*uploadAuditRepo) Create(context.Context, auditdomain.Log) error {
	return nil
}

func (*uploadAuditRepo) ListByGroup(context.Context, uint64, int) ([]auditdomain.Log, error) {
	return nil, nil
}
