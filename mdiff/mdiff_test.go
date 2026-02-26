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

func (m *memAccessor) DeleteFile(path string) error {
	if _, ok := m.files[path]; !ok {
		return &ApplyError{File: path, Message: "file not found"}
	}
	delete(m.files, path)
	return nil
}

// ============================================================
// 空行解析 & 宽松匹配测试
// ============================================================

func TestParseHunkWithEmptyContextLine(t *testing.T) {
	// 模拟 LLM 生成的 diff，空上下文行无前导空格
	patchText := "--- test.py\n+++ test.py\n@@ -1,5 +1,5 @@\n def func():\n\n     pass\n-    old\n+    new\n"
	// 第二行是空行（本应是 " " 即空格开头的上下文行，但 LLM 省略了空格）

	fd, err := ParseFileDiff(patchText)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(fd.Hunks) == 0 {
		t.Fatal("expected at least 1 hunk")
	}

	// 验证空行被正确解析为上下文行
	foundEmptyContext := false
	for _, dl := range fd.Hunks[0].Lines {
		if dl.Kind == OpEqual && dl.Text == "" {
			foundEmptyContext = true
			break
		}
	}
	if !foundEmptyContext {
		t.Error("expected empty context line to be parsed")
	}
}

func TestParseHunkEmptyLineAsDelete(t *testing.T) {
	// 当只有 oldRemaining > 0 时，空行应被视为删除的空行
	// @@ -1,4 +1,1 @@ 表示旧文件 4 行变为新文件 1 行
	// keep (Equal) → oldRem=3,newRem=0
	// -removed1 (Delete) → oldRem=2
	// 空行 → 此时只有 oldRem > 0，应视为 Delete
	// -removed2 (Delete) → oldRem=0
	patchText := "--- test.py\n+++ test.py\n@@ -1,4 +1,1 @@\n keep\n-removed1\n\n-removed2\n"

	fd, err := ParseFileDiff(patchText)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(fd.Hunks) == 0 {
		t.Fatal("expected at least 1 hunk")
	}

	// 验证所有 4 个旧行都被正确消费
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

func TestApplyWithTrailingWhitespaceDiff(t *testing.T) {
	// 源文件有行尾空格，但 patch 中的上下文没有（LLM 常见问题）
	oldContent := "def func():  \n    old_code\n    return\n"
	// patch 中上下文行没有行尾空格
	patchText := "--- test.py\n+++ test.py\n@@ -1,3 +1,3 @@\n def func():\n-    old_code\n+    new_code\n     return\n"

	result, err := PatchText(oldContent, patchText)
	if err != nil {
		t.Fatalf("patch with trailing whitespace diff failed: %v", err)
	}
	if !strings.Contains(result, "new_code") {
		t.Errorf("expected 'new_code' in result, got %q", result)
	}
}

func TestRoundTripWithEmptyLines(t *testing.T) {
	// Python 风格代码，函数间有空行
	oldContent := "def func1():\n    pass\n\n\ndef func2():\n    old\n\n\ndef func3():\n    pass\n"
	newContent := "def func1():\n    pass\n\n\ndef func2():\n    new\n\n\ndef func3():\n    pass\n"

	roundTripTest(t, oldContent, newContent)
}

func TestRoundTripMultipleEmptyLines(t *testing.T) {
	// 连续多个空行
	oldContent := "a\n\n\n\nb\n"
	newContent := "a\n\n\n\nc\n"

	roundTripTest(t, oldContent, newContent)
}

func TestMatchAtLoose(t *testing.T) {
	// 行尾有空格的情况
	lines := []string{"hello  ", "world\t", "end"}

	// 精确匹配失败
	if matchAt(lines, []string{"hello", "world"}, 0) {
		t.Error("strict match should fail with trailing whitespace")
	}
	// 宽松匹配成功
	if !matchAtLoose(lines, []string{"hello", "world"}, 0) {
		t.Error("loose match should succeed with trailing whitespace diff")
	}
	// 宽松匹配也不应该匹配不同内容
	if matchAtLoose(lines, []string{"hello", "different"}, 0) {
		t.Error("loose match should fail for different content")
	}
}

// ============================================================
// 多 hunk 同时修改测试
// ============================================================

func TestMultiHunkSameFile(t *testing.T) {
	// 同一文件两处修改，相距较远
	var oldLines, newLines []string
	for i := 0; i < 30; i++ {
		oldLines = append(oldLines, fmt.Sprintf("line%d", i+1))
		newLines = append(newLines, fmt.Sprintf("line%d", i+1))
	}
	newLines[2] = "CHANGED_A"  // 第3行
	newLines[25] = "CHANGED_B" // 第26行

	oldContent := strings.Join(oldLines, "\n") + "\n"
	newContent := strings.Join(newLines, "\n") + "\n"

	// 生成 diff，验证产生 2 个 hunk
	fd := DiffFiles("test.go", oldContent, "test.go", newContent, 3)
	if len(fd.Hunks) != 2 {
		t.Fatalf("expected 2 hunks, got %d", len(fd.Hunks))
	}

	// 往返验证
	roundTripTest(t, oldContent, newContent)
}

func TestMultiHunkThreeChanges(t *testing.T) {
	// 三处修改
	var oldLines, newLines []string
	for i := 0; i < 50; i++ {
		oldLines = append(oldLines, fmt.Sprintf("line%d", i+1))
		newLines = append(newLines, fmt.Sprintf("line%d", i+1))
	}
	newLines[5] = "CHANGE_1"
	newLines[25] = "CHANGE_2"
	newLines[45] = "CHANGE_3"

	oldContent := strings.Join(oldLines, "\n") + "\n"
	newContent := strings.Join(newLines, "\n") + "\n"

	fd := DiffFiles("test.go", oldContent, "test.go", newContent, 3)
	if len(fd.Hunks) != 3 {
		t.Fatalf("expected 3 hunks, got %d", len(fd.Hunks))
	}

	roundTripTest(t, oldContent, newContent)
}

func TestMultiHunkInsertAndDelete(t *testing.T) {
	// 一处删除、一处插入
	var oldLines, newLines []string
	for i := 0; i < 30; i++ {
		oldLines = append(oldLines, fmt.Sprintf("line%d", i+1))
	}
	// 删除第5行，在第25行后插入新行
	for i := 0; i < 30; i++ {
		if i == 4 {
			continue // 删除 line5
		}
		newLines = append(newLines, fmt.Sprintf("line%d", i+1))
		if i == 24 {
			newLines = append(newLines, "INSERTED_LINE")
		}
	}

	oldContent := strings.Join(oldLines, "\n") + "\n"
	newContent := strings.Join(newLines, "\n") + "\n"
	roundTripTest(t, oldContent, newContent)
}

func TestMultiHunkUnsortedHunks(t *testing.T) {
	// 验证 ApplyHunks 能处理未排序的 hunks
	lines := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}

	// 故意乱序：hunk2 在前，hunk1 在后
	hunks := []Hunk{
		{
			OldStart: 8, OldLines: 1, NewStart: 8, NewLines: 1,
			Lines: []DiffLine{
				{Kind: OpDelete, Text: "h"},
				{Kind: OpInsert, Text: "H"},
			},
		},
		{
			OldStart: 2, OldLines: 1, NewStart: 2, NewLines: 1,
			Lines: []DiffLine{
				{Kind: OpDelete, Text: "b"},
				{Kind: OpInsert, Text: "B"},
			},
		},
	}

	result, err := ApplyHunks(lines, hunks)
	if err != nil {
		t.Fatalf("ApplyHunks failed: %v", err)
	}
	expected := []string{"a", "B", "c", "d", "e", "f", "g", "H", "i", "j"}
	if strings.Join(result, ",") != strings.Join(expected, ",") {
		t.Errorf("got %v, want %v", result, expected)
	}
}

