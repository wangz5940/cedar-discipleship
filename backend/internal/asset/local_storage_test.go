package asset

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalStorageSaveRemovesPartialFile(t *testing.T) {
	root := t.TempDir()
	storage := NewLocalStorage(root)

	_, err := storage.Save(context.Background(), "team-agp-resources/objects/00000000000000000000000000000001", "partial.bin", &failingReader{})
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

func TestLocalStorageRejectsUnmanagedPath(t *testing.T) {
	t.Parallel()

	storage := NewLocalStorage(t.TempDir())
	if _, err := storage.Save(context.Background(), "unmanaged", "lesson.pdf", strings.NewReader("pdf")); err == nil {
		t.Fatal("Save accepted an unmanaged resource path")
	}
	if _, err := storage.Resolve(context.Background(), "unmanaged/lesson.pdf"); err == nil {
		t.Fatal("Resolve accepted an unmanaged resource path")
	}
	if err := storage.Delete(context.Background(), "unmanaged/lesson.pdf"); err == nil {
		t.Fatal("Delete accepted an unmanaged resource path")
	}
}

func TestLocalStorageSaveUsesStableResourcePath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	storage := NewLocalStorage(root)
	object, err := storage.Save(
		context.Background(),
		"team-agp-resources/objects/0000000000000000000000000000000a",
		"lesson.pdf",
		strings.NewReader("version two"),
	)
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	want := "team-agp-resources/objects/0000000000000000000000000000000a/lesson.pdf"
	if object.StoragePath != want {
		t.Fatalf("storage path = %q, want %q", object.StoragePath, want)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(want))); err != nil {
		t.Fatalf("saved file missing: %v", err)
	}
}

func TestLocalStorageResolveRejectsSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	storage := NewLocalStorage(root)
	if _, err := storage.Resolve(context.Background(), "linked/secret.txt"); err == nil {
		t.Fatal("Resolve accepted a path containing a symlink")
	}
	if err := storage.Delete(context.Background(), "linked/secret.txt"); err == nil {
		t.Fatal("Delete accepted a path containing a symlink")
	}
	if _, err := os.Stat(filepath.Join(outside, "secret.txt")); err != nil {
		t.Fatalf("outside file was removed: %v", err)
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
