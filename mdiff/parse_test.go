package mdiff

import (
	"fmt"
	"strings"
	"testing"
)

// ============================================================
// 1. 内部函数测试 — parseFileName / parseHunkHeader / parseRange
// ============================================================

func TestParseFileNameGitPrefix(t *testing.T) {
	name := parseFileName("--- a/src/main.go", "--- ")
	if name != "src/main.go" {
		t.Errorf("expected 'src/main.go', got %q", name)
	}
	name = parseFileName("+++ b/src/main.go", "+++ ")
	if name != "src/main.go" {
		t.Errorf("expected 'src/main.go', got %q", name)
	}
}

func TestParseFileNamePathStartingWithB(t *testing.T) {
	name := parseFileName("--- a/b/config.go", "--- ")
	if name != "b/config.go" {
		t.Errorf("expected 'b/config.go', got %q", name)
	}
}

func TestParseFileNamePathStartingWithA(t *testing.T) {
	name := parseFileName("+++ b/a/util.go", "+++ ")
	if name != "a/util.go" {
		t.Errorf("expected 'a/util.go', got %q", name)
	}
}

func TestParseFileNameNoPrefix(t *testing.T) {
	name := parseFileName("--- myfile.go", "--- ")
	if name != "myfile.go" {
		t.Errorf("expected 'myfile.go', got %q", name)
	}
}

func TestParseHunkHeaderStandard(t *testing.T) {
	oldStart, oldLines, newStart, newLines, err := parseHunkHeader("@@ -1,5 +1,5 @@")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if oldStart != 1 || oldLines != 5 || newStart != 1 || newLines != 5 {
		t.Errorf("got %d,%d %d,%d; want 1,5 1,5", oldStart, oldLines, newStart, newLines)
	}
}

func TestParseHunkHeaderNoCount(t *testing.T) {
	oldStart, oldLines, newStart, newLines, err := parseHunkHeader("@@ -1 +1 @@")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if oldLines != 1 || newLines != 1 {
		t.Errorf("expected count=1 for both, got old=%d new=%d", oldLines, newLines)
	}
	if oldStart != 1 || newStart != 1 {
		t.Errorf("expected start=1 for both, got old=%d new=%d", oldStart, newStart)
	}
}

func TestParseHunkHeaderNewFile(t *testing.T) {
	oldStart, oldLines, newStart, newLines, err := parseHunkHeader("@@ -0,0 +1,3 @@")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if oldStart != 0 || oldLines != 0 || newStart != 1 || newLines != 3 {
		t.Errorf("got %d,%d %d,%d; want 0,0 1,3", oldStart, oldLines, newStart, newLines)
	}
}

func TestParseHunkHeaderWithContext(t *testing.T) {
	oldStart, oldLines, newStart, newLines, err := parseHunkHeader("@@ -10,5 +10,7 @@ func main()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if oldStart != 10 || oldLines != 5 || newStart != 10 || newLines != 7 {
		t.Errorf("got %d,%d %d,%d; want 10,5 10,7", oldStart, oldLines, newStart, newLines)
	}
}

func TestParseHunkHeaderInvalid(t *testing.T) {
	_, _, _, _, err := parseHunkHeader("@@ invalid @@")
	if err == nil {
		t.Error("expected error for invalid hunk header")
	}
}