func TestMultiHunkAdjacentChanges(t *testing.T) {
	// 两个相邻的修改（上下文紧密衔接）
	oldContent := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\n"
	newContent := "line1\nCHANGED_2\nline3\nline4\nline5\nCHANGED_6\nline7\nline8\n"
	roundTripTest(t, oldContent, newContent)
}

func TestMultiHunkWithLineCountChange(t *testing.T) {
	// 第一个 hunk 增加行数，第二个 hunk 减少行数
	var oldLines, newLines []string
	for i := 0; i < 30; i++ {
		oldLines = append(oldLines, fmt.Sprintf("line%d", i+1))
		newLines = append(newLines, fmt.Sprintf("line%d", i+1))
	}
	// 在第5行后插入2行（增加）
	expanded := make([]string, 0)
	for i, l := range newLines {
		expanded = append(expanded, l)
		if i == 4 {
			expanded = append(expanded, "EXTRA_A", "EXTRA_B")
		}
	}
	// 删除第25行（减少）
	var finalNew []string
	for i, l := range expanded {
		if i == 26 { // 原始第25行，因插入偏移了2行
			continue
		}
		finalNew = append(finalNew, l)
	}

	oldContent := strings.Join(oldLines, "\n") + "\n"
	newContent := strings.Join(finalNew, "\n") + "\n"
	roundTripTest(t, oldContent, newContent)
}

func TestMultiHunkFuzzyMatch(t *testing.T) {
	// 模拟 LLM 生成的 diff，行号有偏移
	lines := []string{
		"package main", "", "import \"fmt\"", "",
		"func hello() {", "\tfmt.Println(\"hello\")", "}",
		"", "func world() {", "\tfmt.Println(\"world\")", "}",
	}
	hunks := []Hunk{
		{
			OldStart: 20, // 故意设错行号
			OldLines: 3,
			NewStart: 20,
			NewLines: 3,
			Lines: []DiffLine{
				{Kind: OpEqual, Text: "func hello() {"},
				{Kind: OpDelete, Text: "\tfmt.Println(\"hello\")"},
				{Kind: OpInsert, Text: "\tfmt.Println(\"hi\")"},
				{Kind: OpEqual, Text: "}"},
			},
		},
		{
			OldStart: 30, // 故意设错行号
			OldLines: 3,
			NewStart: 30,
			NewLines: 3,
			Lines: []DiffLine{
				{Kind: OpEqual, Text: "func world() {"},
				{Kind: OpDelete, Text: "\tfmt.Println(\"world\")"},
				{Kind: OpInsert, Text: "\tfmt.Println(\"earth\")"},
				{Kind: OpEqual, Text: "}"},
			},
		},
	}

	result, err := ApplyHunks(lines, hunks)
	if err != nil {
		t.Fatalf("fuzzy multi-hunk failed: %v", err)
	}

	expected := []string{
		"package main", "", "import \"fmt\"", "",
		"func hello() {", "\tfmt.Println(\"hi\")", "}",
		"", "func world() {", "\tfmt.Println(\"earth\")", "}",
	}
	if strings.Join(result, "\n") != strings.Join(expected, "\n") {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(result, "\n"), strings.Join(expected, "\n"))
	}
}

