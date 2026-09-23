package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestWritePDFRangeErrorIncludesCauseWithoutStackOrPath(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		detail string
	}{
		{"pdfcpu", errors.New("pdfcpu: dict=markupAnnot entry=Subj: unsupported in version 1.3\ninternal stack"), "pdfcpu: dict=markupAnnot entry=Subj: unsupported in version 1.3"},
		{"file", fmt.Errorf("stat source PDF: %w", &os.PathError{Op: "stat", Path: "/private/source.pdf", Err: os.ErrNotExist}), "PDF file access failed: file does not exist"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writePDFRangeError(w, tc.err)
			if w.Code != http.StatusInternalServerError || w.Header().Get("X-AGP-Error-Code") != "pdf_range_failed" {
				t.Fatalf("status=%d error_code=%q", w.Code, w.Header().Get("X-AGP-Error-Code"))
			}
			var body struct {
				Error  string `json:"error"`
				Detail string `json:"detail"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error != "pdf_range_failed" || body.Detail != tc.detail {
				t.Fatalf("response = %+v", body)
			}
		})
	}
}

func TestTrimPDFRangeWithNewerAnnotationThanDeclaredVersion(t *testing.T) {
	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.3\n")
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Annots [5 0 R] >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>",
		"<< /Type /Annot /Subtype /Text /Rect [10 10 20 20] /Contents (note) /Subj (subject) >>",
	}
	offsets := []int{0}
	for i, object := range objects {
		offsets = append(offsets, pdf.Len())
		fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	src := filepath.Join(t.TempDir(), "source.pdf")
	if err := os.WriteFile(src, pdf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}

	trimmed, err := trimPDFRange(src, "1-1")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(trimmed, []byte("%PDF-")) {
		t.Fatalf("trimmed output is not a PDF: %q", trimmed[:min(len(trimmed), 8)])
	}
	unchanged, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(unchanged, pdf.Bytes()) {
		t.Fatal("source PDF was modified")
	}
}
