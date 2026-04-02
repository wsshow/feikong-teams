package mdiff

import (
	"fmt"
	"strings"
	"testing"
)

// ============================================================
// 1. ApplyFileDiff 基础测试
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

// ============================================================
// 2. ApplyHunks 匹配策略测试 (exact / loose / fuzzy / normalized)
// ============================================================

func TestApply_ExactMatch(t *testing.T) {
	original := "line1\nline2\nline3\nline4\nline5\n"
	patch := "--- test.go\n+++ test.go\n@@ -2,3 +2,3 @@\n line2\n-line3\n+changed3\n line4\n"
	result, err := PatchText(original, patch)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if result != "line1\nline2\nchanged3\nline4\nline5\n" {
		t.Errorf("unexpected: %q", result)
	}
}

func TestApply_LooseMatch_TrailingWhitespace(t *testing.T) {
	original := "line1  \nline2\t\nline3   \n"
	patch := "--- test.go\n+++ test.go\n@@ -1,3 +1,3 @@\n line1\n-line2\n+changed2\n line3\n"
	result, err := PatchText(original, patch)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !strings.Contains(result, "changed2") {
		t.Error("loose match should handle trailing whitespace difference")
	}
}

func TestApplyHunksFuzzyMatch(t *testing.T) {
	lines := []string{"a", "b", "old", "d", "e"}
	hunks := []Hunk{
		{
			OldStart: 10, OldLines: 3, NewStart: 10, NewLines: 3,
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

func TestApply_FuzzyMatch_ShiftedLines(t *testing.T) {
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = fmt.Sprintf("line%d", i+1)
	}
	content := strings.Join(lines, "\n") + "\n"

	patch := "--- test.go\n+++ test.go\n@@ -5,3 +5,3 @@\n line20\n-line21\n+CHANGED21\n line22\n"
	result, err := PatchText(content, patch)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !strings.Contains(result, "CHANGED21") {
		t.Error("fuzzy match should find shifted content")
	}
}

func TestApply_NormalizedMatch_IndentDiff(t *testing.T) {
	original := "func main() {\n    fmt.Println(\"hello\")\n    return\n}\n"
	patch := "--- test.go\n+++ test.go\n@@ -1,4 +1,4 @@\n func main() {\n-  fmt.Println(\"hello\")\n+  fmt.Println(\"world\")\n   return\n }\n"
	result, err := PatchText(original, patch)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !strings.Contains(result, "world") {
		t.Error("normalized match should handle indent differences")
	}
}

func TestApplyHunksContextMismatch(t *testing.T) {
	lines := []string{"a", "b", "c"}
	hunks := []Hunk{
		{
			OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1,
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
	if !strings.Contains(ae.Message, "context mismatch") {
		t.Errorf("error message should contain 'context mismatch': %s", ae.Message)
	}
}

func TestApplyErrorFormat(t *testing.T) {
	ae := &ApplyError{File: "test.go", HunkIdx: 2, Message: "context mismatch"}
	expected := "patch test.go hunk #3: context mismatch"
	if ae.Error() != expected {
		t.Errorf("got %q, want %q", ae.Error(), expected)
	}
}

func TestMatchAt(t *testing.T) {
	lines := []string{"a", "b", "c", "d"}

	if !matchAt(lines, []string{"b", "c"}, 1) {
		t.Error("expected match at pos 1")
	}
	if matchAt(lines, []string{"c", "d"}, 3) {
		t.Error("expected no match at out-of-bounds pos")
	}
	if matchAt(lines, []string{"a"}, -1) {
		t.Error("expected no match at negative pos")
	}
	if !matchAt(lines, nil, 0) {
		t.Error("expected match for empty pattern")
	}
	if matchAt(lines, []string{"x"}, 0) {
		t.Error("expected no match for wrong content")
	}
}

func TestMatchAtLoose(t *testing.T) {
	lines := []string{"hello  ", "world\t", "end"}

	if matchAt(lines, []string{"hello", "world"}, 0) {
		t.Error("strict match should fail with trailing whitespace")
	}
	if !matchAtLoose(lines, []string{"hello", "world"}, 0) {
		t.Error("loose match should succeed with trailing whitespace diff")
	}
	if matchAtLoose(lines, []string{"hello", "different"}, 0) {
		t.Error("loose match should fail for different content")
	}
}

func TestMatchAtNormalized(t *testing.T) {
	oldLines := []string{
		"    function hello() {",
		"        console.log('hello');",
		"    }",
	}
	hunks := []Hunk{
		{OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 3,
			Lines: []DiffLine{
				{Kind: OpEqual, Text: "  function hello() {"},
				{Kind: OpDelete, Text: "\tconsole.log('hello');"},
				{Kind: OpInsert, Text: "        console.log('hi');"},
				{Kind: OpEqual, Text: "  }"},
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

// ============================================================
// 3. PatchText 便捷函数测试
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
	_ = err // 不应 panic
}

func TestPatchText_NilHunks(t *testing.T) {
	result, err := PatchText("hello", "--- a\n+++ a\n")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if result != "hello" {
		t.Errorf("expected unchanged, got %q", result)
	}
}

// ============================================================
// 4. 多文件 Diff & Apply 测试
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
					OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1,
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
}

func TestApplyMultiFileDiffMultipleChanges(t *testing.T) {
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

// ============================================================
// 5. 新建 / 修改 / 删除文件测试
// ============================================================

func TestApply_AllInsertions(t *testing.T) {
	original := "line1\nline2\n"
	patch := "--- test.go\n+++ test.go\n@@ -1,2 +1,5 @@\n line1\n+added1\n+added2\n+added3\n line2\n"
	result, err := PatchText(original, patch)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if strings.Count(result, "added") != 3 {
		t.Errorf("expected 3 added lines: %q", result)
	}
}

func TestApply_AllDeletions(t *testing.T) {
	original := "line1\nline2\nline3\nline4\nline5\n"
	patch := "--- test.go\n+++ test.go\n@@ -1,5 +1,2 @@\n line1\n-line2\n-line3\n-line4\n line5\n"
	result, err := PatchText(original, patch)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if result != "line1\nline5\n" {
		t.Errorf("unexpected: %q", result)
	}
}

func TestApply_EmptyOriginal_NewFile(t *testing.T) {
	patch := "--- /dev/null\n+++ new.go\n@@ -0,0 +1,3 @@\n+package main\n+\n+func init() {}\n"
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	accessor := &memAccessor{files: map[string]string{}}
	result := ApplyMultiFileDiff(mfd, accessor)
	if result.Failed != 0 {
		t.Errorf("new file should succeed")
	}
	if !strings.Contains(accessor.files["new.go"], "func init()") {
		t.Error("new file content incorrect")
	}
}

func TestApply_DeleteFile(t *testing.T) {
	patch := "--- old.go\n+++ /dev/null\n@@ -1,2 +0,0 @@\n-package old\n-func deprecated() {}\n"
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	accessor := &memAccessor{files: map[string]string{
		"old.go": "package old\nfunc deprecated() {}\n",
	}}
	result := ApplyMultiFileDiff(mfd, accessor)
	if result.Failed != 0 {
		t.Errorf("delete should succeed, got %d failures", result.Failed)
	}
	if _, exists := accessor.files["old.go"]; exists {
		t.Error("old.go should be deleted")
	}
}

func TestApply_MultiFile_CreateModifyDelete(t *testing.T) {
	files := map[string]string{
		"existing.go": "package main\n\nfunc old() {}\n",
		"remove.go":   "package main\n\nfunc remove() {}\n",
	}

	patch := `--- /dev/null
+++ brand_new.go
@@ -0,0 +1,3 @@
+package main
+
+func brandNew() {}
--- existing.go
+++ existing.go
@@ -1,3 +1,3 @@
 package main
 
-func old() {}
+func updated() {}
--- remove.go
+++ /dev/null
@@ -1,3 +0,0 @@
-package main
-
-func remove() {}
`
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	accessor := &memAccessor{files: files}
	result := ApplyMultiFileDiff(mfd, accessor)

	if result.Failed != 0 {
		t.Errorf("expected 0 failures, got %d", result.Failed)
		for _, r := range result.Results {
			if !r.Success {
				t.Logf("  %s: %s", r.Path, r.Error)
			}
		}
	}

	if _, ok := accessor.files["brand_new.go"]; !ok {
		t.Error("brand_new.go should be created")
	}
	if !strings.Contains(accessor.files["existing.go"], "func updated()") {
		t.Error("existing.go should be modified")
	}
	if _, ok := accessor.files["remove.go"]; ok {
		t.Error("remove.go should be deleted")
	}
}

func TestApply_MultiFile_NewAndDeleteFile(t *testing.T) {
	files := map[string]string{
		"existing.go":  "old content\n",
		"delete_me.go": "old content\n",
	}

	patch := `--- /dev/null
+++ new_file.go
@@ -0,0 +1,3 @@
+package main
+
+func hello() {}
--- delete_me.go
+++ /dev/null
@@ -1,1 +0,0 @@
-old content
`
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	accessor := &memAccessor{files: files}
	result := ApplyMultiFileDiff(mfd, accessor)

	if result.Failed != 0 {
		t.Errorf("expected 0 failures, got %d", result.Failed)
		for _, r := range result.Results {
			t.Logf("  %s: success=%v err=%s", r.Path, r.Success, r.Error)
		}
	}

	if _, ok := accessor.files["new_file.go"]; !ok {
		t.Error("new_file.go should have been created")
	} else if !strings.Contains(accessor.files["new_file.go"], "func hello()") {
		t.Error("new_file.go content incorrect")
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

// ============================================================
// 6. 多 Hunk 同时修改测试
// ============================================================

func TestMultiHunkSameFile(t *testing.T) {
	var oldLines, newLines []string
	for i := 0; i < 30; i++ {
		oldLines = append(oldLines, fmt.Sprintf("line%d", i+1))
		newLines = append(newLines, fmt.Sprintf("line%d", i+1))
	}
	newLines[2] = "CHANGED_A"
	newLines[25] = "CHANGED_B"

	oldContent := strings.Join(oldLines, "\n") + "\n"
	newContent := strings.Join(newLines, "\n") + "\n"

	fd := DiffFiles("test.go", oldContent, "test.go", newContent, 3)
	if len(fd.Hunks) != 2 {
		t.Fatalf("expected 2 hunks, got %d", len(fd.Hunks))
	}

	roundTripTest(t, oldContent, newContent)
}

func TestMultiHunkThreeChanges(t *testing.T) {
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
	var oldLines, newLines []string
	for i := 0; i < 30; i++ {
		oldLines = append(oldLines, fmt.Sprintf("line%d", i+1))
	}
	for i := 0; i < 30; i++ {
		if i == 4 {
			continue
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
	lines := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}

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
	oldContent := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\n"
	newContent := "line1\nCHANGED_2\nline3\nline4\nline5\nCHANGED_6\nline7\nline8\n"
	roundTripTest(t, oldContent, newContent)
}

func TestMultiHunkWithLineCountChange(t *testing.T) {
	var oldLines, newLines []string
	for i := 0; i < 30; i++ {
		oldLines = append(oldLines, fmt.Sprintf("line%d", i+1))
		newLines = append(newLines, fmt.Sprintf("line%d", i+1))
	}
	expanded := make([]string, 0)
	for i, l := range newLines {
		expanded = append(expanded, l)
		if i == 4 {
			expanded = append(expanded, "EXTRA_A", "EXTRA_B")
		}
	}
	var finalNew []string
	for i, l := range expanded {
		if i == 26 {
			continue
		}
		finalNew = append(finalNew, l)
	}

	oldContent := strings.Join(oldLines, "\n") + "\n"
	newContent := strings.Join(finalNew, "\n") + "\n"
	roundTripTest(t, oldContent, newContent)
}

func TestMultiHunkFuzzyMatch(t *testing.T) {
	lines := []string{
		"package main", "", "import \"fmt\"", "",
		"func hello() {", "\tfmt.Println(\"hello\")", "}",
		"", "func world() {", "\tfmt.Println(\"world\")", "}",
	}
	hunks := []Hunk{
		{
			OldStart: 20, OldLines: 3, NewStart: 20, NewLines: 3,
			Lines: []DiffLine{
				{Kind: OpEqual, Text: "func hello() {"},
				{Kind: OpDelete, Text: "\tfmt.Println(\"hello\")"},
				{Kind: OpInsert, Text: "\tfmt.Println(\"hi\")"},
				{Kind: OpEqual, Text: "}"},
			},
		},
		{
			OldStart: 30, OldLines: 3, NewStart: 30, NewLines: 3,
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
	lines := []string{"func a() {  ", "\told  ", "}", "", "func b() {  ", "\told  ", "}"}
	hunks := []Hunk{
		{
			OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 3,
			Lines: []DiffLine{
				{Kind: OpEqual, Text: "func a() {"},
				{Kind: OpDelete, Text: "\told"},
				{Kind: OpInsert, Text: "\tnew_a"},
				{Kind: OpEqual, Text: "}"},
			},
		},
		{
			OldStart: 5, OldLines: 3, NewStart: 5, NewLines: 3,
			Lines: []DiffLine{
				{Kind: OpEqual, Text: "func b() {"},
				{Kind: OpDelete, Text: "\told"},
				{Kind: OpInsert, Text: "\tnew_b"},
				{Kind: OpEqual, Text: "}"},
			},
		},
	}

	result, err := ApplyHunks(lines, hunks)
	if err != nil {
		t.Fatalf("loose multi-hunk failed: %v", err)
	}

	if result[0] != "func a() {  " {
		t.Errorf("Equal line should preserve original whitespace, got %q", result[0])
	}
	if result[1] != "\tnew_a" {
		t.Errorf("Insert line should use hunk content, got %q", result[1])
	}
}

func TestApplyWithTrailingWhitespaceDiff(t *testing.T) {
	oldContent := "def func():  \n    old_code\n    return\n"
	patchText := "--- test.py\n+++ test.py\n@@ -1,3 +1,3 @@\n def func():\n-    old_code\n+    new_code\n     return\n"

	result, err := PatchText(oldContent, patchText)
	if err != nil {
		t.Fatalf("patch with trailing whitespace diff failed: %v", err)
	}
	if !strings.Contains(result, "new_code") {
		t.Errorf("expected 'new_code' in result, got %q", result)
	}
}

// ============================================================
// 7. 部分应用（Partial Apply）测试
// ============================================================

func TestPartialApplySkipsFailedHunks(t *testing.T) {
	oldLines := []string{
		"line1", "line2", "line3", "line4", "line5",
		"line6", "line7", "line8", "line9", "line10",
	}

	hunks := []Hunk{
		{OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 3,
			Lines: []DiffLine{
				{Kind: OpEqual, Text: "line1"},
				{Kind: OpDelete, Text: "line2"},
				{Kind: OpInsert, Text: "CHANGED_2"},
				{Kind: OpEqual, Text: "line3"},
			}},
		{OldStart: 5, OldLines: 3, NewStart: 5, NewLines: 3,
			Lines: []DiffLine{
				{Kind: OpEqual, Text: "WRONG_CONTEXT"},
				{Kind: OpDelete, Text: "line6"},
				{Kind: OpInsert, Text: "CHANGED_6"},
				{Kind: OpEqual, Text: "line7"},
			}},
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
	if result[1] != "CHANGED_2" {
		t.Errorf("hunk 1 should have been applied, got line2=%q", result[1])
	}
	if result[8] != "CHANGED_9" {
		t.Errorf("hunk 3 should have been applied, got line9=%q", result[8])
	}
	if result[4] != "line5" || result[5] != "line6" {
		t.Errorf("failed hunk region should be unchanged, got line5=%q line6=%q", result[4], result[5])
	}
}

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

func TestPartialApplyFileDiff(t *testing.T) {
	oldContent := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\n"

	fd := &FileDiff{
		OldName: "test.txt",
		NewName: "test.txt",
		Hunks: []Hunk{
			{OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 3,
				Lines: []DiffLine{
					{Kind: OpEqual, Text: "line1"},
					{Kind: OpDelete, Text: "line2"},
					{Kind: OpInsert, Text: "CHANGED"},
					{Kind: OpEqual, Text: "line3"},
				}},
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
	if !strings.Contains(result, "CHANGED") {
		t.Errorf("partial result should contain successful hunk change, got: %q", result)
	}
	if !strings.Contains(result, "line7") {
		t.Errorf("partial result should keep failed hunk region unchanged, got: %q", result)
	}
}

func TestApply_MultiHunk_MiddleFails(t *testing.T) {
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("line%d", i+1)
	}
	content := strings.Join(lines, "\n") + "\n"

	patch := `--- test.go
+++ test.go
@@ -2,3 +2,3 @@
 line2
-line3
+changed3
 line4
@@ -14,3 +14,3 @@
 wrong_context
-line15
+changed15
 line16
@@ -24,3 +24,3 @@
 line24
-line25
+changed25
 line26
`
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	newContent, err := ApplyFileDiff(content, &mfd.Files[0])
	if err == nil {
		t.Log("all hunks applied unexpectedly")
	} else if pae, ok := err.(*PartialApplyError); ok {
		t.Logf("partial: %d/%d applied", pae.Applied, pae.Total)
	}

	if !strings.Contains(newContent, "changed3") {
		t.Error("hunk1 (line3->changed3) should have been applied")
	}
	if !strings.Contains(newContent, "changed25") {
		t.Error("hunk3 (line25->changed25) should have been applied")
	}
	if strings.Contains(newContent, "changed15") {
		t.Error("hunk2 should not have been applied (bad context)")
	}
}

func TestApply_MultiFilePatch_MixedResults(t *testing.T) {
	files := map[string]string{
		"ok.go":   "line1\nline2\nline3\n",
		"fail.go": "completely\ndifferent\ncontent\n",
		"ok2.go":  "aaa\nbbb\nccc\n",
	}

	patch := `--- ok.go
+++ ok.go
@@ -1,3 +1,3 @@
 line1
-line2
+changed2
 line3
--- fail.go
+++ fail.go
@@ -1,3 +1,3 @@
 wrong_context
-wrong_line
+new_line
 wrong_end
--- ok2.go
+++ ok2.go
@@ -1,3 +1,3 @@
 aaa
-bbb
+BBB
 ccc
`
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	accessor := &memAccessor{files: files}
	result := ApplyMultiFileDiff(mfd, accessor)

	if result.TotalFiles != 3 {
		t.Errorf("expected 3 total, got %d", result.TotalFiles)
	}
	if result.Succeeded != 2 {
		t.Errorf("expected 2 succeeded, got %d", result.Succeeded)
	}
	if result.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", result.Failed)
	}
}

func TestApply_MultiFile_SequentialApply(t *testing.T) {
	files := map[string]string{
		"a.go": "line1\nline2\nline3\n",
		"b.go": "lineA\nlineB\nlineC\n",
	}

	patch := `--- a.go
+++ a.go
@@ -1,3 +1,3 @@
 line1
-line2
+changed
 line3
--- b.go
+++ b.go
@@ -1,3 +1,3 @@
 wrong_context
-lineB
+changed
 lineC
`
	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	accessor := &memAccessor{files: files}
	result := ApplyMultiFileDiff(mfd, accessor)

	if result.Succeeded != 1 {
		t.Errorf("expected 1 success, got %d", result.Succeeded)
	}
	if accessor.files["a.go"] != "line1\nchanged\nline3\n" {
		t.Errorf("file a.go should be modified: got %q", accessor.files["a.go"])
	}
}

// ============================================================
// 8. 错误诊断测试
// ============================================================

func TestErrorDiagnosticContainsActualContent(t *testing.T) {
	oldLines := []string{"aaa", "bbb", "ccc"}

	hunks := []Hunk{
		{OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 2,
			Lines: []DiffLine{
				{Kind: OpEqual, Text: "xxx"},
				{Kind: OpDelete, Text: "yyy"},
				{Kind: OpInsert, Text: "new"},
			}},
	}

	_, err := ApplyHunks(oldLines, hunks)
	if err == nil {
		t.Fatal("expected error")
	}

	errMsg := err.Error()
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

// ============================================================
// 9. 大文件 & 压力测试
// ============================================================

func TestApply_LargeFile_100Lines(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = fmt.Sprintf("line_%03d", i+1)
	}
	original := strings.Join(lines, "\n") + "\n"

	newLines := make([]string, len(lines))
	copy(newLines, lines)
	newLines[2] = "CHANGED_003"
	newLines[49] = "CHANGED_050"
	newLines[97] = "CHANGED_098"
	expected := strings.Join(newLines, "\n") + "\n"

	fd := DiffFiles("big.go", original, "big.go", expected, 3)
	patchText := FormatFileDiff(fd)

	result, err := PatchText(original, patchText)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if result != expected {
		t.Error("large file roundtrip failed")
	}
}

func TestApply_RepeatedContextLines(t *testing.T) {
	original := "{\n}\n\n{\n}\n\n{\n  target\n}\n\n{\n}\n"
	patch := "--- test.go\n+++ test.go\n@@ -7,3 +7,3 @@\n {\n-  target\n+  replaced\n }\n"
	result, err := PatchText(original, patch)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !strings.Contains(result, "replaced") {
		t.Error("should replace the correct occurrence")
	}
	if strings.Count(result, "replaced") != 1 {
		t.Errorf("should replace exactly 1 occurrence, got %d", strings.Count(result, "replaced"))
	}
}

func TestLargeFilePartialApplyScenario(t *testing.T) {
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
	pae, ok := err.(*PartialApplyError)
	if !ok {
		t.Fatalf("expected *PartialApplyError, got %T: %v", err, err)
	}

	if pae.Applied < 15 {
		t.Errorf("expected most hunks to succeed, only %d/%d applied", pae.Applied, pae.Total)
	}
	if result[1] != "CHANGED_2" {
		t.Errorf("first hunk should have been applied, got: %q", result[1])
	}

	t.Logf("partial apply: %d/%d hunks applied, %d failed", pae.Applied, pae.Total, len(pae.Errors))
}

// ============================================================
// 10. LLM 特有边界测试
// ============================================================

func TestApply_FuzzyMatch_ShiftedLineNumbers(t *testing.T) {
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = fmt.Sprintf("line%d", i+1)
	}
	content := strings.Join(lines, "\n") + "\n"

	patch := `--- test.go
+++ test.go
@@ -5,3 +5,3 @@
 line10
-line11
+changed11
 line12
`
	result, err := PatchText(content, patch)
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !strings.Contains(result, "changed11") {
		t.Error("fuzzy match should have found the correct position")
	}
}

func TestApply_WhitespaceMismatch(t *testing.T) {
	original := "\tline1\n\told_value\n\tline3\n"
	patch := "--- test.go\n+++ test.go\n@@ -1,3 +1,3 @@\n \tline1\n-\told_value\n+\tnew_value\n \tline3\n"
	result, err := PatchText(original, patch)
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !strings.Contains(result, "new_value") {
		t.Error("whitespace mismatch should be handled by loose/normalized matching")
	}
}

func TestLLM_ContextLineStartsWithMinusOrPlus(t *testing.T) {
	original := "x = a + b\n-1 if error\nresult\n"
	patch := `--- test.go
+++ test.go
@@ -1,3 +1,3 @@
 x = a + b
 -1 if error
-result
+new_result
`
	result, err := PatchText(original, patch)
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !strings.Contains(result, "-1 if error") {
		t.Error("context line starting with '-' should be preserved")
	}
	if !strings.Contains(result, "new_result") {
		t.Error("replacement should be applied")
	}
}

func TestLLM_LargeMultiFilePatch(t *testing.T) {
	files := make(map[string]string)
	var patchParts []string

	for fileIdx := 0; fileIdx < 5; fileIdx++ {
		name := fmt.Sprintf("file%d.go", fileIdx)
		lines := make([]string, 50)
		for i := range lines {
			lines[i] = fmt.Sprintf("file%d_line%d", fileIdx, i+1)
		}
		files[name] = strings.Join(lines, "\n") + "\n"

		patchParts = append(patchParts, fmt.Sprintf(`--- %s
+++ %s
@@ -9,3 +9,3 @@
 file%d_line9
-file%d_line10
+file%d_line10_changed
 file%d_line11
@@ -29,3 +29,3 @@
 file%d_line29
-file%d_line30
+file%d_line30_changed
 file%d_line31`, name, name,
			fileIdx, fileIdx, fileIdx, fileIdx,
			fileIdx, fileIdx, fileIdx, fileIdx))
	}

	patch := strings.Join(patchParts, "\n") + "\n"

	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(mfd.Files) != 5 {
		t.Fatalf("expected 5 files, got %d", len(mfd.Files))
	}

	accessor := &memAccessor{files: files}
	result := ApplyMultiFileDiff(mfd, accessor)

	if result.Failed != 0 {
		t.Errorf("expected 0 failures, got %d", result.Failed)
		for _, r := range result.Results {
			if !r.Success {
				t.Logf("  %s: %s", r.Path, r.Error)
			}
		}
	}

	for fileIdx := 0; fileIdx < 5; fileIdx++ {
		name := fmt.Sprintf("file%d.go", fileIdx)
		content := accessor.files[name]
		if !strings.Contains(content, fmt.Sprintf("file%d_line10_changed", fileIdx)) {
			t.Errorf("%s: line10 not changed", name)
		}
		if !strings.Contains(content, fmt.Sprintf("file%d_line30_changed", fileIdx)) {
			t.Errorf("%s: line30 not changed", name)
		}
	}
}

func TestLLM_EmptyContextLine(t *testing.T) {
	original := "func a() {}\n\nfunc b() {}\n"
	patch := `--- test.go
+++ test.go
@@ -1,3 +1,3 @@
 func a() {}
 
-func b() {}
+func c() {}
`
	result, err := PatchText(original, patch)
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !strings.Contains(result, "func c()") {
		t.Errorf("empty context line not handled: got %q", result)
	}
}

// ============================================================
// 11. splitLines 测试
// ============================================================

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", -1},
		{"\n", -1},
		{"a", 1},
		{"a\n", 1},
		{"a\nb", 2},
		{"a\nb\n", 2},
		{"a\n\nb\n", 3},
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
