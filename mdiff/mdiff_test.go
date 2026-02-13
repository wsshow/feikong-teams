package mdiff

import (
	"fmt"
	"strings"
	"testing"
)

// ============================================================
// Diff 算法测试
// ============================================================

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

func TestDiffSingleLine(t *testing.T) {
	edits := Diff([]string{"old"}, []string{"new"})

	hasDelete := false
	hasInsert := false
	for _, e := range edits {
		if e.Kind == OpDelete && e.Text == "old" {
			hasDelete = true
		}
		if e.Kind == OpInsert && e.Text == "new" {
			hasInsert = true
		}
	}
	if !hasDelete || !hasInsert {
		t.Error("expected delete of 'old' and insert of 'new'")
	}
}

func TestDiffInsertAtBeginning(t *testing.T) {
	oldLines := []string{"b", "c"}
	newLines := []string{"a", "b", "c"}
	edits := Diff(oldLines, newLines)

	if edits[0].Kind != OpInsert || edits[0].Text != "a" {
		t.Error("expected insert 'a' at beginning")
	}
}

func TestDiffInsertAtEnd(t *testing.T) {
	oldLines := []string{"a", "b"}
	newLines := []string{"a", "b", "c"}
	edits := Diff(oldLines, newLines)

	lastEdit := edits[len(edits)-1]
	if lastEdit.Kind != OpInsert || lastEdit.Text != "c" {
		t.Error("expected insert 'c' at end")
	}
}

func TestDiffDeleteAtBeginning(t *testing.T) {
	oldLines := []string{"a", "b", "c"}
	newLines := []string{"b", "c"}
	edits := Diff(oldLines, newLines)

	if edits[0].Kind != OpDelete || edits[0].Text != "a" {
		t.Error("expected delete 'a' at beginning")
	}
}

func TestDiffDeleteAtEnd(t *testing.T) {
	oldLines := []string{"a", "b", "c"}
	newLines := []string{"a", "b"}
	edits := Diff(oldLines, newLines)

	lastNonEqual := edits[len(edits)-1]
	for i := len(edits) - 1; i >= 0; i-- {
		if edits[i].Kind != OpEqual {
			lastNonEqual = edits[i]
			break
		}
	}
	if lastNonEqual.Kind != OpDelete || lastNonEqual.Text != "c" {
		t.Error("expected delete 'c' at end")
	}
}

func TestDiffCompleteReplacement(t *testing.T) {
	oldLines := []string{"a", "b", "c"}
	newLines := []string{"x", "y", "z"}
	edits := Diff(oldLines, newLines)

	deletes := 0
	inserts := 0
	for _, e := range edits {
		switch e.Kind {
		case OpDelete:
			deletes++
		case OpInsert:
			inserts++
		}
	}
	if deletes != 3 || inserts != 3 {
		t.Errorf("expected 3 deletes and 3 inserts, got %d/%d", deletes, inserts)
	}
}

func TestDiffMultipleChanges(t *testing.T) {
	// 多处分散变更
	oldLines := []string{"a", "b", "c", "d", "e", "f", "g"}
	newLines := []string{"a", "B", "c", "d", "E", "f", "g"}
	edits := Diff(oldLines, newLines)

	changes := 0
	for _, e := range edits {
		if e.Kind != OpEqual {
			changes++
		}
	}
	// 应有 b→B 和 e→E 的变更（各 1 delete + 1 insert）
	if changes != 4 {
		t.Errorf("expected 4 non-equal edits, got %d", changes)
	}
}

func TestDiffWithEmptyLines(t *testing.T) {
	oldLines := []string{"a", "", "b"}
	newLines := []string{"a", "", "c"}
	edits := Diff(oldLines, newLines)

	hasDelete := false
	hasInsert := false
	for _, e := range edits {
		if e.Kind == OpDelete && e.Text == "b" {
			hasDelete = true
		}
		if e.Kind == OpInsert && e.Text == "c" {
			hasInsert = true
		}
	}
	if !hasDelete || !hasInsert {
		t.Error("expected change from 'b' to 'c'")
	}
}