func TestMultiHunkLooseMatch(t *testing.T) {
	// 原文件有行尾空格，patch 没有
	lines := []string{"func a() {  ", "\told  ", "}", "", "func b() {  ", "\told  ", "}"}
	hunks := []Hunk{
		{
			OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 3,
			Lines: []DiffLine{
				{Kind: OpEqual, Text: "func a() {"}, // 没有行尾空格
				{Kind: OpDelete, Text: "\told"},     // 没有行尾空格
				{Kind: OpInsert, Text: "\tnew_a"},
				{Kind: OpEqual, Text: "}"},
			},
		},
		{
			OldStart: 5, OldLines: 3, NewStart: 5, NewLines: 3,
			Lines: []DiffLine{
				{Kind: OpEqual, Text: "func b() {"}, // 没有行尾空格
				{Kind: OpDelete, Text: "\told"},     // 没有行尾空格
				{Kind: OpInsert, Text: "\tnew_b"},
				{Kind: OpEqual, Text: "}"},
			},
		},
	}

	result, err := ApplyHunks(lines, hunks)
	if err != nil {
		t.Fatalf("loose multi-hunk failed: %v", err)
	}

	// Equal 行应保留原文件的空白（含行尾空格）
	if result[0] != "func a() {  " {
		t.Errorf("Equal line should preserve original whitespace, got %q", result[0])
	}
	if result[1] != "\tnew_a" {
		t.Errorf("Insert line should use hunk content, got %q", result[1])
	}
	if result[4] != "func b() {  " {
		t.Errorf("Equal line should preserve original whitespace, got %q", result[4])
	}
}

func TestApplyMultiFileDiffMultipleChanges(t *testing.T) {
	// 多文件各有多处修改
	files := map[string]string{
		"main.go": "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"old1\")\n}\n\nfunc helper() {\n\tfmt.Println(\"old2\")\n}\n",
		"util.go": "package main\n\nvar x = 1\n\nvar y = 2\n",
	}

	changes := []FileChange{
		{
			Path:       "main.go",
			OldContent: files["main.go"],
			NewContent: "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"new1\")\n}\n\nfunc helper() {\n\tfmt.Println(\"new2\")\n}\n",
		},
		{
			Path:       "util.go",
			OldContent: files["util.go"],
			NewContent: "package main\n\nvar x = 10\n\nvar y = 20\n",
		},
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
		t.FailNow()
	}

	if !strings.Contains(accessor.files["main.go"], "new1") || !strings.Contains(accessor.files["main.go"], "new2") {
		t.Errorf("main.go not properly patched: %s", accessor.files["main.go"])
	}
	if !strings.Contains(accessor.files["util.go"], "x = 10") || !strings.Contains(accessor.files["util.go"], "y = 20") {
		t.Errorf("util.go not properly patched: %s", accessor.files["util.go"])
	}
}

