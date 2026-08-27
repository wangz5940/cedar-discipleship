package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyRelativePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantOK  bool
		wantErr bool
	}{
		{
			name:   "decoded absolute-style resource path",
			input:  "/PPT/%E5%AF%BC%E8%AF%BB.pdf",
			want:   filepath.Join("PPT", "导读.pdf"),
			wantOK: true,
		},
		{
			name:   "api asset path is skipped",
			input:  "/api/assets/12/download",
			wantOK: false,
		},
		{
			name:    "path traversal is rejected",
			input:   "/../secret.pdf",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := legacyRelativePath(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("legacyRelativePath() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("legacyRelativePath() error = %v", err)
			}
			if ok != tt.wantOK {
				t.Fatalf("legacyRelativePath() ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Fatalf("legacyRelativePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMigratedCategory(t *testing.T) {
	t.Parallel()

	got := migratedCategory(legacyAsset{
		Category:    "share",
		Title:       "马太福音导读",
		StoragePath: "/Mentor/matthew.pdf",
	})
	if got != "mentor" {
		t.Fatalf("migratedCategory() = %q, want mentor", got)
	}
}

func TestNormalizedLegacyKey(t *testing.T) {
	t.Parallel()

	encoded := normalizedLegacyKey("/Newtestament/%5BB311%5D%E6%96%B0%E7%BA%A6.mp4")
	decoded := normalizedLegacyKey("Newtestament/[B311]新约.mp4")
	if encoded == "" || encoded != decoded {
		t.Fatalf("normalized keys = %q and %q, want equal non-empty values", encoded, decoded)
	}
}

func TestDiscoverLegacyResourceFilesScansKnownDirectories(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	files := []string{
		"Kuangye.md",
		"newtestament.md",
		"weekly_task.md",
		filepath.Join("Book", "基督是一切-江守道.pdf"),
		filepath.Join("Mentor", "马太福音（上）导读.pdf"),
		filepath.Join("Newtestament", "[B311]新约圣经-08.mp4"),
		filepath.Join("PPT", "马可福音讲义.pptx"),
		filepath.Join("Passage", "经文.pdf"),
		filepath.Join("Book", "ignored.tmp"),
		filepath.Join("Book", ".hidden.pdf"),
		filepath.Join("Book", ".cache", "hidden.pdf"),
	}
	for _, name := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		if err := os.WriteFile(path, []byte("resource"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
	}

	got, err := discoverLegacyResourceFiles(root)
	if err != nil {
		t.Fatalf("discoverLegacyResourceFiles() error = %v", err)
	}
	var values []string
	for _, file := range got {
		values = append(values, filepath.ToSlash(file.RelativePath)+":"+file.Category)
	}
	want := []string{
		"Book/基督是一切-江守道.pdf:book",
		"Kuangye.md:markdown",
		"Mentor/马太福音（上）导读.pdf:mentor",
		"Newtestament/[B311]新约圣经-08.mp4:video",
		"PPT/马可福音讲义.pptx:handout",
		"Passage/经文.pdf:passage",
		"newtestament.md:markdown",
		"weekly_task.md:markdown",
	}
	if strings.Join(values, "\n") != strings.Join(want, "\n") {
		t.Fatalf("legacy files = %v, want %v", values, want)
	}
}

func TestConfigResourceFileNameExtractsURLPath(t *testing.T) {
	t.Parallel()

	got := configResourceFileName("https://mouss.synology.me:7399/newtestament.md")
	if got != "newtestament.md" {
		t.Fatalf("configResourceFileName() = %q, want newtestament.md", got)
	}
}

func TestResolveLegacySourcePathFallsBackToDataAssets(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	relative := filepath.Join("1", "ministry-2", "1787207296413257683-演示文稿1.pptx")
	assetPath := filepath.Join(root, "data", "assets", relative)
	if err := os.MkdirAll(filepath.Dir(assetPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(assetPath, []byte("ppt"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, ok, err := resolveLegacySourcePath(root, "", relative)
	if err != nil {
		t.Fatalf("resolveLegacySourcePath() error = %v", err)
	}
	if !ok {
		t.Fatal("resolveLegacySourcePath() ok = false, want true")
	}
	if got != assetPath {
		t.Fatalf("resolveLegacySourcePath() = %q, want %q", got, assetPath)
	}
}

func TestResolveLegacySourcePathUsesConfiguredAssetsRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	assetsRoot := t.TempDir()
	relative := filepath.Join("1", "outline", "1787212447286017312-image.JPG")
	assetPath := filepath.Join(assetsRoot, relative)
	if err := os.MkdirAll(filepath.Dir(assetPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(assetPath, []byte("image"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, ok, err := resolveLegacySourcePath(root, assetsRoot, relative)
	if err != nil {
		t.Fatalf("resolveLegacySourcePath() error = %v", err)
	}
	if !ok {
		t.Fatal("resolveLegacySourcePath() ok = false, want true")
	}
	if got != assetPath {
		t.Fatalf("resolveLegacySourcePath() = %q, want %q", got, assetPath)
	}
}

func TestPreferCleanupAssetUsesCanonicalFileTitle(t *testing.T) {
	t.Parallel()

	pageScoped := cleanupAssetCandidate{
		ID:             7,
		Category:       "book",
		Title:          "《基督是一切》36-40页",
		OriginalName:   "基督是一切-江守道.pdf",
		FileSize:       1024,
		ChecksumSHA256: "0ede4c556a220000000000000000000000000000000000000000000000000000",
		AssetKind:      assetKindOwned,
	}
	canonical := cleanupAssetCandidate{
		ID:             21,
		Category:       "book",
		Title:          "基督是一切-江守道",
		OriginalName:   "基督是一切-江守道.pdf",
		FileSize:       1024,
		ChecksumSHA256: "0ede4c556a220000000000000000000000000000000000000000000000000000",
		AssetKind:      assetKindOwned,
	}
	if !preferCleanupAsset(canonical, pageScoped) {
		t.Fatal("preferCleanupAsset() did not choose canonical file title over page-scoped title")
	}
	if preferCleanupAsset(pageScoped, canonical) {
		t.Fatal("preferCleanupAsset() chose page-scoped title over canonical file title")
	}
}

func TestSafeFileName(t *testing.T) {
	t.Parallel()

	got := safeFileName("../导读.pdf")
	if got != "导读.pdf" {
		t.Fatalf("safeFileName() = %q, want 导读.pdf", got)
	}
}