func TestDiffIdenticalSingleLine(t *testing.T) {
	edits := Diff([]string{"same"}, []string{"same"})
	if len(edits) != 1 || edits[0].Kind != OpEqual {
		t.Error("expected single OpEqual edit")
	}
}

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
	// 使用足够长的文件来验证上下文行数
	var oldLines, newLines []string
	for i := 0; i < 20; i++ {
		oldLines = append(oldLines, fmt.Sprintf("line%d", i+1))
		newLines = append(newLines, fmt.Sprintf("line%d", i+1))
	}
	newLines[10] = "changed" // 修改第11行

	// 只要 1 行上下文
	fd := UnifiedDiff("f.txt", "f.txt", oldLines, newLines, 1)
	output := FormatFileDiff(fd)

	// 上下文应只包含 line10 和 line12，不包含 line8
	if strings.Contains(output, " line8") {
		t.Error("context line8 should not appear with contextLines=1")
	}
	if !strings.Contains(output, " line10") {
		t.Error("context line10 should appear")
	}
}

func TestUnifiedDiffMultipleHunks(t *testing.T) {
	// 两处修改相距较远，应生成两个独立 hunk
	var oldLines, newLines []string
	for i := 0; i < 30; i++ {
		oldLines = append(oldLines, fmt.Sprintf("line%d", i+1))
		newLines = append(newLines, fmt.Sprintf("line%d", i+1))
	}
	newLines[2] = "changed1"  // 第3行
	newLines[25] = "changed2" // 第26行

	fd := UnifiedDiff("f.txt", "f.txt", oldLines, newLines, 3)
	if len(fd.Hunks) != 2 {
		t.Errorf("expected 2 hunks, got %d", len(fd.Hunks))
	}
}

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

func TestNewFileHunkHeader(t *testing.T) {
	// 新文件应生成 @@ -0,0 +1,N @@
	fd := DiffFiles("/dev/null", "", "new.go", "package main\n\nfunc main() {}\n", 3)
	output := FormatFileDiff(fd)

	if !strings.Contains(output, "@@ -0,0 +1,") {
		t.Errorf("new file hunk should have OldStart=0, got:\n%s", output)
	}
}

func TestDeleteFileHunkHeader(t *testing.T) {
	// 删除文件应生成 @@ -1,N +0,0 @@
	fd := DiffFiles("old.go", "package main\n\nfunc main() {}\n", "/dev/null", "", 3)
	output := FormatFileDiff(fd)

	if !strings.Contains(output, "+0,0 @@") {
		t.Errorf("delete file hunk should have NewStart=0, got:\n%s", output)
	}
}

// ============================================================
// Parse 测试
// ============================================================

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

func TestParseEmpty(t *testing.T) {
	mfd, err := ParseMultiFileDiff("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mfd.Files) != 0 {
		t.Errorf("expected 0 files, got %d", len(mfd.Files))
	}
}

func TestParseWhitespaceOnly(t *testing.T) {
	mfd, err := ParseMultiFileDiff("   \n  \n  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mfd.Files) != 0 {
		t.Errorf("expected 0 files, got %d", len(mfd.Files))
	}
}

func TestParseSingleFileDiffEmpty(t *testing.T) {
	fd, err := ParseFileDiff("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fd == nil {
		t.Fatal("expected non-nil FileDiff")
	}
}

func TestParseFileNameGitPrefix(t *testing.T) {
	// --- a/path/to/file 应正确提取为 path/to/file
	name := parseFileName("--- a/src/main.go", "--- ")
	if name != "src/main.go" {
		t.Errorf("expected 'src/main.go', got %q", name)
	}

	// +++ b/path/to/file 应正确提取
	name = parseFileName("+++ b/src/main.go", "+++ ")
	if name != "src/main.go" {
		t.Errorf("expected 'src/main.go', got %q", name)
	}
}

