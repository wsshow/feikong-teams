// Package mdiff 提供多文件 diff 和 patch 功能。
//
// 核心能力:
//   - Diff: 基于 Myers 算法计算两个文本之间的最小编辑距离
//   - Format: 生成标准 unified diff 格式输出
//   - Parse: 解析 unified diff 格式文本
//   - Patch: 将 diff 应用到原始文件（支持模糊匹配）
//   - 多文件: 批量 diff/patch 操作
//
// 用法示例:
//
//	// 计算单文件 diff
//	fd := mdiff.DiffFiles("old.go", oldContent, "new.go", newContent, 3)
//	patch := mdiff.FormatFileDiff(fd)
//
//	// 应用 patch
//	newContent, err := mdiff.ApplyFileDiff(oldContent, fd)
//
//	// 多文件 diff
//	changes := []mdiff.FileChange{
//	    {Path: "a.go", OldContent: old1, NewContent: new1},
//	    {Path: "b.go", OldContent: old2, NewContent: new2},
//	}
//	mfd := mdiff.DiffMultiFiles(changes, 3)
//	patch := mdiff.FormatMultiFileDiff(mfd)
//
//	// 解析并应用多文件 patch
//	mfd, _ := mdiff.ParseMultiFileDiff(patchText)
//	result := mdiff.ApplyMultiFileDiff(mfd, accessor)
package mdiff

import (
	"fmt"
	"strings"
)

// FileChange 描述一个文件的变更（用于批量 diff）
type FileChange struct {
	Path       string // 文件路径
	OldContent string // 旧内容（空字符串=新文件）
	NewContent string // 新内容（空字符串=删除文件）
}

// DiffFiles 计算两个文件版本之间的 diff
// contextLines 为上下文行数，0 表示默认值(3)
func DiffFiles(oldName, oldContent, newName, newContent string, contextLines int) *FileDiff {
	oldLines := splitLines(oldContent)
	newLines := splitLines(newContent)
	return UnifiedDiff(oldName, newName, oldLines, newLines, contextLines)
}

// DiffMultiFiles 批量计算多个文件的 diff
func DiffMultiFiles(changes []FileChange, contextLines int) *MultiFileDiff {
	mfd := &MultiFileDiff{}
	for _, c := range changes {
		oldName := c.Path
		newName := c.Path
		if c.OldContent == "" {
			oldName = "/dev/null"
		}
		if c.NewContent == "" {
			newName = "/dev/null"
		}
		fd := DiffFiles(oldName, c.OldContent, newName, c.NewContent, contextLines)
		if fd != nil && len(fd.Hunks) > 0 {
			mfd.Files = append(mfd.Files, *fd)
		}
	}
	return mfd
}

// PatchText 直接用 patch 文本修改原始内容
// 这是最便捷的单文件 patch API
func PatchText(original, patchText string) (string, error) {
	fd, err := ParseFileDiff(patchText)
	if err != nil {
		return "", err
	}
	return ApplyFileDiff(original, fd)
}

// DiffStat 返回 diff 的统计信息
type DiffStat struct {
	FilesChanged int // 变更文件数
	Insertions   int // 新增行数
	Deletions    int // 删除行数
}

func (s DiffStat) String() string {
	var parts []string
	parts = append(parts, pluralize(s.FilesChanged, "file changed", "files changed"))
	if s.Insertions > 0 {
		parts = append(parts, pluralize(s.Insertions, "insertion(+)", "insertions(+)"))
	}
	if s.Deletions > 0 {
		parts = append(parts, pluralize(s.Deletions, "deletion(-)", "deletions(-)"))
	}
	return strings.Join(parts, ", ")
}

// Stat 计算多文件 diff 的统计信息
func Stat(mfd *MultiFileDiff) DiffStat {
	stat := DiffStat{
		FilesChanged: len(mfd.Files),
	}
	for _, fd := range mfd.Files {
		for _, h := range fd.Hunks {
			for _, dl := range h.Lines {
				switch dl.Kind {
				case OpInsert:
					stat.Insertions++
				case OpDelete:
					stat.Deletions++
				}
			}
		}
	}
	return stat
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %s", n, plural)
}
