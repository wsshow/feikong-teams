package mdiff

import (
	"fmt"
	"strings"
	"testing"
)

// ============================================================
// Benchmarks — 基准性能测试
// ============================================================

func BenchmarkDiffSmall(b *testing.B) {
	oldLines := []string{"a", "b", "c", "d", "e"}
	newLines := []string{"a", "x", "c", "d", "y"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Diff(oldLines, newLines)
	}
}

func BenchmarkDiffLarge(b *testing.B) {
	var oldLines, newLines []string
	for i := 0; i < 1000; i++ {
		oldLines = append(oldLines, fmt.Sprintf("line %d original", i))
		newLines = append(newLines, fmt.Sprintf("line %d original", i))
	}
	// 散布 10 处修改
	for _, idx := range []int{50, 150, 250, 350, 450, 550, 650, 750, 850, 950} {
		newLines[idx] = fmt.Sprintf("line %d CHANGED", idx)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Diff(oldLines, newLines)
	}
}

func BenchmarkRoundTripSmall(b *testing.B) {
	oldContent := "line1\nline2\nline3\nline4\nline5\n"
	newContent := "line1\nchanged\nline3\nline4\nline5\n"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fd := DiffFiles("f.txt", oldContent, "f.txt", newContent, 3)
		patchStr := FormatFileDiff(fd)
		parsed, _ := ParseFileDiff(patchStr)
		ApplyFileDiff(oldContent, parsed)
		_ = parsed
	}
}

func BenchmarkRoundTripMultiHunk(b *testing.B) {
	var oldLines, newLines []string
	for i := 0; i < 200; i++ {
		oldLines = append(oldLines, fmt.Sprintf("line%d", i+1))
		newLines = append(newLines, fmt.Sprintf("line%d", i+1))
	}
	newLines[10] = "CHANGED_A"
	newLines[50] = "CHANGED_B"
	newLines[100] = "CHANGED_C"
	newLines[150] = "CHANGED_D"
	oldContent := strings.Join(oldLines, "\n") + "\n"
	newContent := strings.Join(newLines, "\n") + "\n"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fd := DiffFiles("f.txt", oldContent, "f.txt", newContent, 3)
		patchStr := FormatFileDiff(fd)
		parsed, _ := ParseFileDiff(patchStr)
		ApplyFileDiff(oldContent, parsed)
		_ = parsed
	}
}

func BenchmarkApplyHunksForward(b *testing.B) {
	var oldLines []string
	for i := 0; i < 500; i++ {
		oldLines = append(oldLines, fmt.Sprintf("line%d", i+1))
	}
	hunks := []Hunk{
		{OldStart: 10, OldLines: 3, NewStart: 10, NewLines: 3,
			Lines: []DiffLine{
				{Kind: OpEqual, Text: "line10"},
				{Kind: OpDelete, Text: "line11"},
				{Kind: OpInsert, Text: "CHANGED_11"},
				{Kind: OpEqual, Text: "line12"},
			}},
		{OldStart: 100, OldLines: 3, NewStart: 100, NewLines: 3,
			Lines: []DiffLine{
				{Kind: OpEqual, Text: "line100"},
				{Kind: OpDelete, Text: "line101"},
				{Kind: OpInsert, Text: "CHANGED_101"},
				{Kind: OpEqual, Text: "line102"},
			}},
		{OldStart: 300, OldLines: 3, NewStart: 300, NewLines: 3,
			Lines: []DiffLine{
				{Kind: OpEqual, Text: "line300"},
				{Kind: OpDelete, Text: "line301"},
				{Kind: OpInsert, Text: "CHANGED_301"},
				{Kind: OpEqual, Text: "line302"},
			}},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ApplyHunks(oldLines, hunks)
	}
}
