package mdiff

import (
	"fmt"
	"strings"
	"testing"
)

// ============================================================
// UnifiedDiff & Format 测试
// ============================================================

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

func TestUnifiedDiffNoChange(t *testing.T) {
	lines := []string{"a", "b", "c"}
	fd := UnifiedDiff("f.txt", "f.txt", lines, lines, 3)
	output := FormatFileDiff(fd)
	if output != "" {
		t.Errorf("expected empty output for no change, got %q", output)
	}
}

func TestUnifiedDiffContextLines(t *testing.T) {
	var oldLines, newLines []string
	for i := 0; i < 20; i++ {
		oldLines = append(oldLines, fmt.Sprintf("line%d", i+1))
		newLines = append(newLines, fmt.Sprintf("line%d", i+1))
	}
	newLines[10] = "changed"

	fd := UnifiedDiff("f.txt", "f.txt", oldLines, newLines, 1)
	output := FormatFileDiff(fd)

	if strings.Contains(output, " line8") {
		t.Error("context line8 should not appear with contextLines=1")
	}
	if !strings.Contains(output, " line10") {
		t.Error("context line10 should appear")
	}
}

func TestUnifiedDiffMultipleHunks(t *testing.T) {
	var oldLines, newLines []string
	for i := 0; i < 30; i++ {
		oldLines = append(oldLines, fmt.Sprintf("line%d", i+1))
		newLines = append(newLines, fmt.Sprintf("line%d", i+1))
	}
	newLines[2] = "changed1"
	newLines[25] = "changed2"

	fd := UnifiedDiff("f.txt", "f.txt", oldLines, newLines, 3)
	if len(fd.Hunks) != 2 {
		t.Errorf("expected 2 hunks, got %d", len(fd.Hunks))
	}
}

// ============================================================
// FormatFileDiff / FormatMultiFileDiff / formatFileName 测试
// ============================================================

func TestFormatFileDiffNil(t *testing.T) {
	if FormatFileDiff(nil) != "" {
		t.Error("expected empty string for nil FileDiff")
	}
}

func TestFormatFileDiffEmptyHunks(t *testing.T) {
	fd := &FileDiff{OldName: "a.txt", NewName: "b.txt"}
	if FormatFileDiff(fd) != "" {
		t.Error("expected empty string for FileDiff with no hunks")
	}
}

func TestFormatMultiFileDiffNil(t *testing.T) {
	if FormatMultiFileDiff(nil) != "" {
		t.Error("expected empty string for nil MultiFileDiff")
	}
}

func TestFormatFileName(t *testing.T) {
	if formatFileName("") != "/dev/null" {
		t.Error("expected /dev/null for empty name")
	}
	if formatFileName("test.go") != "test.go" {
		t.Error("expected pass-through for normal name")
	}
}

// ============================================================
// 新文件 / 删除文件 Hunk Header 测试
// ============================================================

func TestNewFileHunkHeader(t *testing.T) {
	fd := DiffFiles("/dev/null", "", "new.go", "package main\n\nfunc main() {}\n", 3)
	output := FormatFileDiff(fd)

	if !strings.Contains(output, "@@ -0,0 +1,") {
		t.Errorf("new file hunk should have OldStart=0, got:\n%s", output)
	}
}

func TestDeleteFileHunkHeader(t *testing.T) {
	fd := DiffFiles("old.go", "package main\n\nfunc main() {}\n", "/dev/null", "", 3)
	output := FormatFileDiff(fd)

	if !strings.Contains(output, "+0,0 @@") {
		t.Errorf("delete file hunk should have NewStart=0, got:\n%s", output)
	}
}

func TestDeleteFile(t *testing.T) {
	changes := []FileChange{
		{
			Path:       "old.go",
			OldContent: "package old\n\nvar x = 1\n",
			NewContent: "",
		},
	}

	mfd := DiffMultiFiles(changes, 3)

	if len(mfd.Files) != 1 {
		t.Fatalf("expected 1 file diff, got %d", len(mfd.Files))
	}
	if mfd.Files[0].NewName != "/dev/null" {
		t.Errorf("expected NewName=/dev/null, got %q", mfd.Files[0].NewName)
	}

	for _, h := range mfd.Files[0].Hunks {
		for _, dl := range h.Lines {
			if dl.Kind == OpInsert {
				t.Error("delete file diff should not have insert lines")
			}
		}
	}
}

// ============================================================
// 统计信息测试
// ============================================================

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

func TestStatEmpty(t *testing.T) {
	stat := Stat(&MultiFileDiff{})
	if stat.FilesChanged != 0 || stat.Insertions != 0 || stat.Deletions != 0 {
		t.Error("expected all-zero stat for empty diff")
	}
}

func TestDiffStatString(t *testing.T) {
	tests := []struct {
		stat DiffStat
		want string
	}{
		{DiffStat{1, 1, 0}, "1 file changed, 1 insertion(+)"},
		{DiffStat{2, 3, 1}, "2 files changed, 3 insertions(+), 1 deletion(-)"},
		{DiffStat{1, 0, 5}, "1 file changed, 5 deletions(-)"},
		{DiffStat{0, 0, 0}, "0 files changed"},
	}
	for _, tt := range tests {
		got := tt.stat.String()
		if got != tt.want {
			t.Errorf("DiffStat%+v.String() = %q, want %q", tt.stat, got, tt.want)
		}
	}
}

func TestPluralize(t *testing.T) {
	if pluralize(1, "file", "files") != "1 file" {
		t.Error("expected singular")
	}
	if pluralize(3, "file", "files") != "3 files" {
		t.Error("expected plural")
	}
	if pluralize(0, "file", "files") != "0 files" {
		t.Error("expected plural for 0")
	}
}