func TestParseRange(t *testing.T) {
	tests := []struct {
		input     string
		wantStart int
		wantCount int
		wantErr   bool
	}{
		{"1,5", 1, 5, false},
		{"0,0", 0, 0, false},
		{"42", 42, 1, false},
		{"abc", 0, 0, true},
		{"1,abc", 0, 0, true},
	}
	for _, tt := range tests {
		start, count, err := parseRange(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseRange(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && (start != tt.wantStart || count != tt.wantCount) {
			t.Errorf("parseRange(%q) = %d,%d, want %d,%d",
				tt.input, start, count, tt.wantStart, tt.wantCount)
		}
	}
}

func TestSortHunksByOldStart(t *testing.T) {
	hunks := []Hunk{
		{OldStart: 20},
		{OldStart: 5},
		{OldStart: 10},
	}
	sorted := sortHunksByOldStart(hunks)
	if sorted[0].OldStart != 5 || sorted[1].OldStart != 10 || sorted[2].OldStart != 20 {
		t.Errorf("hunks not sorted: %d, %d, %d", sorted[0].OldStart, sorted[1].OldStart, sorted[2].OldStart)
	}
	if hunks[0].OldStart != 20 {
		t.Error("original slice was modified")
	}
}

func TestExtractOldLines(t *testing.T) {
	hunk := &Hunk{
		OldLines: 3,
		Lines: []DiffLine{
			{Kind: OpEqual, Text: "a"},
			{Kind: OpDelete, Text: "b"},
			{Kind: OpInsert, Text: "c"},
			{Kind: OpEqual, Text: "d"},
		},
	}
	got := extractOldLines(hunk)
	expected := []string{"a", "b", "d"}
	if strings.Join(got, ",") != strings.Join(expected, ",") {
		t.Errorf("got %v, want %v", got, expected)
	}
}

// ============================================================
// 2. ParseMultiFileDiff / ParseFileDiff 基础测试
// ============================================================

func TestParse_EmptyInput(t *testing.T) {
	cases := []string{"", " ", "\n", "\n\n\n", "\t\n "}
	for _, c := range cases {
		mfd, err := ParseMultiFileDiff(c)
		if err != nil {
			t.Errorf("empty input %q should not error: %v", c, err)
		}
		if mfd == nil || len(mfd.Files) != 0 {
			t.Errorf("empty input %q should produce 0 files", c)
		}
	}
}

func TestParseSingleFile(t *testing.T) {
	patch := "--- a.go\n+++ a.go\n@@ -1,3 +1,3 @@\n line1\n-old\n+new\n line3\n"
	fd, err := ParseFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if fd.OldName != "a.go" {
		t.Errorf("expected a.go, got %q", fd.OldName)
	}
	if len(fd.Hunks) != 1 {
		t.Errorf("expected 1 hunk, got %d", len(fd.Hunks))
	}
}

func TestParseSingleFile_Empty(t *testing.T) {
	fd, err := ParseFileDiff("")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if fd == nil {
		t.Fatal("should return empty FileDiff, not nil")
	}
}

func TestParseAndApply(t *testing.T) {
	oldContent := "line1\nline2\nline3\nline4\nline5\n"
	newContent := "line1\nline2\nchanged\nline4\nline5\n"

	fd := DiffFiles("test.txt", oldContent, "test.txt", newContent, 3)
	patchStr := FormatFileDiff(fd)

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

// ============================================================
// 3. 文件名解析测试
// ============================================================

func TestParse_FileNameWithSpaces(t *testing.T) {
	patch := "--- my file.go\n+++ my file.go\n@@ -1,1 +1,1 @@\n-old\n+new\n"
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if mfd.Files[0].OldName != "my file.go" {
		t.Errorf("expected 'my file.go', got %q", mfd.Files[0].OldName)
	}
}

func TestParse_FileNameWithTimestamp(t *testing.T) {
	patch := "--- a/file.go\t2024-01-01 00:00:00.000000000 +0000\n+++ b/file.go\t2024-01-01 00:00:00.000000000 +0000\n@@ -1,1 +1,1 @@\n-old\n+new\n"
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if mfd.Files[0].OldName != "file.go" {
		t.Errorf("expected 'file.go', got %q", mfd.Files[0].OldName)
	}
}

func TestParse_FilePath_DeepNested(t *testing.T) {
	patch := "--- src/components/ui/Button/Button.tsx\n+++ src/components/ui/Button/Button.tsx\n@@ -1,1 +1,1 @@\n-old\n+new\n"
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if mfd.Files[0].OldName != "src/components/ui/Button/Button.tsx" {
		t.Errorf("deep path not preserved: %q", mfd.Files[0].OldName)
	}
}

// ============================================================
// 4. Hunk 解析边界测试
// ============================================================

func TestParse_OnlyHunkHeader_NoBody(t *testing.T) {
	patch := "--- a.go\n+++ a.go\n@@ -1,0 +1,0 @@\n"
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(mfd.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(mfd.Files))
	}
	if len(mfd.Files[0].Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(mfd.Files[0].Hunks))
	}
	if len(mfd.Files[0].Hunks[0].Lines) != 0 {
		t.Errorf("expected 0 lines in hunk, got %d", len(mfd.Files[0].Hunks[0].Lines))
	}
}

func TestParse_HunkHeaderWithFunctionContext(t *testing.T) {
	patch := "--- a.go\n+++ a.go\n@@ -10,3 +10,3 @@ func main() {\n line10\n-old\n+new\n line12\n"
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	h := &mfd.Files[0].Hunks[0]
	if h.OldStart != 10 || h.NewStart != 10 {
		t.Errorf("hunk start wrong: old=%d new=%d", h.OldStart, h.NewStart)
	}
	if len(h.Lines) != 4 {
		t.Errorf("expected 4 lines, got %d", len(h.Lines))
	}
}

func TestParse_SingleLineHunk(t *testing.T) {
	patch := "--- a.go\n+++ a.go\n@@ -5 +5 @@\n-old\n+new\n"
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	h := &mfd.Files[0].Hunks[0]
	if h.OldLines != 1 || h.NewLines != 1 {
		t.Errorf("expected 1,1 got %d,%d", h.OldLines, h.NewLines)
	}
}

func TestParse_ZeroLineHunk_NewFile(t *testing.T) {
	patch := "--- /dev/null\n+++ new.go\n@@ -0,0 +1,3 @@\n+line1\n+line2\n+line3\n"
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if mfd.Files[0].OldName != "/dev/null" {
		t.Errorf("OldName should be /dev/null, got %q", mfd.Files[0].OldName)
	}
	h := &mfd.Files[0].Hunks[0]
	if h.OldLines != 0 {
		t.Errorf("old lines should be 0, got %d", h.OldLines)
	}
	insertCount := 0
	for _, l := range h.Lines {
		if l.Kind == OpInsert {
			insertCount++
		}
	}
	if insertCount != 3 {
		t.Errorf("expected 3 insert lines, got %d", insertCount)
	}
}

func TestParse_NoNewlineAtEnd(t *testing.T) {
	patch := "--- a.go\n+++ a.go\n@@ -1,3 +1,3 @@\n line1\n-old\n+new\n line3\n\\ No newline at end of file\n"
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	h := &mfd.Files[0].Hunks[0]
	for _, l := range h.Lines {
		if strings.Contains(l.Text, "No newline") {
			t.Error("'No newline at end of file' should be skipped, not stored as a line")
		}
	}
	if len(h.Lines) != 4 {
		t.Errorf("expected 4 lines (not counting no-newline marker), got %d", len(h.Lines))
	}
}

func TestParse_EmptyLinesInHunk(t *testing.T) {
	patch := "--- a.py\n+++ a.py\n@@ -1,5 +1,5 @@\n def foo():\n     pass\n \n-def bar():\n+def baz():\n     pass\n"
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	h := &mfd.Files[0].Hunks[0]
	if len(h.Lines) < 5 {
		t.Errorf("expected at least 5 lines (including empty context), got %d", len(h.Lines))
	}
	found := false
	for _, l := range h.Lines {
		if l.Kind == OpEqual && l.Text == "" {
			found = true
			break
		}
	}
	if !found {
		t.Error("empty context line not found")
	}
}

func TestParseHunkEmptyLineAsDelete(t *testing.T) {
	patchText := "--- test.py\n+++ test.py\n@@ -1,4 +1,1 @@\n keep\n-removed1\n\n-removed2\n"

	fd, err := ParseFileDiff(patchText)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(fd.Hunks) == 0 {
		t.Fatal("expected at least 1 hunk")
	}

	h := fd.Hunks[0]
	totalOld := 0
	for _, dl := range h.Lines {
		if dl.Kind == OpEqual || dl.Kind == OpDelete {
			totalOld++
		}
	}
	if totalOld != 4 {
		t.Errorf("expected 4 old lines consumed, got %d", totalOld)
	}
}

func TestParse_OnlyDash_Line(t *testing.T) {
	patch := "--- a.md\n+++ a.md\n@@ -1,3 +1,3 @@\n # Title\n-old content\n+new content\n ---\n"
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	h := &mfd.Files[0].Hunks[0]
	if len(h.Lines) < 3 {
		t.Errorf("expected >= 3 lines, got %d", len(h.Lines))
	}
	found := false
	for _, l := range h.Lines {
		if l.Kind == OpEqual && l.Text == "---" {
			found = true
		}
	}
	if !found {
		t.Error("context line '---' not found")
	}
}

// ============================================================
// 5. 多文件/多 Hunk 解析测试
// ============================================================

func TestParse_ConsecutiveHunksInOneFile(t *testing.T) {
	var hunks []string
	for i := 0; i < 5; i++ {
		start := i*20 + 1
		hunks = append(hunks, fmt.Sprintf("@@ -%d,3 +%d,3 @@\n ctx%d_1\n-old%d\n+new%d\n ctx%d_2", start, start, i, i, i, i))
	}
	patch := "--- big.go\n+++ big.go\n" + strings.Join(hunks, "\n") + "\n"

	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(mfd.Files[0].Hunks) != 5 {
		t.Errorf("expected 5 hunks, got %d", len(mfd.Files[0].Hunks))
	}
}

func TestParse_MultiFile_10Files(t *testing.T) {
	var parts []string
	for i := 0; i < 10; i++ {
		parts = append(parts, fmt.Sprintf("--- file%d.go\n+++ file%d.go\n@@ -1,1 +1,1 @@\n-old%d\n+new%d", i, i, i, i))
	}
	patch := strings.Join(parts, "\n") + "\n"

	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(mfd.Files) != 10 {
		t.Errorf("expected 10 files, got %d", len(mfd.Files))
	}
}

func TestParse_MultiFile_EmptyLineBetweenFiles(t *testing.T) {
	patch := `--- file1.go
+++ file1.go
@@ -1,3 +1,3 @@
 package main
-var x = 1
+var x = 2
 func main() {}

--- file2.go
+++ file2.go
@@ -1,3 +1,3 @@
 package util
-var y = 1
+var y = 2
 func helper() {}
`
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(mfd.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(mfd.Files))
		for i, f := range mfd.Files {
			t.Logf("file[%d]: old=%s new=%s hunks=%d", i, f.OldName, f.NewName, len(f.Hunks))
		}
	}
}