func TestParseHunkEarlyTerminateOnFileSeparator(t *testing.T) {
	// 模拟 hunk 行计数不准确，但遇到下一个文件分隔符时应终止
	patchText := "--- a.go\n+++ a.go\n@@ -1,5 +1,5 @@\n line1\n-old\n+new\n line3\n--- b.go\n+++ b.go\n@@ -1,3 +1,3 @@\n line1\n-old\n+new\n line3\n"

	mfd, err := ParseMultiFileDiff(patchText)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(mfd.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(mfd.Files))
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
	// 验证不修改原 slice
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

func TestRoundTripMultiHunkRealWorld(t *testing.T) {
	// 模拟真实 Go 文件的多处修改
	oldContent := `package handler

import (
	"fmt"
	"net/http"
)

func HandleGet(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", 400)
		return
	}
	data := fetchData(id)
	fmt.Fprintf(w, "data: %s", data)
}

func HandlePost(w http.ResponseWriter, r *http.Request) {
	body := readBody(r)
	if body == "" {
		http.Error(w, "empty body", 400)
		return
	}
	saveData(body)
	w.WriteHeader(201)
}

func HandleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", 400)
		return
	}
	deleteData(id)
	w.WriteHeader(204)
}
`
	newContent := `package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func HandleGet(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	data := fetchData(id)
	json.NewEncoder(w).Encode(data)
}

func HandlePost(w http.ResponseWriter, r *http.Request) {
	body := readBody(r)
	if body == "" {
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}
	saveData(body)
	w.WriteHeader(http.StatusCreated)
}

func HandleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	deleteData(id)
	w.WriteHeader(http.StatusNoContent)
}
`
	roundTripTest(t, oldContent, newContent)
}

// ============================================================
// Python 代码编辑场景测试
// ============================================================

func TestRoundTripPythonMultiFunctionEdit(t *testing.T) {
	// Python 文件中同时修改多个函数
	oldContent := `import os
import sys
from typing import List, Optional


def read_config(path: str) -> dict:
    """读取配置文件"""
    with open(path, 'r') as f:
        return json.load(f)


def process_data(data: List[dict]) -> List[dict]:
    """处理数据"""
    result = []
    for item in data:
        if item.get('active'):
            result.append(item)
    return result


def save_output(data: List[dict], path: str) -> None:
    """保存输出"""
    with open(path, 'w') as f:
        json.dump(data, f)


def main():
    config = read_config('config.json')
    data = process_data(config['data'])
    save_output(data, 'output.json')
    print("Done")


if __name__ == '__main__':
    main()
`
	newContent := `import os
import sys
import logging
from typing import List, Optional

logger = logging.getLogger(__name__)


def read_config(path: str) -> dict:
    """读取配置文件"""
    logger.info(f"Reading config from {path}")
    with open(path, 'r') as f:
        return json.load(f)


def process_data(data: List[dict], filter_key: str = 'active') -> List[dict]:
    """处理数据，支持自定义过滤键"""
    result = []
    for item in data:
        if item.get(filter_key):
            result.append(item)
    logger.info(f"Processed {len(result)}/{len(data)} items")
    return result


def save_output(data: List[dict], path: str) -> None:
    """保存输出"""
    with open(path, 'w') as f:
        json.dump(data, f, indent=2)
    logger.info(f"Saved {len(data)} items to {path}")


def main():
    logging.basicConfig(level=logging.INFO)
    config = read_config('config.json')
    data = process_data(config['data'])
    save_output(data, 'output.json')
    logger.info("Done")


if __name__ == '__main__':
    main()
`
	roundTripTest(t, oldContent, newContent)
}

func TestRoundTripPythonClassEdit(t *testing.T) {
	// Python 类中同时修改多个方法
	oldContent := `class UserService:
    def __init__(self, db):
        self.db = db

    def get_user(self, user_id: int):
        return self.db.query(User).filter_by(id=user_id).first()

    def create_user(self, name: str, email: str):
        user = User(name=name, email=email)
        self.db.add(user)
        self.db.commit()
        return user

    def update_user(self, user_id: int, **kwargs):
        user = self.get_user(user_id)
        for key, value in kwargs.items():
            setattr(user, key, value)
        self.db.commit()
        return user

    def delete_user(self, user_id: int):
        user = self.get_user(user_id)
        self.db.delete(user)
        self.db.commit()
`
	newContent := `class UserService:
    def __init__(self, db):
        self.db = db
        self.cache = {}

    def get_user(self, user_id: int):
        if user_id in self.cache:
            return self.cache[user_id]
        user = self.db.query(User).filter_by(id=user_id).first()
        if user:
            self.cache[user_id] = user
        return user

    def create_user(self, name: str, email: str):
        user = User(name=name, email=email)
        self.db.add(user)
        self.db.commit()
        self.cache[user.id] = user
        return user

    def update_user(self, user_id: int, **kwargs):
        user = self.get_user(user_id)
        if not user:
            raise ValueError(f"User {user_id} not found")
        for key, value in kwargs.items():
            setattr(user, key, value)
        self.db.commit()
        self.cache[user_id] = user
        return user

    def delete_user(self, user_id: int):
        user = self.get_user(user_id)
        if not user:
            raise ValueError(f"User {user_id} not found")
        self.db.delete(user)
        self.db.commit()
        self.cache.pop(user_id, None)
`
	roundTripTest(t, oldContent, newContent)
}

func TestRoundTripPythonIndentationHeavy(t *testing.T) {
	// Python 深层缩进和空行较多的场景
	oldContent := `def complex_handler(request):
    if request.method == 'POST':
        data = request.json

        if 'items' in data:
            for item in data['items']:
                if item.get('type') == 'A':
                    process_type_a(item)

                elif item.get('type') == 'B':
                    process_type_b(item)

                else:
                    log.warning(f"Unknown type: {item.get('type')}")

        return {'status': 'ok'}

    return {'error': 'method not allowed'}
`
	newContent := `def complex_handler(request):
    if request.method == 'POST':
        data = request.json

        if 'items' not in data:
            return {'error': 'items required'}

        results = []
        for item in data['items']:
            if item.get('type') == 'A':
                result = process_type_a(item)

            elif item.get('type') == 'B':
                result = process_type_b(item)

            else:
                log.warning(f"Unknown type: {item.get('type')}")
                continue

            results.append(result)

        return {'status': 'ok', 'results': results}

    return {'error': 'method not allowed'}
`
	roundTripTest(t, oldContent, newContent)
}

// ============================================================
// HTML/CSS/JS/TS 代码编辑场景测试
// ============================================================

func TestRoundTripHTMLMultiSectionEdit(t *testing.T) {
	// HTML 文件中同时修改 head 和 body 部分
	oldContent := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>My App</title>
    <link rel="stylesheet" href="style.css">
</head>
<body>
    <header>
        <h1>Welcome</h1>
        <nav>
            <a href="/">Home</a>
            <a href="/about">About</a>
        </nav>
    </header>

    <main>
        <section class="hero">
            <h2>Hello World</h2>
            <p>This is a simple page.</p>
        </section>

        <section class="content">
            <div class="card">
                <h3>Card Title</h3>
                <p>Card content here.</p>
            </div>
        </section>
    </main>

    <footer>
        <p>&copy; 2024 My App</p>
    </footer>

    <script src="app.js"></script>
</body>
</html>
`
	newContent := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>My App - Dashboard</title>
    <link rel="stylesheet" href="style.css">
    <link rel="stylesheet" href="dashboard.css">
</head>
<body>
    <header>
        <h1>Dashboard</h1>
        <nav>
            <a href="/">Home</a>
            <a href="/dashboard">Dashboard</a>
            <a href="/about">About</a>
        </nav>
    </header>

    <main>
        <section class="hero">
            <h2>Welcome Back</h2>
            <p>Here is your dashboard overview.</p>
        </section>

        <section class="content">
            <div class="card">
                <h3>Statistics</h3>
                <p>Total users: <span id="user-count">0</span></p>
            </div>
            <div class="card">
                <h3>Recent Activity</h3>
                <ul id="activity-list"></ul>
            </div>
        </section>
    </main>

    <footer>
        <p>&copy; 2025 My App. All rights reserved.</p>
    </footer>

    <script src="app.js"></script>
    <script src="dashboard.js"></script>
</body>
</html>
`
	roundTripTest(t, oldContent, newContent)
}

func TestRoundTripCSSMultiRuleEdit(t *testing.T) {
	// CSS 文件中同时修改多个规则
	oldContent := `:root {
    --primary: #3498db;
    --secondary: #2ecc71;
    --bg: #ffffff;
    --text: #333333;
}

body {
    font-family: Arial, sans-serif;
    background-color: var(--bg);
    color: var(--text);
    margin: 0;
    padding: 0;
}

.container {
    max-width: 1200px;
    margin: 0 auto;
    padding: 20px;
}

.header {
    background-color: var(--primary);
    color: white;
    padding: 10px 20px;
}

.card {
    border: 1px solid #ddd;
    border-radius: 4px;
    padding: 16px;
    margin-bottom: 16px;
}

.btn {
    display: inline-block;
    padding: 8px 16px;
    background-color: var(--primary);
    color: white;
    border: none;
    border-radius: 4px;
    cursor: pointer;
}

.footer {
    background-color: #f5f5f5;
    padding: 20px;
    text-align: center;
}
`
	newContent := `:root {
    --primary: #6366f1;
    --secondary: #10b981;
    --bg: #f8fafc;
    --text: #1e293b;
    --border: #e2e8f0;
    --shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
}

body {
    font-family: 'Inter', system-ui, sans-serif;
    background-color: var(--bg);
    color: var(--text);
    margin: 0;
    padding: 0;
    line-height: 1.6;
}

.container {
    max-width: 1280px;
    margin: 0 auto;
    padding: 24px;
}

.header {
    background-color: var(--primary);
    color: white;
    padding: 12px 24px;
    box-shadow: var(--shadow);
}

.card {
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 20px;
    margin-bottom: 20px;
    box-shadow: var(--shadow);
    transition: transform 0.2s ease;
}

.card:hover {
    transform: translateY(-2px);
}

.btn {
    display: inline-block;
    padding: 10px 20px;
    background-color: var(--primary);
    color: white;
    border: none;
    border-radius: 6px;
    cursor: pointer;
    font-weight: 500;
    transition: background-color 0.2s ease;
}

.btn:hover {
    background-color: #4f46e5;
}

.footer {
    background-color: #f1f5f9;
    padding: 24px;
    text-align: center;
    border-top: 1px solid var(--border);
}
`
	roundTripTest(t, oldContent, newContent)
}

func TestRoundTripJavaScriptMultiFunctionEdit(t *testing.T) {
	// JavaScript 文件中同时修改多个函数和导入
	oldContent := `import { useState } from 'react';

function fetchData(url) {
    return fetch(url)
        .then(res => res.json())
        .catch(err => console.error(err));
}

function formatDate(date) {
    return date.toLocaleDateString();
}

function UserList({ users }) {
    return (
        <ul>
            {users.map(user => (
                <li key={user.id}>{user.name}</li>
            ))}
        </ul>
    );
}

export default function App() {
    const [users, setUsers] = useState([]);

    const loadUsers = () => {
        fetchData('/api/users').then(data => setUsers(data));
    };

    return (
        <div>
            <h1>User Management</h1>
            <button onClick={loadUsers}>Load Users</button>
            <UserList users={users} />
        </div>
    );
}
`
	newContent := `import { useState, useEffect, useCallback } from 'react';

async function fetchData(url, options = {}) {
    try {
        const res = await fetch(url, options);
        if (!res.ok) throw new Error(` + "`HTTP ${res.status}`" + `);
        return await res.json();
    } catch (err) {
        console.error('Fetch error:', err);
        throw err;
    }
}

function formatDate(date, locale = 'zh-CN') {
    return new Intl.DateTimeFormat(locale, {
        year: 'numeric',
        month: 'long',
        day: 'numeric',
    }).format(date);
}

function UserList({ users, onSelect }) {
    if (users.length === 0) {
        return <p>No users found.</p>;
    }
    return (
        <ul>
            {users.map(user => (
                <li key={user.id} onClick={() => onSelect(user)}>
                    {user.name} - {formatDate(new Date(user.createdAt))}
                </li>
            ))}
        </ul>
    );
}

export default function App() {
    const [users, setUsers] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);

    const loadUsers = useCallback(async () => {
        setLoading(true);
        setError(null);
        try {
            const data = await fetchData('/api/users');
            setUsers(data);
        } catch (err) {
            setError(err.message);
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        loadUsers();
    }, [loadUsers]);

    return (
        <div>
            <h1>User Management</h1>
            {error && <p style={{ color: 'red' }}>{error}</p>}
            <button onClick={loadUsers} disabled={loading}>
                {loading ? 'Loading...' : 'Refresh'}
            </button>
            <UserList users={users} onSelect={u => console.log(u)} />
        </div>
    );
}
`
	roundTripTest(t, oldContent, newContent)
}

func TestRoundTripTypeScriptMultiEdit(t *testing.T) {
	// TypeScript 接口和实现同时修改
	oldContent := `interface User {
    id: number;
    name: string;
    email: string;
}

interface ApiResponse<T> {
    data: T;
    status: number;
}

class UserRepository {
    private baseUrl: string;

    constructor(baseUrl: string) {
        this.baseUrl = baseUrl;
    }

    async getUser(id: number): Promise<User> {
        const res = await fetch(` + "`${this.baseUrl}/users/${id}`" + `);
        const json: ApiResponse<User> = await res.json();
        return json.data;
    }

    async listUsers(): Promise<User[]> {
        const res = await fetch(` + "`${this.baseUrl}/users`" + `);
        const json: ApiResponse<User[]> = await res.json();
        return json.data;
    }

    async createUser(user: Omit<User, 'id'>): Promise<User> {
        const res = await fetch(` + "`${this.baseUrl}/users`" + `, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(user),
        });
        const json: ApiResponse<User> = await res.json();
        return json.data;
    }
}

export { UserRepository };
export type { User, ApiResponse };
`
	newContent := `interface User {
    id: number;
    name: string;
    email: string;
    role: 'admin' | 'user' | 'guest';
    createdAt: string;
}

interface PaginatedResponse<T> {
    data: T[];
    total: number;
    page: number;
    pageSize: number;
}

interface ApiResponse<T> {
    data: T;
    status: number;
    message?: string;
}

class UserRepository {
    private baseUrl: string;
    private token: string | null;

    constructor(baseUrl: string, token?: string) {
        this.baseUrl = baseUrl;
        this.token = token ?? null;
    }

    private getHeaders(): HeadersInit {
        const headers: HeadersInit = { 'Content-Type': 'application/json' };
        if (this.token) {
            headers['Authorization'] = ` + "`Bearer ${this.token}`" + `;
        }
        return headers;
    }

    async getUser(id: number): Promise<User> {
        const res = await fetch(` + "`${this.baseUrl}/users/${id}`" + `, {
            headers: this.getHeaders(),
        });
        if (!res.ok) throw new Error(` + "`Failed to get user: ${res.status}`" + `);
        const json: ApiResponse<User> = await res.json();
        return json.data;
    }

    async listUsers(page = 1, pageSize = 20): Promise<PaginatedResponse<User>> {
        const url = ` + "`${this.baseUrl}/users?page=${page}&pageSize=${pageSize}`" + `;
        const res = await fetch(url, {
            headers: this.getHeaders(),
        });
        if (!res.ok) throw new Error(` + "`Failed to list users: ${res.status}`" + `);
        return await res.json();
    }

    async createUser(user: Omit<User, 'id' | 'createdAt'>): Promise<User> {
        const res = await fetch(` + "`${this.baseUrl}/users`" + `, {
            method: 'POST',
            headers: this.getHeaders(),
            body: JSON.stringify(user),
        });
        if (!res.ok) throw new Error(` + "`Failed to create user: ${res.status}`" + `);
        const json: ApiResponse<User> = await res.json();
        return json.data;
    }

    async deleteUser(id: number): Promise<void> {
        const res = await fetch(` + "`${this.baseUrl}/users/${id}`" + `, {
            method: 'DELETE',
            headers: this.getHeaders(),
        });
        if (!res.ok) throw new Error(` + "`Failed to delete user: ${res.status}`" + `);
    }
}

export { UserRepository };
export type { User, ApiResponse, PaginatedResponse };
`
	roundTripTest(t, oldContent, newContent)
}

func TestRoundTripVueComponentEdit(t *testing.T) {
	// Vue/Svelte 风格的单文件组件，同时修改 template/script/style
	oldContent := `<template>
  <div class="todo-app">
    <h1>Todo List</h1>
    <input v-model="newTodo" @keyup.enter="addTodo" placeholder="Add a todo">
    <ul>
      <li v-for="todo in todos" :key="todo.id">
        {{ todo.text }}
        <button @click="removeTodo(todo.id)">Delete</button>
      </li>
    </ul>
  </div>
</template>

<script>
export default {
  data() {
    return {
      newTodo: '',
      todos: [],
    };
  },
  methods: {
    addTodo() {
      if (this.newTodo.trim()) {
        this.todos.push({ id: Date.now(), text: this.newTodo });
        this.newTodo = '';
      }
    },
    removeTodo(id) {
      this.todos = this.todos.filter(t => t.id !== id);
    },
  },
};
</script>

<style scoped>
.todo-app {
  max-width: 500px;
  margin: 0 auto;
}
input {
  width: 100%;
  padding: 8px;
}
li {
  display: flex;
  justify-content: space-between;
  padding: 8px 0;
}
</style>
`
	newContent := `<template>
  <div class="todo-app">
    <h1>{{ title }}</h1>
    <div class="input-group">
      <input v-model="newTodo" @keyup.enter="addTodo" placeholder="What needs to be done?">
      <button @click="addTodo" :disabled="!newTodo.trim()">Add</button>
    </div>
    <div class="filters">
      <button v-for="f in filters" :key="f" @click="filter = f" :class="{ active: filter === f }">
        {{ f }}
      </button>
    </div>
    <ul>
      <li v-for="todo in filteredTodos" :key="todo.id" :class="{ done: todo.done }">
        <input type="checkbox" v-model="todo.done">
        <span>{{ todo.text }}</span>
        <button @click="removeTodo(todo.id)">Delete</button>
      </li>
    </ul>
    <p class="stats">{{ remaining }} items left</p>
  </div>
</template>

<script>
export default {
  data() {
    return {
      title: 'Todo List',
      newTodo: '',
      todos: [],
      filter: 'All',
      filters: ['All', 'Active', 'Done'],
    };
  },
  computed: {
    filteredTodos() {
      if (this.filter === 'Active') return this.todos.filter(t => !t.done);
      if (this.filter === 'Done') return this.todos.filter(t => t.done);
      return this.todos;
    },
    remaining() {
      return this.todos.filter(t => !t.done).length;
    },
  },
  methods: {
    addTodo() {
      const text = this.newTodo.trim();
      if (text) {
        this.todos.push({ id: Date.now(), text, done: false });
        this.newTodo = '';
      }
    },
    removeTodo(id) {
      this.todos = this.todos.filter(t => t.id !== id);
    },
  },
};
</script>

<style scoped>
.todo-app {
  max-width: 600px;
  margin: 0 auto;
  padding: 20px;
}
.input-group {
  display: flex;
  gap: 8px;
}
input[type="text"] {
  flex: 1;
  padding: 10px;
  border: 1px solid #ddd;
  border-radius: 4px;
}
.filters {
  display: flex;
  gap: 4px;
  margin: 12px 0;
}
.filters button.active {
  font-weight: bold;
  text-decoration: underline;
}
li {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 0;
  border-bottom: 1px solid #eee;
}
li.done span {
  text-decoration: line-through;
  opacity: 0.6;
}
.stats {
  color: #888;
  font-size: 14px;
}
</style>
`
	roundTripTest(t, oldContent, newContent)
}

func TestMultiFilePythonAndHTML(t *testing.T) {
	// 同时修改 Python 后端和 HTML 模板
	files := map[string]string{
		"app.py":               "from flask import Flask, render_template\n\napp = Flask(__name__)\n\n@app.route('/')\ndef index():\n    return render_template('index.html', title='Home')\n\n@app.route('/about')\ndef about():\n    return render_template('about.html', title='About')\n",
		"templates/index.html": "<h1>{{ title }}</h1>\n<p>Welcome to our site.</p>\n<a href=\"/about\">About</a>\n",
	}

	changes := []FileChange{
		{
			Path:       "app.py",
			OldContent: files["app.py"],
			NewContent: "from flask import Flask, render_template, jsonify\n\napp = Flask(__name__)\n\n@app.route('/')\ndef index():\n    return render_template('index.html', title='Home', version='2.0')\n\n@app.route('/about')\ndef about():\n    return render_template('about.html', title='About')\n\n@app.route('/api/health')\ndef health():\n    return jsonify({'status': 'ok'})\n",
		},
		{
			Path:       "templates/index.html",
			OldContent: files["templates/index.html"],
			NewContent: "<h1>{{ title }}</h1>\n<p>Welcome to our site. Version {{ version }}.</p>\n<nav>\n    <a href=\"/about\">About</a>\n    <a href=\"/api/health\">Health</a>\n</nav>\n",
		},
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
		t.FailNow()
	}

	if !strings.Contains(accessor.files["app.py"], "jsonify") {
		t.Error("app.py should contain jsonify import")
	}
	if !strings.Contains(accessor.files["app.py"], "health") {
		t.Error("app.py should contain health endpoint")
	}
	if !strings.Contains(accessor.files["templates/index.html"], "Version {{ version }}") {
		t.Error("index.html should contain version variable")
	}
	if !strings.Contains(accessor.files["templates/index.html"], "Health") {
		t.Error("index.html should contain Health link")
	}
}

func TestRoundTripPythonLLMStyleDiff(t *testing.T) {
	// 模拟 LLM 生成的 Python 文件 diff，验证行尾空白宽松匹配
	oldContent := "def greet(name):  \n    msg = f\"Hello, {name}!\"  \n    print(msg)  \n    return msg  \n"
	// LLM 修改时可能不保留行尾空格
	newContent := "def greet(name):\n    msg = f\"Hi, {name}!\"\n    print(msg)\n    return msg\n"

	fd := DiffFiles("app.py", oldContent, "app.py", newContent, 3)
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
		t.Errorf("round-trip mismatch:\ngot:  %q\nwant: %q", result, newContent)
	}
}

