package mdiff

import (
	"strings"
	"testing"
)

func TestDiffEmpty(t *testing.T) {
	edits := Diff(nil, nil)
	if len(edits) != 0 {
		t.Errorf("expected 0 edits, got %d", len(edits))
	}
}

func TestDiffAllInsert(t *testing.T) {
	edits := Diff(nil, []string{"a", "b", "c"})
	if len(edits) != 3 {
		t.Fatalf("expected 3 edits, got %d", len(edits))
	}
	for _, e := range edits {
		if e.Kind != OpInsert {
			t.Errorf("expected OpInsert, got %d", e.Kind)
		}
	}
}

func TestDiffAllDelete(t *testing.T) {
	edits := Diff([]string{"a", "b", "c"}, nil)
	if len(edits) != 3 {
		t.Fatalf("expected 3 edits, got %d", len(edits))
	}
	for _, e := range edits {
		if e.Kind != OpDelete {
			t.Errorf("expected OpDelete, got %d", e.Kind)
		}
	}
}

func TestDiffNoChange(t *testing.T) {
	lines := []string{"a", "b", "c"}
	edits := Diff(lines, lines)
	for _, e := range edits {
		if e.Kind != OpEqual {
			t.Errorf("expected all OpEqual, got %d for %q", e.Kind, e.Text)
		}
	}
}

func TestDiffSimpleChange(t *testing.T) {
	oldLines := []string{"a", "b", "c"}
	newLines := []string{"a", "x", "c"}
	edits := Diff(oldLines, newLines)

	hasDelete := false
	hasInsert := false
	for _, e := range edits {
		if e.Kind == OpDelete && e.Text == "b" {
			hasDelete = true
		}
		if e.Kind == OpInsert && e.Text == "x" {
			hasInsert = true
		}
	}
	if !hasDelete {
		t.Error("expected delete of b")
	}
	if !hasInsert {
		t.Error("expected insert of x")
	}
}

func TestUnifiedDiffFormat(t *testing.T) {
	oldLines := []string{"line1", "line2", "line3", "line4", "line5"}
	newLines := []string{"line1", "line2", "changed", "line4", "line5"}

	fd := UnifiedDiff("a.txt", "b.txt", oldLines, newLines, 3)
	output := FormatFileDiff(fd)

	if !strings.Contains(output, "--- a.txt") {
		t.Error("missing --- header")
	}
	if !strings.Contains(output, "+++ b.txt") {
		t.Error("missing +++ header")
	}
	if !strings.Contains(output, "-line3") {
		t.Error("missing deleted line")
	}
	if !strings.Contains(output, "+changed") {
		t.Error("missing inserted line")
	}
	if !strings.Contains(output, " line2") {
		t.Error("missing context line")
	}
}

func TestParseAndApply(t *testing.T) {
	oldContent := "line1\nline2\nline3\nline4\nline5\n"
	newContent := "line1\nline2\nchanged\nline4\nline5\n"

	fd := DiffFiles("test.txt", oldContent, "test.txt", newContent, 3)
	patchStr := FormatFileDiff(fd)
	t.Logf("Patch:\n%s", patchStr)

	parsed, err := ParseFileDiff(patchStr)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(parsed.Hunks) != len(fd.Hunks) {
		t.Fatalf("hunk count mismatch: got %d, want %d", len(parsed.Hunks), len(fd.Hunks))
	}

	result, err := ApplyFileDiff(oldContent, parsed)
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if result != newContent {
		t.Errorf("apply result mismatch:\ngot:  %q\nwant: %q", result, newContent)
	}
}

func TestMultiFileDiff(t *testing.T) {
	changes := []FileChange{
		{
			Path:       "file1.go",
			OldContent: "package main\n\nfunc hello() {\n}\n",
			NewContent: "package main\n\nfunc hello() {\n\tfmt.Println(\"hello\")\n}\n",
		},
		{
			Path:       "file2.go",
			OldContent: "package util\n\nvar x = 1\n",
			NewContent: "package util\n\nvar x = 2\n",
		},
	}

	mfd := DiffMultiFiles(changes, 3)
	if len(mfd.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(mfd.Files))
	}

	output := FormatMultiFileDiff(mfd)
	t.Logf("Multi-file diff:\n%s", output)

	if !strings.Contains(output, "file1.go") {
		t.Error("missing file1.go")
	}
	if !strings.Contains(output, "file2.go") {
		t.Error("missing file2.go")
	}
}

func TestMultiFileApply(t *testing.T) {
	files := map[string]string{
		"a.go": "package a\n\nvar x = 1\n",
		"b.go": "package b\n\nfunc f() {}\n",
	}

	changes := []FileChange{
		{Path: "a.go", OldContent: files["a.go"], NewContent: "package a\n\nvar x = 2\n"},
		{Path: "b.go", OldContent: files["b.go"], NewContent: "package b\n\nfunc f() {\n\treturn\n}\n"},
	}

	mfd := DiffMultiFiles(changes, 3)
	patchStr := FormatMultiFileDiff(mfd)

	parsed, err := ParseMultiFileDiff(patchStr)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	accessor := &memAccessor{files: files}
	result := ApplyMultiFileDiff(parsed, accessor)

	if result.Failed != 0 {
		for _, r := range result.Results {
			if !r.Success {
				t.Errorf("file %s failed: %s", r.Path, r.Error)
			}
		}
	}

	if accessor.files["a.go"] != "package a\n\nvar x = 2\n" {
		t.Errorf("a.go content wrong: %q", accessor.files["a.go"])
	}
	if accessor.files["b.go"] != "package b\n\nfunc f() {\n\treturn\n}\n" {
		t.Errorf("b.go content wrong: %q", accessor.files["b.go"])
	}
}

func TestStat(t *testing.T) {
	changes := []FileChange{
		{
			Path:       "a.go",
			OldContent: "line1\nline2\nline3\n",
			NewContent: "line1\nchanged\nline3\nnew line\n",
		},
	}

	mfd := DiffMultiFiles(changes, 3)
	stat := Stat(mfd)

	if stat.FilesChanged != 1 {
		t.Errorf("expected 1 file changed, got %d", stat.FilesChanged)
	}
	if stat.Insertions < 1 {
		t.Errorf("expected at least 1 insertion, got %d", stat.Insertions)
	}
	if stat.Deletions < 1 {
		t.Errorf("expected at least 1 deletion, got %d", stat.Deletions)
	}
}

func TestNewFile(t *testing.T) {
	changes := []FileChange{
		{
			Path:       "new.go",
			OldContent: "",
			NewContent: "package newpkg\n\nfunc init() {}\n",
		},
	}

	mfd := DiffMultiFiles(changes, 3)
	patchStr := FormatMultiFileDiff(mfd)

	parsed, err := ParseMultiFileDiff(patchStr)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	accessor := &memAccessor{files: map[string]string{}}
	result := ApplyMultiFileDiff(parsed, accessor)

	if result.Failed != 0 {
		t.Errorf("expected 0 failures, got %d", result.Failed)
	}
	if !strings.Contains(accessor.files["new.go"], "package newpkg") {
		t.Errorf("new file content wrong: %q", accessor.files["new.go"])
	}
}

type memAccessor struct {
	files map[string]string
}

func (m *memAccessor) ReadFile(path string) (string, error) {
	content, ok := m.files[path]
	if !ok {
		return "", &ApplyError{File: path, Message: "file not found"}
	}
	return content, nil
}

func (m *memAccessor) WriteFile(path string, content string) error {
	m.files[path] = content
	return nil
}