func TestParse_MultipleHunksInOneFile_ThenSecondFile(t *testing.T) {
	patch := `--- a.go
+++ a.go
@@ -1,3 +1,3 @@
 line1
-old1
+new1
 line3
@@ -10,3 +10,3 @@
 line10
-old10
+new10
 line12
--- b.go
+++ b.go
@@ -1,2 +1,2 @@
-hello
+world
 end
`
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(mfd.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(mfd.Files))
	}
	if len(mfd.Files) >= 1 && len(mfd.Files[0].Hunks) != 2 {
		t.Errorf("file1 should have 2 hunks, got %d", len(mfd.Files[0].Hunks))
	}
	if len(mfd.Files) >= 2 && len(mfd.Files[1].Hunks) != 1 {
		t.Errorf("file2 should have 1 hunk, got %d", len(mfd.Files[1].Hunks))
	}
}

func TestParseHunkEarlyTerminateOnFileSeparator(t *testing.T) {
	patchText := "--- a.go\n+++ a.go\n@@ -1,5 +1,5 @@\n line1\n-old\n+new\n line3\n--- b.go\n+++ b.go\n@@ -1,3 +1,3 @@\n line1\n-old\n+new\n line3\n"

	mfd, err := ParseMultiFileDiff(patchText)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(mfd.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(mfd.Files))
	}
}