// ============================================================
// 部分应用与增强匹配测试
// ============================================================

// TestPartialApplySkipsFailedHunks 验证部分 hunk 失败时其余 hunk 仍然生效
func TestPartialApplySkipsFailedHunks(t *testing.T) {
	oldLines := []string{
		"line1", "line2", "line3", "line4", "line5",
		"line6", "line7", "line8", "line9", "line10",
	}

	hunks := []Hunk{
		// hunk 1: 正确的上下文，应该成功
		{OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 3,
			Lines: []DiffLine{
				{Kind: OpEqual, Text: "line1"},
				{Kind: OpDelete, Text: "line2"},
				{Kind: OpInsert, Text: "CHANGED_2"},
				{Kind: OpEqual, Text: "line3"},
			}},
		// hunk 2: 错误的上下文，应该失败
		{OldStart: 5, OldLines: 3, NewStart: 5, NewLines: 3,
			Lines: []DiffLine{
				{Kind: OpEqual, Text: "WRONG_CONTEXT"},
				{Kind: OpDelete, Text: "line6"},
				{Kind: OpInsert, Text: "CHANGED_6"},
				{Kind: OpEqual, Text: "line7"},
			}},
		// hunk 3: 正确的上下文，应该成功
		{OldStart: 8, OldLines: 3, NewStart: 8, NewLines: 3,
			Lines: []DiffLine{
				{Kind: OpEqual, Text: "line8"},
				{Kind: OpDelete, Text: "line9"},
				{Kind: OpInsert, Text: "CHANGED_9"},
				{Kind: OpEqual, Text: "line10"},
			}},
	}

	result, err := ApplyHunks(oldLines, hunks)
	if err == nil {
		t.Fatal("expected PartialApplyError, got nil")
	}

	pae, ok := err.(*PartialApplyError)
	if !ok {
		t.Fatalf("expected *PartialApplyError, got %T: %v", err, err)
	}

	if pae.Applied != 2 || pae.Total != 3 {
		t.Errorf("expected 2/3 applied, got %d/%d", pae.Applied, pae.Total)
	}

	// 验证成功的 hunk 已经生效
	if result[1] != "CHANGED_2" {
		t.Errorf("hunk 1 should have been applied, got line2=%q", result[1])
	}
	if result[8] != "CHANGED_9" {
		t.Errorf("hunk 3 should have been applied, got line9=%q", result[8])
	}
	// 验证失败的 hunk 区域保持不变
	if result[4] != "line5" || result[5] != "line6" {
		t.Errorf("failed hunk region should be unchanged, got line5=%q line6=%q", result[4], result[5])
	}
}

