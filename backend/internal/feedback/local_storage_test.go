package feedback

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalStorageRoundTrip(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	storage := NewLocalStorage(root)
	stored, err := storage.Save(t.Context(), ".png", []byte("sanitized"))
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if !strings.HasPrefix(stored.StoragePath, "feedback/objects/") ||
		filepath.Ext(stored.StoragePath) != ".png" ||
		stored.FileSize != 9 {
		t.Fatalf("stored object = %#v", stored)
	}
	resolved, err := storage.Resolve(t.Context(), stored.StoragePath)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	content, err := os.ReadFile(resolved.AbsolutePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(content, []byte("sanitized")) {
		t.Fatalf("stored content = %q", content)
	}
	if err := storage.Delete(t.Context(), stored.StoragePath); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := os.Stat(resolved.AbsolutePath); !os.IsNotExist(err) {
		t.Fatalf("deleted path stat error = %v", err)
	}
}

func TestLocalStorageRejectsPathsOutsideFeedbackNamespace(t *testing.T) {
	t.Parallel()

	storage := NewLocalStorage(t.TempDir())
	for _, path := range []string{"../secret", "feedback/../secret", "team-agp-resources/objects/file.png"} {
		if _, err := storage.Resolve(t.Context(), path); err == nil {
			t.Fatalf("Resolve(%q) succeeded", path)
		}
		if err := storage.Delete(t.Context(), path); err == nil {
			t.Fatalf("Delete(%q) succeeded", path)
		}
	}
}