// ============================================================
// 6. Hunk 行计数不准确 —— 优雅容错
// ============================================================

func TestParse_HunkCountsExceed_GracefulHandling(t *testing.T) {
	patch := `--- a.go
+++ a.go
@@ -1,10 +1,10 @@
 line1
-old
+new
 line3
--- b.go
+++ b.go
@@ -1,1 +1,1 @@
-hello
+world
`
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(mfd.Files) != 2 {
		t.Errorf("expected 2 files (should gracefully handle count mismatch), got %d", len(mfd.Files))
		for i, f := range mfd.Files {
			t.Logf("  file[%d] old=%q new=%q hunks=%d", i, f.OldName, f.NewName, len(f.Hunks))
		}
	}
}

func TestParse_HunkCountOvershoot_EatsNextFile(t *testing.T) {
	patch := `--- file1.go
+++ file1.go
@@ -1,5 +1,5 @@
 line1
-old
+new
 line3
--- file2.go
+++ file2.go
@@ -1,3 +1,3 @@
 package a
-var x = 1
+var x = 2
 end
`
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(mfd.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(mfd.Files))
		for i, f := range mfd.Files {
			t.Logf("file[%d]: old=%s new=%s hunks=%d", i, f.OldName, f.NewName, len(f.Hunks))
		}
	}
}

