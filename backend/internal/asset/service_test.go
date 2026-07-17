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
