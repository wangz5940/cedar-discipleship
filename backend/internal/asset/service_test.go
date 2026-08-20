package asset

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestServiceUploadDeletesStoredFileWhenMetadataCreateFails(t *testing.T) {
	createErr := errors.New("create failed")
	storage := &fakeStorage{
		stored: &StoredObject{StoragePath: "1/book/upload.pdf"},
	}
	service := NewService(&fakeRepository{createErr: createErr}, storage, "")

	_, err := service.Upload(context.Background(), UploadRequest{
		GroupID:  1,
		ActorID:  2,
		Category: "book",
		FileName: "upload.pdf",
		Reader:   strings.NewReader("pdf"),
	})
	if !errors.Is(err, createErr) {
		t.Fatalf("Upload error = %v, want %v", err, createErr)
	}
	if storage.deleted != storage.stored.StoragePath {
		t.Fatalf("deleted path = %q, want %q", storage.deleted, storage.stored.StoragePath)
	}
}

func TestInferTaskBindingType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		taskType string
		url      string
		fileName string
		want     string
	}{
		{name: "PNG image", fileName: "本周提纲.png", want: "image"},
		{name: "JPEG image without outline in name", fileName: "week-12.jpeg", want: "image"},
		{name: "WebP image", fileName: "diagram.webp", want: "image"},
		{name: "video", fileName: "lesson.mp4", want: "video"},
		{name: "markdown", fileName: "lesson.md", want: "markdown"},
		{name: "PDF reading", fileName: "lesson.pdf", want: "reading"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := inferTaskBindingType(tt.taskType, tt.url, tt.fileName); got != tt.want {
				t.Fatalf("inferTaskBindingType(%q, %q, %q) = %q, want %q", tt.taskType, tt.url, tt.fileName, got, tt.want)
			}
		})
	}
}

type fakeRepository struct {
	createErr error
}

func (r *fakeRepository) FindByID(context.Context, uint64, uint64) (*Asset, error) {
	return nil, errors.New("not implemented")
}

func (r *fakeRepository) List(context.Context, uint64, int) ([]Asset, error) {
	return nil, errors.New("not implemented")
}

func (r *fakeRepository) Create(context.Context, *Asset, uint64) (uint64, error) {
	return 0, r.createErr
}

func (r *fakeRepository) Delete(context.Context, uint64, uint64) error {
	return errors.New("not implemented")
}

type fakeStorage struct {
	stored  *StoredObject
	deleted string
}

func (s *fakeStorage) Save(context.Context, string, string, io.Reader) (*StoredObject, error) {
	return s.stored, nil
}

func (s *fakeStorage) Resolve(context.Context, string) (*ResolvedObject, error) {
	return nil, errors.New("not implemented")
}

func (s *fakeStorage) Delete(_ context.Context, objectKey string) error {
	s.deleted = objectKey
	return nil
}