func TestParse_WrongHunkCounts(t *testing.T) {
	patch := `--- test.go
+++ test.go
@@ -1,3 +1,4 @@
 func main() {
+    fmt.Println("hello")
+    fmt.Println("world")
 }
`
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	h := &mfd.Files[0].Hunks[0]
	oldCount, newCount := 0, 0
	for _, l := range h.Lines {
		switch l.Kind {
		case OpEqual:
			oldCount++
			newCount++
		case OpDelete:
			oldCount++
		case OpInsert:
			newCount++
		}
	}
	t.Logf("Header old=%d new=%d, actual old=%d new=%d", h.OldLines, h.NewLines, oldCount, newCount)
	if len(h.Lines) < 3 {
		t.Errorf("hunk should have at least 3 lines, got %d", len(h.Lines))
	}
}

// ============================================================
// 7. git diff 扩展格式测试
// ============================================================

func TestParse_GitDiffWithMode(t *testing.T) {
	patch := `diff --git a/cmd/main.go b/cmd/main.go
old mode 100644
new mode 100755
index abc1234..def5678 100644
--- a/cmd/main.go
+++ b/cmd/main.go
@@ -1,3 +1,3 @@
 package main
-var x = 1
+var x = 2
 func main() {}
`
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(mfd.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(mfd.Files))
	}
	if mfd.Files[0].OldName != "cmd/main.go" {
		t.Errorf("expected 'cmd/main.go', got %q", mfd.Files[0].OldName)
	}
}

func TestParse_GitDiffHeader(t *testing.T) {
	patch := `diff --git a/file1.go b/file1.go
index abc123..def456 100644
--- a/file1.go
+++ b/file1.go
@@ -1,3 +1,3 @@
 line1
-old
+new
 line3
diff --git a/file2.go b/file2.go
--- a/file2.go
+++ b/file2.go
@@ -1,2 +1,2 @@
-hello
+world
 end
`
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(mfd.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(mfd.Files))
	}
	if len(mfd.Files) > 0 && mfd.Files[0].OldName != "file1.go" {
		t.Errorf("file1 OldName: got %q, want %q", mfd.Files[0].OldName, "file1.go")
	}
}

// ============================================================
// 8. 换行符 / CRLF 处理测试
// ============================================================

func TestParse_MixedCRLFandLF(t *testing.T) {
	patch := "--- a.go\r\n+++ a.go\n@@ -1,2 +1,2 @@\r\n-old\n+new\r\n end\n"
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(mfd.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(mfd.Files))
	}
	for _, l := range mfd.Files[0].Hunks[0].Lines {
		if strings.Contains(l.Text, "\r") {
			t.Errorf("line should not contain CR: %q", l.Text)
		}
	}
}

func TestParse_WindowsCRLF(t *testing.T) {
	patch := "--- file.go\r\n+++ file.go\r\n@@ -1,3 +1,3 @@\r\n line1\r\n-old\r\n+new\r\n line3\r\n"
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(mfd.Files) != 1 {
		t.Errorf("expected 1 file, got %d", len(mfd.Files))
	}
	if len(mfd.Files) > 0 && len(mfd.Files[0].Hunks) > 0 {
		h := &mfd.Files[0].Hunks[0]
		for _, l := range h.Lines {
			if strings.HasSuffix(l.Text, "\r") {
				t.Errorf("line has trailing CR: %q", l.Text)
			}
		}
	}
}

// ============================================================
// 9. LLM 特有的解析容错测试
// ============================================================