func TestParseFileNamePathStartingWithB(t *testing.T) {
	// --- a/b/config.go 应提取为 b/config.go（不应被误剥离）
	name := parseFileName("--- a/b/config.go", "--- ")
	if name != "b/config.go" {
		t.Errorf("expected 'b/config.go', got %q", name)
	}
}

func TestParseFileNamePathStartingWithA(t *testing.T) {
	// +++ b/a/util.go 应提取为 a/util.go（不应被误剥离）
	name := parseFileName("+++ b/a/util.go", "+++ ")
	if name != "a/util.go" {
		t.Errorf("expected 'a/util.go', got %q", name)
	}
}

func TestParseFileNameWithTimestamp(t *testing.T) {
	name := parseFileName("--- a/file.go\t2024-01-01 00:00:00", "--- ")
	if name != "file.go" {
		t.Errorf("expected 'file.go', got %q", name)
	}
}

func TestParseFileNameNoPrefix(t *testing.T) {
	// 无 a/b/ 前缀（mdiff 自身生成的格式）
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
	// 省略行数时默认为 1
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
	// 带函数名上下文
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

// ============================================================
// Patch 应用测试
// ============================================================

func TestApplyFileDiffNil(t *testing.T) {
	result, err := ApplyFileDiff("content", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "content" {
		t.Errorf("expected unchanged content, got %q", result)
	}
}

func TestApplyFileDiffEmptyHunks(t *testing.T) {
	fd := &FileDiff{OldName: "test.go", NewName: "test.go"}
	result, err := ApplyFileDiff("content", fd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "content" {
		t.Errorf("expected unchanged content, got %q", result)
	}
}

func TestApplyFileDiffPreservesTrailingNewline(t *testing.T) {
	oldContent := "line1\nline2\n"
	newContent := "line1\nchanged\n"

	fd := DiffFiles("f.txt", oldContent, "f.txt", newContent, 3)
	result, err := ApplyFileDiff(oldContent, fd)
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if result != newContent {
		t.Errorf("got %q, want %q", result, newContent)
	}
	// 确保保留尾部换行
	if result[len(result)-1] != '\n' {
		t.Error("trailing newline lost")
	}
}

func TestApplyFileDiffNoTrailingNewline(t *testing.T) {
	oldContent := "line1\nline2"
	newContent := "line1\nchanged"

	fd := DiffFiles("f.txt", oldContent, "f.txt", newContent, 3)
	result, err := ApplyFileDiff(oldContent, fd)
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if result != newContent {
		t.Errorf("got %q, want %q", result, newContent)
	}
}

func TestApplyHunksFuzzyMatch(t *testing.T) {
	// 模拟行号偏移的情况——hunk 记录的行号与实际不符
	lines := []string{"a", "b", "old", "d", "e"}
	hunks := []Hunk{
		{
			OldStart: 10, // 故意设错行号，触发模糊匹配
			OldLines: 3,
			NewStart: 10,
			NewLines: 3,
			Lines: []DiffLine{
				{Kind: OpEqual, Text: "b"},
				{Kind: OpDelete, Text: "old"},
				{Kind: OpInsert, Text: "new"},
				{Kind: OpEqual, Text: "d"},
			},
		},
	}
	result, err := ApplyHunks(lines, hunks)
	if err != nil {
		t.Fatalf("fuzzy match failed: %v", err)
	}
	expected := []string{"a", "b", "new", "d", "e"}
	if strings.Join(result, "\n") != strings.Join(expected, "\n") {
		t.Errorf("got %v, want %v", result, expected)
	}
}

func TestApplyHunksContextMismatch(t *testing.T) {
	lines := []string{"a", "b", "c"}
	hunks := []Hunk{
		{
			OldStart: 1,
			OldLines: 1,
			NewStart: 1,
			NewLines: 1,
			Lines: []DiffLine{
				{Kind: OpDelete, Text: "nonexistent_line"},
				{Kind: OpInsert, Text: "new"},
			},
		},
	}
	_, err := ApplyHunks(lines, hunks)
	if err == nil {
		t.Fatal("expected error for context mismatch")
	}
	ae, ok := err.(*ApplyError)
	if !ok {
		t.Fatalf("expected *ApplyError, got %T", err)
	}
	// 验证错误信息包含有用的诊断信息
	if !strings.Contains(ae.Message, "context mismatch") {
		t.Errorf("error message should contain 'context mismatch': %s", ae.Message)
	}
	if !strings.Contains(ae.Message, "3 lines") {
		t.Errorf("error message should contain file line count: %s", ae.Message)
	}
}

func TestApplyErrorFormat(t *testing.T) {
	ae := &ApplyError{
		File:    "test.go",
		HunkIdx: 2,
		Message: "context mismatch",
	}
	expected := "patch test.go hunk #3: context mismatch"
	if ae.Error() != expected {
		t.Errorf("got %q, want %q", ae.Error(), expected)
	}
}

func TestMatchAt(t *testing.T) {
	lines := []string{"a", "b", "c", "d"}

	// 正常匹配
	if !matchAt(lines, []string{"b", "c"}, 1) {
		t.Error("expected match at pos 1")
	}
	// 越界
	if matchAt(lines, []string{"c", "d"}, 3) {
		t.Error("expected no match at out-of-bounds pos")
	}
	// 负数位置
	if matchAt(lines, []string{"a"}, -1) {
		t.Error("expected no match at negative pos")
	}
	// 空 pattern 总是匹配
	if !matchAt(lines, nil, 0) {
		t.Error("expected match for empty pattern")
	}
	// 不匹配内容
	if matchAt(lines, []string{"x"}, 0) {
		t.Error("expected no match for wrong content")
	}
}

// ============================================================
// PatchText 测试
// ============================================================

func TestPatchText(t *testing.T) {
	original := "line1\nline2\nline3\n"
	expected := "line1\nline2\nchanged\n"

	fd := DiffFiles("f.txt", original, "f.txt", expected, 3)
	patchStr := FormatFileDiff(fd)

	result, err := PatchText(original, patchStr)
	if err != nil {
		t.Fatalf("PatchText error: %v", err)
	}
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestPatchTextEmpty(t *testing.T) {
	result, err := PatchText("original content", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "original content" {
		t.Errorf("expected unchanged content, got %q", result)
	}
}

func TestPatchTextInvalidPatch(t *testing.T) {
	_, err := PatchText("content", "@@ invalid hunk @@\n-old\n+new\n")
	// 无效的 patch 应该返回错误或空结果（取决于解析容错度）
	// 至少不应该 panic
	_ = err
}

// ============================================================
// 多文件 Diff & Apply 测试
// ============================================================

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

func TestMultiFileDiffNoChanges(t *testing.T) {
	changes := []FileChange{
		{Path: "same.go", OldContent: "package same\n", NewContent: "package same\n"},
	}
	mfd := DiffMultiFiles(changes, 3)
	if len(mfd.Files) != 0 {
		t.Errorf("expected 0 changed files, got %d", len(mfd.Files))
	}
}

func TestApplyMultiFileDiffNil(t *testing.T) {
	result := ApplyMultiFileDiff(nil, &memAccessor{files: map[string]string{}})
	if result.TotalFiles != 0 || result.Failed != 0 {
		t.Error("expected empty result for nil input")
	}
}

func TestApplyMultiFileDiffReadError(t *testing.T) {
	mfd := &MultiFileDiff{
		Files: []FileDiff{
			{
				OldName: "missing.go",
				NewName: "missing.go",
				Hunks: []Hunk{{
					OldStart: 1, OldLines: 1,
					NewStart: 1, NewLines: 1,
					Lines: []DiffLine{
						{Kind: OpDelete, Text: "old"},
						{Kind: OpInsert, Text: "new"},
					},
				}},
			},
		},
	}
	accessor := &memAccessor{files: map[string]string{}}
	result := ApplyMultiFileDiff(mfd, accessor)
	if result.Failed != 1 {
		t.Errorf("expected 1 failure, got %d", result.Failed)
	}
	if result.Succeeded != 0 {
		t.Errorf("expected 0 succeeded, got %d", result.Succeeded)
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

// ============================================================
// 新文件 & 删除文件测试
// ============================================================

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

	// 验证 diff 中全是删除行
	for _, h := range mfd.Files[0].Hunks {
		for _, dl := range h.Lines {
			if dl.Kind == OpInsert {
				t.Error("delete file diff should not have insert lines")
			}
		}
	}
}

// ============================================================
// 往返一致性（Round-trip）测试
// ============================================================

func TestRoundTripSimple(t *testing.T) {
	roundTripTest(t, "line1\nline2\nline3\n", "line1\nchanged\nline3\n")
}

func TestRoundTripInsertOnly(t *testing.T) {
	roundTripTest(t, "a\nb\n", "a\nnew\nb\n")
}

func TestRoundTripDeleteOnly(t *testing.T) {
	roundTripTest(t, "a\nb\nc\n", "a\nc\n")
}

func TestRoundTripCompleteRewrite(t *testing.T) {
	roundTripTest(t, "old1\nold2\nold3\n", "new1\nnew2\nnew3\nnew4\n")
}

func TestRoundTripLargeFile(t *testing.T) {
	var oldLines, newLines []string
	for i := 0; i < 100; i++ {
		oldLines = append(oldLines, fmt.Sprintf("line %d", i))
		if i == 30 || i == 70 {
			newLines = append(newLines, fmt.Sprintf("changed %d", i))
		} else if i == 50 {
			// 删除第 50 行
			continue
		} else {
			newLines = append(newLines, fmt.Sprintf("line %d", i))
		}
	}
	// 末尾插入
	newLines = append(newLines, "appended line")

	oldContent := strings.Join(oldLines, "\n") + "\n"
	newContent := strings.Join(newLines, "\n") + "\n"
	roundTripTest(t, oldContent, newContent)
}

func TestRoundTripEmptyLines(t *testing.T) {
	roundTripTest(t, "a\n\nb\n\nc\n", "a\n\nB\n\nc\n")
}

func TestRoundTripSingleLineFile(t *testing.T) {
	roundTripTest(t, "old\n", "new\n")
}

func roundTripTest(t *testing.T, oldContent, newContent string) {
	t.Helper()

	fd := DiffFiles("test.txt", oldContent, "test.txt", newContent, 3)
	patchStr := FormatFileDiff(fd)

	parsed, err := ParseFileDiff(patchStr)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	result, err := ApplyFileDiff(oldContent, parsed)
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if result != newContent {
		t.Errorf("round-trip mismatch:\nold:    %q\nwant:   %q\ngot:    %q\npatch:\n%s",
			oldContent, newContent, result, patchStr)
	}
}

// ============================================================
// splitLines 测试
// ============================================================

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input string
		want  int // 期望行数，-1 表示 nil
	}{
		{"", -1},
		{"\n", -1},
		{"a", 1},
		{"a\n", 1},
		{"a\nb", 2},
		{"a\nb\n", 2},
		{"a\n\nb\n", 3}, // 包含空行
	}
	for _, tt := range tests {
		got := splitLines(tt.input)
		wantNil := tt.want == -1
		if wantNil {
			if got != nil {
				t.Errorf("splitLines(%q) = %v, want nil", tt.input, got)
			}
		} else {
			if len(got) != tt.want {
				t.Errorf("splitLines(%q) = %d lines, want %d", tt.input, len(got), tt.want)
			}
		}
	}
}

// ============================================================
// 辅助结构
// ============================================================

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
