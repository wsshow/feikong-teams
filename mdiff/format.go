package mdiff

import (
	"fmt"
	"strings"
)

// DiffLine 表示 unified diff 中的一行
type DiffLine struct {
	Kind OpKind
	Text string
}

// Hunk 表示 unified diff 中的一个变更块
type Hunk struct {
	OldStart int        // 旧文件起始行号（从1开始）
	OldLines int        // 旧文件中的行数
	NewStart int        // 新文件起始行号（从1开始）
	NewLines int        // 新文件中的行数
	Lines    []DiffLine // 变更行
}

// FileDiff 表示单个文件的 diff 结果
type FileDiff struct {
	OldName string // 旧文件名
	NewName string // 新文件名
	Hunks   []Hunk // 变更块列表
}

// MultiFileDiff 表示多个文件的 diff 结果
type MultiFileDiff struct {
	Files []FileDiff
}

// defaultContextLines 默认上下文行数
const defaultContextLines = 3

// UnifiedDiff 计算两个文件版本之间的 unified diff
// contextLines 指定每个 hunk 前后保留的上下文行数，0 表示使用默认值(3)
func UnifiedDiff(oldName, newName string, oldLines, newLines []string, contextLines int) *FileDiff {
	if contextLines <= 0 {
		contextLines = defaultContextLines
	}

	edits := Diff(oldLines, newLines)
	if len(edits) == 0 {
		return &FileDiff{
			OldName: oldName,
			NewName: newName,
		}
	}

	// 检查是否有任何变更
	hasChanges := false
	for _, e := range edits {
		if e.Kind != OpEqual {
			hasChanges = true
			break
		}
	}
	if !hasChanges {
		return &FileDiff{
			OldName: oldName,
			NewName: newName,
		}
	}

	hunks := buildHunks(edits, contextLines)

	return &FileDiff{
		OldName: oldName,
		NewName: newName,
		Hunks:   hunks,
	}
}

// buildHunks 从编辑序列中构建 hunk 列表
func buildHunks(edits []Edit, contextLines int) []Hunk {
	n := len(edits)
	if n == 0 {
		return nil
	}

	// 标记每个 edit 是否需要包含在输出中
	include := make([]bool, n)

	// 先标记所有变更行
	for i, e := range edits {
		if e.Kind != OpEqual {
			include[i] = true
		}
	}

	// 向前后扩展上下文
	for i := 0; i < n; i++ {
		if edits[i].Kind != OpEqual {
			// 向前扩展
			for j := i - 1; j >= 0 && j >= i-contextLines; j-- {
				include[j] = true
			}
			// 向后扩展
			for j := i + 1; j < n && j <= i+contextLines; j++ {
				include[j] = true
			}
		}
	}

	// 将连续的 include 区域分组为 hunk
	var hunks []Hunk
	i := 0
	for i < n {
		if !include[i] {
			i++
			continue
		}

		// 找到连续 include 区域
		start := i
		for i < n && include[i] {
			i++
		}

		// 构建 hunk
		var lines []DiffLine
		oldCount := 0
		newCount := 0

		// 确定 hunk 起始行号
		hunkOldStart := -1
		hunkNewStart := -1

		for idx := start; idx < i; idx++ {
			e := edits[idx]
			switch e.Kind {
			case OpEqual:
				if hunkOldStart == -1 {
					hunkOldStart = e.OldPos
				}
				if hunkNewStart == -1 {
					hunkNewStart = e.NewPos
				}
				lines = append(lines, DiffLine{Kind: OpEqual, Text: e.Text})
				oldCount++
				newCount++
			case OpDelete:
				if hunkOldStart == -1 {
					hunkOldStart = e.OldPos
				}
				lines = append(lines, DiffLine{Kind: OpDelete, Text: e.Text})
				oldCount++
			case OpInsert:
				if hunkNewStart == -1 {
					hunkNewStart = e.NewPos
				}
				lines = append(lines, DiffLine{Kind: OpInsert, Text: e.Text})
				newCount++
			}
		}

		// 如果 hunkOldStart 没设置（全是插入），从相邻 edit 推导
		if hunkOldStart == -1 {
			if start > 0 && (edits[start-1].Kind == OpEqual || edits[start-1].Kind == OpDelete) {
				hunkOldStart = edits[start-1].OldPos + 1
			} else {
				hunkOldStart = 0
			}
		}
		if hunkNewStart == -1 {
			if start > 0 && (edits[start-1].Kind == OpEqual || edits[start-1].Kind == OpInsert) {
				hunkNewStart = edits[start-1].NewPos + 1
			} else {
				hunkNewStart = 0
			}
		}

		// 转为 1-based；当行数为 0 时（新建/删除文件），起始行号应为 0
		hunkOldStartFinal := hunkOldStart + 1
		hunkNewStartFinal := hunkNewStart + 1
		if oldCount == 0 {
			hunkOldStartFinal = 0
		}
		if newCount == 0 {
			hunkNewStartFinal = 0
		}

		hunks = append(hunks, Hunk{
			OldStart: hunkOldStartFinal,
			OldLines: oldCount,
			NewStart: hunkNewStartFinal,
			NewLines: newCount,
			Lines:    lines,
		})
	}

	return hunks
}

// FormatFileDiff 将单个文件 diff 格式化为 unified diff 字符串
func FormatFileDiff(fd *FileDiff) string {
	if fd == nil || len(fd.Hunks) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- %s\n", formatFileName(fd.OldName)))
	sb.WriteString(fmt.Sprintf("+++ %s\n", formatFileName(fd.NewName)))

	for _, h := range fd.Hunks {
		sb.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", h.OldStart, h.OldLines, h.NewStart, h.NewLines))
		for _, line := range h.Lines {
			switch line.Kind {
			case OpEqual:
				sb.WriteString(" " + line.Text + "\n")
			case OpDelete:
				sb.WriteString("-" + line.Text + "\n")
			case OpInsert:
				sb.WriteString("+" + line.Text + "\n")
			}
		}
	}

	return sb.String()
}

// FormatMultiFileDiff 将多文件 diff 格式化为 unified diff 字符串
func FormatMultiFileDiff(mfd *MultiFileDiff) string {
	if mfd == nil || len(mfd.Files) == 0 {
		return ""
	}

	var sb strings.Builder
	for i := range mfd.Files {
		sb.WriteString(FormatFileDiff(&mfd.Files[i]))
	}

	return sb.String()
}

// formatFileName 格式化文件名（处理特殊情况）
func formatFileName(name string) string {
	if name == "" {
		return "/dev/null"
	}
	return name
}