// TestPartialApplyAllFailed 验证全部 hunk 失败时返回 ApplyError
func TestPartialApplyAllFailed(t *testing.T) {
	oldLines := []string{"line1", "line2", "line3"}

	hunks := []Hunk{
		{OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 2,
			Lines: []DiffLine{
				{Kind: OpEqual, Text: "WRONG"},
				{Kind: OpDelete, Text: "ALSO_WRONG"},
				{Kind: OpInsert, Text: "NEW"},
			}},
	}

	_, err := ApplyHunks(oldLines, hunks)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	_, ok := err.(*ApplyError)
	if !ok {
		t.Errorf("expected *ApplyError when all hunks fail, got %T", err)
	}
}

// TestMatchAtNormalized 验证归一化匹配（忽略前导和尾部空白）
func TestMatchAtNormalized(t *testing.T) {
	oldLines := []string{
		"    function hello() {",
		"        console.log('hello');",
		"    }",
	}

	// LLM 生成的 hunk 使用了不同的缩进
	hunks := []Hunk{
		{OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 3,
			Lines: []DiffLine{
				{Kind: OpEqual, Text: "  function hello() {"},     // 2空格 vs 4空格
				{Kind: OpDelete, Text: "\tconsole.log('hello');"}, // tab vs 8空格
				{Kind: OpInsert, Text: "        console.log('hi');"},
				{Kind: OpEqual, Text: "  }"}, // 2空格 vs 4空格
			}},
	}

	result, err := ApplyHunks(oldLines, hunks)
	if err != nil {
		t.Fatalf("expected normalized match to succeed, got: %v", err)
	}

	if result[1] != "        console.log('hi');" {
		t.Errorf("unexpected result: %q", result[1])
	}
}

