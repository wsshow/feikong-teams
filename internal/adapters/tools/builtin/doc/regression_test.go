package doc

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestDocumentWorkspaceAndImmutableSnapshot(t *testing.T) {
	base := t.TempDir()
	tools, err := NewTools(base)
	if err != nil {
		t.Fatal(err)
	}
	outside := writeDocTestFile(t, "outside.txt", "outside")
	resp, err := tools.ReadDocumentSmart(context.Background(), &ReadDocumentSmartRequest{FilePath: outside})
	if err != nil || resp.ErrorMessage == "" {
		t.Fatalf("outside path accepted: %#v %v", resp, err)
	}
	if err := os.Symlink(outside, filepath.Join(base, "link.txt")); err != nil {
		t.Skip(err)
	}
	resp, err = tools.ReadDocumentSmart(context.Background(), &ReadDocumentSmartRequest{FilePath: "link.txt"})
	if err != nil || resp.ErrorMessage == "" {
		t.Fatalf("symlink accepted: %#v %v", resp, err)
	}
	path := filepath.Join(base, "note.txt")
	if err := os.WriteFile(path, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, _, cleanup, err := tools.snapshot(context.Background(), "note.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if err := os.WriteFile(path, []byte("after"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(snapshot)
	if err != nil || string(data) != "before" {
		t.Fatalf("snapshot changed: %q %v", data, err)
	}
}
func TestDocumentOptionalRangesAndUnicodeLimits(t *testing.T) {
	path := writeDocTestFile(t, "note.txt", "first\nsecond\nthird\n")
	tools := docTools(t, path)
	all, err := tools.ReadDocumentByLines(context.Background(), &ReadDocumentByLinesRequest{FilePath: path})
	if err != nil || all.ErrorMessage != "" || all.Content != "first\nsecond\nthird\n" {
		t.Fatalf("omitted range: %#v %v", all, err)
	}
	first, err := tools.ReadDocumentByLines(context.Background(), &ReadDocumentByLinesRequest{FilePath: path, EndLine: intPointer(0)})
	if err != nil || first.Content != "first" {
		t.Fatalf("explicit zero range: %#v %v", first, err)
	}
	if err := os.WriteFile(path, []byte("你好世界\n\n"), 0600); err != nil {
		t.Fatal(err)
	}
	truncated, err := tools.ReadDocumentSmart(context.Background(), &ReadDocumentSmartRequest{FilePath: path, MaxChars: 3, CleanContent: boolPointer(false)})
	if err != nil || truncated.Content != "你好世" || !utf8.ValidString(truncated.Content) || truncated.ReturnedSize != 3 {
		t.Fatalf("Unicode truncation: %#v %v", truncated, err)
	}
	plain, err := tools.ReadDocumentSmart(context.Background(), &ReadDocumentSmartRequest{FilePath: path, CleanContent: boolPointer(false)})
	if err != nil || plain.Content != "你好世界\n\n" {
		t.Fatalf("explicit false ignored: %#v %v", plain, err)
	}
}

func TestDocumentArchiveExpansionLimit(t *testing.T) {
	base := t.TempDir()
	tools, err := NewTools(base)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(base, "large.docx")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	entry, err := archive.Create("large.xml")
	if err != nil {
		t.Fatal(err)
	}
	chunk := make([]byte, 1<<20)
	for range 65 {
		if _, err := entry.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	resp, err := tools.ReadDocumentSmart(context.Background(), &ReadDocumentSmartRequest{FilePath: "large.docx"})
	if err != nil || !strings.Contains(resp.ErrorMessage, "expanded size limit") {
		t.Fatalf("unbounded archive accepted: %#v %v", resp, err)
	}
}
