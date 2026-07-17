package asset

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalStorageSaveRemovesPartialFile(t *testing.T) {
	root := t.TempDir()
	storage := NewLocalStorage(root)

	_, err := storage.Save(context.Background(), "group", "partial.bin", &failingReader{})
	if err == nil {
		t.Fatal("Save succeeded, want copy error")
	}

	var files []string
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk storage root: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("partial files = %v, want none", files)
	}
}

type failingReader struct {
	read bool
}

func (r *failingReader) Read(buffer []byte) (int, error) {
	if !r.read {
		r.read = true
		return copy(buffer, "partial"), nil
	}
	return 0, errors.New("read failed")
}

var _ io.Reader = (*failingReader)(nil)