// TestPartialApplyFileDiff 验证 ApplyFileDiff 的部分应用返回内容
func TestPartialApplyFileDiff(t *testing.T) {
	oldContent := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\n"

	fd := &FileDiff{
		OldName: "test.txt",
		NewName: "test.txt",
		Hunks: []Hunk{
			// 正确的 hunk
			{OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 3,
				Lines: []DiffLine{
					{Kind: OpEqual, Text: "line1"},
					{Kind: OpDelete, Text: "line2"},
					{Kind: OpInsert, Text: "CHANGED"},
					{Kind: OpEqual, Text: "line3"},
				}},
			// 错误上下文的 hunk
			{OldStart: 6, OldLines: 3, NewStart: 6, NewLines: 3,
				Lines: []DiffLine{
					{Kind: OpEqual, Text: "NONEXISTENT"},
					{Kind: OpDelete, Text: "line7"},
					{Kind: OpInsert, Text: "CHANGED7"},
					{Kind: OpEqual, Text: "line8"},
				}},
		},
	}

	result, err := ApplyFileDiff(oldContent, fd)

	// 应该返回 PartialApplyError
	if err == nil {
		t.Fatal("expected PartialApplyError")
	}
	pae, ok := err.(*PartialApplyError)
	if !ok {
		t.Fatalf("expected *PartialApplyError, got %T: %v", err, err)
	}
	if pae.Applied != 1 {
		t.Errorf("expected 1 applied, got %d", pae.Applied)
	}

	// 应该返回部分修改后的内容
	if !strings.Contains(result, "CHANGED") {
		t.Errorf("partial result should contain successful hunk change, got: %q", result)
	}
	// 失败的区域保持不变
	if !strings.Contains(result, "line7") {
		t.Errorf("partial result should keep failed hunk region unchanged, got: %q", result)
	}
}