func TestParse_LLM_MissingSpacePrefix_MultipleHunks(t *testing.T) {
	patch := "--- a.go\n+++ a.go\n@@ -1,3 +1,3 @@\nline1\n-old1\n+new1\nline3\n@@ -10,3 +10,3 @@\nline10\n-old11\n+new11\nline12\n"
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(mfd.Files[0].Hunks) != 2 {
		t.Errorf("expected 2 hunks, got %d", len(mfd.Files[0].Hunks))
	}
	for i, h := range mfd.Files[0].Hunks {
		if len(h.Lines) < 3 {
			t.Errorf("hunk[%d] should have >= 3 lines, got %d", i, len(h.Lines))
		}
	}
}

func TestParse_LLM_TabPrefix_MultiFile(t *testing.T) {
	patch := "--- a.go\n+++ a.go\n@@ -1,3 +1,3 @@\n\tpackage a\n-\tvar x = 1\n+\tvar x = 2\n\tfunc main() {}\n--- b.go\n+++ b.go\n@@ -1,3 +1,3 @@\n\tpackage b\n-\tvar y = 1\n+\tvar y = 2\n\tfunc helper() {}\n"
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(mfd.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(mfd.Files))
	}
	for fi, f := range mfd.Files {
		if len(f.Hunks) == 0 {
			t.Errorf("file[%d] has no hunks", fi)
			continue
		}
		if len(f.Hunks[0].Lines) < 3 {
			t.Errorf("file[%d] hunk should have >= 3 lines, got %d", fi, len(f.Hunks[0].Lines))
		}
	}
}

func TestParse_LLM_CodeFenceVariants(t *testing.T) {
	variants := []string{
		"```diff\n--- a.go\n+++ a.go\n@@ -1,1 +1,1 @@\n-old\n+new\n```",
		"```\n--- a.go\n+++ a.go\n@@ -1,1 +1,1 @@\n-old\n+new\n```",
		"```diff\n--- a.go\n+++ a.go\n@@ -1,1 +1,1 @@\n-old\n+new\n```\n",
	}
	for i, patch := range variants {
		mfd, err := ParseMultiFileDiff(patch)
		if err != nil {
			t.Errorf("variant[%d] parse error: %v", i, err)
			continue
		}
		if len(mfd.Files) != 1 {
			t.Errorf("variant[%d] expected 1 file, got %d", i, len(mfd.Files))
		}
	}
}

func TestParse_CodeFenceWrapped(t *testing.T) {
	patch := "```diff\n--- file.go\n+++ file.go\n@@ -1,3 +1,3 @@\n line1\n-old\n+new\n line3\n```"
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(mfd.Files) != 1 {
		t.Errorf("expected 1 file (code fence should be skipped), got %d", len(mfd.Files))
	}
}

func TestParse_TabIndentedContext(t *testing.T) {
	patch := "--- file.go\n+++ file.go\n@@ -1,3 +1,3 @@\n\tline1\n-\told\n+\tnew\n\tline3\n"
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(mfd.Files) == 0 {
		t.Fatal("expected 1 file")
	}
	h := &mfd.Files[0].Hunks[0]
	if len(h.Lines) < 3 {
		t.Errorf("tab-indented context lines were lost: got %d lines, want >= 3", len(h.Lines))
		for _, l := range h.Lines {
			t.Logf("  [%d] %q", l.Kind, l.Text)
		}
	}
}

func TestParse_NoSpacePrefixContext(t *testing.T) {
	patch := "--- file.go\n+++ file.go\n@@ -1,3 +1,3 @@\nline1\n-old\n+new\nline3\n"
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	h := &mfd.Files[0].Hunks[0]
	if len(h.Lines) < 3 {
		t.Errorf("no-space-prefix context lines were lost: got %d lines, want >= 3", len(h.Lines))
		for _, l := range h.Lines {
			t.Logf("  [%d] %q", l.Kind, l.Text)
		}
	}
}

func TestLLM_MissingTrailingNewline(t *testing.T) {
	patch := "--- file.go\n+++ file.go\n@@ -1,2 +1,2 @@\n-old\n+new\n end"
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(mfd.Files) != 1 {
		t.Errorf("expected 1 file, got %d", len(mfd.Files))
	}
	if len(mfd.Files[0].Hunks) > 0 {
		h := &mfd.Files[0].Hunks[0]
		if len(h.Lines) < 2 {
			t.Errorf("hunk should have at least 2 lines, got %d", len(h.Lines))
		}
	}
}
