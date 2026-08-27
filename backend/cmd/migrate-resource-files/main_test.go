package main

import (
	"path/filepath"
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