// TestErrorDiagnosticContainsActualContent 验证错误信息包含实际文件内容
func TestErrorDiagnosticContainsActualContent(t *testing.T) {
	oldLines := []string{"aaa", "bbb", "ccc"}

	hunks := []Hunk{
		{OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 2,
			Lines: []DiffLine{
				{Kind: OpEqual, Text: "xxx"},  // 不匹配
				{Kind: OpDelete, Text: "yyy"}, // 不匹配
				{Kind: OpInsert, Text: "new"},
			}},
	}

	_, err := ApplyHunks(oldLines, hunks)
	if err == nil {
		t.Fatal("expected error")
	}

	errMsg := err.Error()
	// 错误信息应包含 expected 和 actual 内容
	if !strings.Contains(errMsg, "expected lines") {
		t.Errorf("error should contain expected lines, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "actual lines") {
		t.Errorf("error should contain actual lines, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "aaa") {
		t.Errorf("error should show actual content 'aaa', got: %s", errMsg)
	}
}

// TestLargeFilePartialApplyScenario 模拟大文件多 hunk 部分失败场景
func TestLargeFilePartialApplyScenario(t *testing.T) {
	// 模拟 400 行文件，21 个 hunk
	var oldLines []string
	for i := 0; i < 400; i++ {
		oldLines = append(oldLines, fmt.Sprintf("line%d", i+1))
	}

	var hunks []Hunk
	for h := 0; h < 21; h++ {
		start := h*18 + 1
		if start+2 > 400 {
			break
		}
		context := fmt.Sprintf("line%d", start)
		oldLine := fmt.Sprintf("line%d", start+1)
		endContext := fmt.Sprintf("line%d", start+2)

		// 每第7个 hunk 有错误的上下文
		if h%7 == 6 {
			context = "WRONG_CONTEXT"
		}

		hunks = append(hunks, Hunk{
			OldStart: start, OldLines: 3, NewStart: start, NewLines: 3,
			Lines: []DiffLine{
				{Kind: OpEqual, Text: context},
				{Kind: OpDelete, Text: oldLine},
				{Kind: OpInsert, Text: fmt.Sprintf("CHANGED_%d", start+1)},
				{Kind: OpEqual, Text: endContext},
			},
		})
	}

	result, err := ApplyHunks(oldLines, hunks)

	// 应该是部分成功
	pae, ok := err.(*PartialApplyError)
	if !ok {
		t.Fatalf("expected *PartialApplyError, got %T: %v", err, err)
	}

	// 大部分 hunk 应该成功（21 个中约 3 个失败）
	if pae.Applied < 15 {
		t.Errorf("expected most hunks to succeed, only %d/%d applied", pae.Applied, pae.Total)
	}

	// 验证成功的 hunk 已生效
	if result[1] != "CHANGED_2" {
		t.Errorf("first hunk should have been applied, got: %q", result[1])
	}

	t.Logf("partial apply: %d/%d hunks applied, %d failed", pae.Applied, pae.Total, len(pae.Errors))
}
