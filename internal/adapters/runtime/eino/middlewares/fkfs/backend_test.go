package fkfs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk/filesystem"
)

func TestLocalBackendCreatesBaseDirAndListsEntries(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "workspace")
	backend, err := NewLocalBackend(baseDir)
	if err != nil {
		t.Fatalf("NewLocalBackend() error = %v", err)
	}
	writeTestFile(t, baseDir, "README.md", "hello")
	if err := os.Mkdir(filepath.Join(baseDir, "docs"), 0755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}

	entries, err := backend.LsInfo(context.Background(), &filesystem.LsInfoRequest{})
	if err != nil {
		t.Fatalf("LsInfo() error = %v", err)
	}

	got := map[string]bool{}
	for _, entry := range entries {
		got[entry.Path] = entry.IsDir
	}
	if got["README.md"] {
		t.Fatalf("README.md should be a file: %v", got)
	}
	if !got["docs"] {
		t.Fatalf("docs should be a directory: %v", got)
	}
}

func TestLocalBackendReadSupportsLineWindow(t *testing.T) {
	baseDir := t.TempDir()
	backend := newTestBackend(t, baseDir)
	writeTestFile(t, baseDir, "notes.txt", "line1\nline2\nline3\nline4")

	full, err := backend.Read(context.Background(), &filesystem.ReadRequest{FilePath: filepath.Join(baseDir, "notes.txt")})
	if err != nil {
		t.Fatalf("Read full file: %v", err)
	}
	if full.Content != "line1\nline2\nline3\nline4" {
		t.Fatalf("full content = %q", full.Content)
	}

	window, err := backend.Read(context.Background(), &filesystem.ReadRequest{
		FilePath: "notes.txt",
		Offset:   2,
		Limit:    2,
	})
	if err != nil {
		t.Fatalf("Read window: %v", err)
	}
	if window.Content != "line2\nline3" {
		t.Fatalf("window content = %q, want line2\\nline3", window.Content)
	}
}

func TestLocalBackendReadLimitZeroReturnsEntireFile(t *testing.T) {
	baseDir := t.TempDir()
	backend := newTestBackend(t, baseDir)
	lines := make([]string, 0, 2501)
	for i := 0; i < 2501; i++ {
		lines = append(lines, fmt.Sprintf("line%d", i+1))
	}
	content := strings.Join(lines, "\n")
	writeTestFile(t, baseDir, "large.txt", content)

	got, err := backend.Read(context.Background(), &filesystem.ReadRequest{FilePath: "large.txt"})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if got.Content != content {
		t.Fatalf("Read() returned %d bytes, want %d bytes", len(got.Content), len(content))
	}
}

func TestLocalBackendSupportsVirtualAbsolutePath(t *testing.T) {
	baseDir := t.TempDir()
	backend := newTestBackend(t, baseDir)
	writeTestFile(t, baseDir, "docs/guide.md", "hello")

	got, err := backend.Read(context.Background(), &filesystem.ReadRequest{FilePath: "/docs/guide.md"})
	if err != nil {
		t.Fatalf("Read virtual absolute path: %v", err)
	}
	if got.Content != "hello" {
		t.Fatalf("content = %q, want hello", got.Content)
	}
}

func TestLocalBackendRejectsPathEscape(t *testing.T) {
	baseDir := t.TempDir()
	backend := newTestBackend(t, baseDir)
	writeTestFile(t, filepath.Dir(baseDir), "outside.txt", "outside")

	if _, err := backend.Read(context.Background(), &filesystem.ReadRequest{FilePath: "../outside.txt"}); err == nil {
		t.Fatal("Read() error = nil, want path escape error")
	}
}

func TestLocalBackendMissingFileWrapsNotExist(t *testing.T) {
	backend := newTestBackend(t, t.TempDir())

	_, err := backend.Read(context.Background(), &filesystem.ReadRequest{FilePath: "missing.txt"})
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Read() error = %v, want os.ErrNotExist", err)
	}
}

func TestLocalBackendGrepFiltersAndAddsContext(t *testing.T) {
	baseDir := t.TempDir()
	backend := newTestBackend(t, baseDir)
	writeTestFile(t, baseDir, "main.go", "before\npanic here\nafter\n")
	writeTestFile(t, baseDir, "README.md", "panic in docs\n")
	writeTestFile(t, baseDir, "nested/other.go", "nothing\n")

	matches, err := backend.GrepRaw(context.Background(), &filesystem.GrepRequest{
		Pattern:         "PANIC",
		Path:            baseDir,
		FileType:        "go",
		Glob:            "*.go",
		BeforeLines:     1,
		AfterLines:      1,
		CaseInsensitive: true,
	})
	if err != nil {
		t.Fatalf("GrepRaw() error = %v", err)
	}

	got := grepLines(matches)
	want := []string{"1:before", "2:panic here", "3:after"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("grep lines = %v, want %v", got, want)
	}
}

func TestLocalBackendGrepRejectsInvalidInput(t *testing.T) {
	backend := newTestBackend(t, t.TempDir())

	if _, err := backend.GrepRaw(context.Background(), &filesystem.GrepRequest{}); err == nil {
		t.Fatal("GrepRaw() error = nil, want empty pattern error")
	}
	if _, err := backend.GrepRaw(context.Background(), &filesystem.GrepRequest{Pattern: "["}); err == nil {
		t.Fatal("GrepRaw() error = nil, want invalid regexp error")
	}
}

func TestLocalBackendGlobInfoWriteAndEdit(t *testing.T) {
	baseDir := t.TempDir()
	backend := newTestBackend(t, baseDir)

	if err := backend.Write(context.Background(), &filesystem.WriteRequest{
		FilePath: "src/app.go",
		Content:  "package main\nvar value = 1\n",
	}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := backend.Edit(context.Background(), &filesystem.EditRequest{
		FilePath:   "src/app.go",
		OldString:  "value = 1",
		NewString:  "value = 2",
		ReplaceAll: false,
	}); err != nil {
		t.Fatalf("Edit() error = %v", err)
	}
	content, err := os.ReadFile(filepath.Join(baseDir, "src", "app.go"))
	if err != nil {
		t.Fatalf("read edited file: %v", err)
	}
	if !strings.Contains(string(content), "value = 2") {
		t.Fatalf("edited content = %q, want value = 2", content)
	}

	matches, err := backend.GlobInfo(context.Background(), &filesystem.GlobInfoRequest{
		Path:    baseDir,
		Pattern: "**/*.go",
	})
	if err != nil {
		t.Fatalf("GlobInfo() error = %v", err)
	}
	got := make([]string, 0, len(matches))
	for _, match := range matches {
		got = append(got, match.Path)
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, []string{"src/app.go"}) {
		t.Fatalf("glob matches = %v, want src/app.go", got)
	}

	absoluteMatches, err := backend.GlobInfo(context.Background(), &filesystem.GlobInfoRequest{
		Pattern: "/src/*.go",
	})
	if err != nil {
		t.Fatalf("GlobInfo() absolute error = %v", err)
	}
	got = got[:0]
	for _, match := range absoluteMatches {
		got = append(got, match.Path)
	}
	if !reflect.DeepEqual(got, []string{"/src/app.go"}) {
		t.Fatalf("absolute glob matches = %v, want /src/app.go", got)
	}
}

func TestLocalBackendEditValidatesSingleReplacement(t *testing.T) {
	baseDir := t.TempDir()
	backend := newTestBackend(t, baseDir)
	writeTestFile(t, baseDir, "repeat.txt", "same\nsame\n")

	err := backend.Edit(context.Background(), &filesystem.EditRequest{
		FilePath:  "repeat.txt",
		OldString: "same",
		NewString: "other",
	})
	if err == nil || !strings.Contains(err.Error(), "multiple times") {
		t.Fatalf("Edit() error = %v, want multiple match error", err)
	}

	err = backend.Edit(context.Background(), &filesystem.EditRequest{
		FilePath:  "repeat.txt",
		OldString: "",
		NewString: "other",
	})
	if err == nil || !strings.Contains(err.Error(), "oldString cannot be empty") {
		t.Fatalf("Edit() error = %v, want empty oldString error", err)
	}
}

func TestMatchFileType(t *testing.T) {
	tests := []struct {
		ext      string
		fileType string
		want     bool
	}{
		{ext: "tsx", fileType: "typescript", want: true},
		{ext: "pyi", fileType: "python", want: true},
		{ext: "txt", fileType: "txt", want: true},
		{ext: "md", fileType: "go", want: false},
	}
	for _, tt := range tests {
		if got := matchFileType(tt.ext, tt.fileType); got != tt.want {
			t.Fatalf("matchFileType(%q, %q) = %v, want %v", tt.ext, tt.fileType, got, tt.want)
		}
	}
}

func TestNewLocalBackendReportsInvalidBaseDir(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "file")
	writeTestFile(t, filepath.Dir(filePath), filepath.Base(filePath), "content")

	_, err := NewLocalBackend(filePath)
	if err == nil {
		t.Fatal("NewLocalBackend() error = nil, want error for file base path")
	}
	if !errors.Is(err, os.ErrExist) && !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("NewLocalBackend() error = %v, want directory creation error", err)
	}
}

func newTestBackend(t *testing.T, baseDir string) *LocalBackend {
	t.Helper()

	backend, err := NewLocalBackend(baseDir)
	if err != nil {
		t.Fatalf("NewLocalBackend(%q): %v", baseDir, err)
	}
	return backend
}

func writeTestFile(t *testing.T, baseDir, name, content string) {
	t.Helper()

	path := filepath.Join(baseDir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func grepLines(matches []filesystem.GrepMatch) []string {
	lines := make([]string, 0, len(matches))
	for _, match := range matches {
		lines = append(lines, matchLine(match))
	}
	return lines
}

func matchLine(match filesystem.GrepMatch) string {
	return strconvItoa(match.Line) + ":" + match.Content
}

func strconvItoa(v int) string {
	return fmt.Sprintf("%d", v)
}
