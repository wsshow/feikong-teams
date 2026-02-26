package mdiff

import (
	"fmt"
	"sort"
	"strings"
)

// ApplyError 表示应用 patch 时的错误
type ApplyError struct {
	File    string // 文件名
	HunkIdx int    // 出错的 hunk 索引（从0开始）
	Message string // 错误信息
}

func (e *ApplyError) Error() string {
	return fmt.Sprintf("patch %s hunk #%d: %s", e.File, e.HunkIdx+1, e.Message)
}

// PartialApplyError 表示 patch 部分应用成功
type PartialApplyError struct {
	File    string       // 文件名
	Applied int          // 成功应用的 hunk 数量
	Total   int          // 总 hunk 数量
	Errors  []ApplyError // 失败的 hunk 详情
}

func (e *PartialApplyError) Error() string {
	msgs := make([]string, 0, len(e.Errors))
	for i := range e.Errors {
		msgs = append(msgs, e.Errors[i].Message)
	}
	return fmt.Sprintf("partial apply %s: %d/%d hunks applied, failures: %s",
		e.File, e.Applied, e.Total, strings.Join(msgs, "; "))
}

// maxFuzz 模糊搜索的最大偏移行数
const maxFuzz = 100

// ApplyHunks 将一个文件的 hunks 应用到原始行上
// 采用多策略：先尝试精确前向合并，再尝试宽松前向合并，最后回退到模糊逐 hunk 应用
func ApplyHunks(oldLines []string, hunks []Hunk) ([]string, error) {
	if len(hunks) == 0 {
		return oldLines, nil
	}

	// 按 OldStart 排序，确保有序处理（单 hunk 时跳过排序）
	sorted := hunks
	if len(hunks) > 1 {
		sorted = sortHunksByOldStart(hunks)
	}

	// 策略1: 精确前向合并（最可靠，适用于标准 diff）
	if result, err := applyHunksForward(oldLines, sorted, false); err == nil {
		return result, nil
	}

	// 策略2: 宽松前向合并（忽略行尾空白，适用于 LLM 生成的 diff）
	if result, err := applyHunksForward(oldLines, sorted, true); err == nil {
		return result, nil
	}

	// 策略3: 模糊逐 hunk 应用（处理行号偏移严重的情况）
	return applyHunksFuzzy(oldLines, sorted)
}

// sortHunksByOldStart 按 OldStart 排序 hunks（不修改原 slice）
func sortHunksByOldStart(hunks []Hunk) []Hunk {
	sorted := make([]Hunk, len(hunks))
	copy(sorted, hunks)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].OldStart < sorted[j].OldStart
	})
	return sorted
}

// applyHunksForward 前向合并策略：按顺序处理每个 hunk，逐段拼接原始行和变更行
// 对于 Equal 行使用原文件内容（保留原始空白），对于 Insert 行使用 hunk 内容
// loose=true 时使用宽松匹配（忽略行尾空白）
func applyHunksForward(oldLines []string, hunks []Hunk, loose bool) ([]string, error) {
	// 估算结果容量：原始行数 + 净增加量
	capacity := len(oldLines)
	for i := range hunks {
		capacity += hunks[i].NewLines - hunks[i].OldLines
	}
	if capacity < len(oldLines) {
		capacity = len(oldLines)
	}
	result := make([]string, 0, capacity)
	lastOldEnd := 0 // 上一个 hunk 消费到的位置（0-based）

	for i := range hunks {
		hunk := &hunks[i]
		startIdx := hunk.OldStart - 1 // 转为 0-based
		if startIdx < 0 {
			startIdx = 0
		}

		// 检查 hunk 重叠
		if startIdx < lastOldEnd {
			return nil, fmt.Errorf("hunk #%d overlaps with previous hunk (start=%d, prev_end=%d)", i+1, startIdx+1, lastOldEnd)
		}

		// 内联验证上下文匹配（避免额外分配 extractOldLines）
		oldCount := 0
		for _, dl := range hunk.Lines {
			if dl.Kind == OpEqual || dl.Kind == OpDelete {
				oldCount++
			}
		}
		if startIdx+oldCount > len(oldLines) {
			return nil, fmt.Errorf("hunk #%d: out of range (start=%d, old_lines=%d, file_lines=%d)", i+1, hunk.OldStart, oldCount, len(oldLines))
		}
		// 验证 hunk 中的旧行与原文件匹配
		oldIdx := startIdx
		for _, dl := range hunk.Lines {
			if dl.Kind != OpEqual && dl.Kind != OpDelete {
				continue
			}
			if loose {
				if strings.TrimRight(oldLines[oldIdx], " \t") != strings.TrimRight(dl.Text, " \t") {
					return nil, fmt.Errorf("hunk #%d: loose context mismatch at line %d", i+1, oldIdx+1)
				}
			} else {
				if oldLines[oldIdx] != dl.Text {
					return nil, fmt.Errorf("hunk #%d: context mismatch at line %d", i+1, oldIdx+1)
				}
			}
			oldIdx++
		}

		// 拷贝 hunks 之间未变更的行
		result = append(result, oldLines[lastOldEnd:startIdx]...)

		// 应用 hunk 变更：Equal 行从原文件复制（保留原始空白），Insert 行从 hunk 取
		oldIdx = startIdx
		for _, dl := range hunk.Lines {
			switch dl.Kind {
			case OpEqual:
				result = append(result, oldLines[oldIdx])
				oldIdx++
			case OpDelete:
				oldIdx++
			case OpInsert:
				result = append(result, dl.Text)
			}
		}

		lastOldEnd = startIdx + oldCount
	}

	// 拷贝最后一个 hunk 之后的剩余行
	if lastOldEnd < len(oldLines) {
		result = append(result, oldLines[lastOldEnd:]...)
	}

	return result, nil
}

// applyHunksFuzzy 模糊逐 hunk 应用（处理行号不准的场景）
// 从后往前应用，避免行号偏移影响前面的 hunk
// 采用部分应用模式：跳过无法匹配的 hunk，尽可能多地应用成功的 hunk
func applyHunksFuzzy(oldLines []string, hunks []Hunk) ([]string, error) {
	result := make([]string, len(oldLines))
	copy(result, oldLines)

	var failures []ApplyError
	for i := len(hunks) - 1; i >= 0; i-- {
		newResult, err := applyOneHunk(result, &hunks[i], i)
		if err != nil {
			if ae, ok := err.(*ApplyError); ok {
				failures = append(failures, *ae)
			} else {
				failures = append(failures, ApplyError{HunkIdx: i, Message: err.Error()})
			}
			continue
		}
		result = newResult
	}

	applied := len(hunks) - len(failures)

	if applied == 0 {
		// 全部失败，返回第一个（行号最小的）hunk 的错误
		return nil, &failures[len(failures)-1]
	}

	if len(failures) > 0 {
		return result, &PartialApplyError{
			Applied: applied,
			Total:   len(hunks),
			Errors:  failures,
		}
	}

	return result, nil
}

// extractOldLines 提取 hunk 中旧文件的行（Equal + Delete）
func extractOldLines(hunk *Hunk) []string {
	lines := make([]string, 0, hunk.OldLines)
	for _, dl := range hunk.Lines {
		if dl.Kind == OpEqual || dl.Kind == OpDelete {
			lines = append(lines, dl.Text)
		}
	}
	return lines
}

// applyOneHunk 应用一个 hunk 到行数组（模糊匹配模式）
func applyOneHunk(lines []string, hunk *Hunk, hunkIdx int) ([]string, error) {
	oldHunkLines := extractOldLines(hunk)

	// 尝试精确位置匹配
	startIdx := hunk.OldStart - 1 // 转为0-based
	if startIdx < 0 {
		startIdx = 0
	}

	// 多级匹配策略：精确 → 宽松(行尾空白) → 归一化(前后空白)
	matchPos := searchMatch(lines, oldHunkLines, startIdx, matchAt)
	if matchPos < 0 && len(oldHunkLines) > 0 {
		matchPos = searchMatch(lines, oldHunkLines, startIdx, matchAtLoose)
	}
	if matchPos < 0 && len(oldHunkLines) > 0 {
		matchPos = searchMatch(lines, oldHunkLines, startIdx, matchAtNormalized)
	}

	if matchPos < 0 {
		// 构建增强版错误信息：包含期望内容和实际内容的对比
		var parts []string

		// 期望的上下文行
		if len(oldHunkLines) > 0 {
			showLines := oldHunkLines
			if len(showLines) > 3 {
				showLines = showLines[:3]
			}
			parts = append(parts, fmt.Sprintf("expected lines:\n%s", formatIndented(showLines)))
		}

		// 文件该位置的实际内容
		if startIdx >= 0 && startIdx < len(lines) {
			endIdx := startIdx + len(oldHunkLines)
			if endIdx > len(lines) {
				endIdx = len(lines)
			}
			actualLines := lines[startIdx:endIdx]
			if len(actualLines) > 3 {
				actualLines = actualLines[:3]
			}
			if len(actualLines) > 0 {
				parts = append(parts, fmt.Sprintf("actual lines at %d:\n%s", hunk.OldStart, formatIndented(actualLines)))
			}
		}

		totalLines := len(lines)
		detail := ""
		if len(parts) > 0 {
			detail = ", " + strings.Join(parts, ", ")
		}
		return nil, &ApplyError{
			HunkIdx: hunkIdx,
			Message: fmt.Sprintf("context mismatch at line %d (file has %d lines, searched +/-%d lines)%s",
				hunk.OldStart, totalLines, maxFuzz, detail),
		}
	}

	// 构建新内容
	newHunkLines := make([]string, 0, hunk.NewLines)
	for _, dl := range hunk.Lines {
		if dl.Kind == OpEqual || dl.Kind == OpInsert {
			newHunkLines = append(newHunkLines, dl.Text)
		}
	}

	// 替换匹配区域
	resultLen := len(lines) - len(oldHunkLines) + len(newHunkLines)
	result := make([]string, 0, resultLen)
	result = append(result, lines[:matchPos]...)
	result = append(result, newHunkLines...)
	result = append(result, lines[matchPos+len(oldHunkLines):]...)

	return result, nil
}

// formatIndented 格式化行列表用于错误信息展示（每行缩进显示）
func formatIndented(lines []string) string {
	var sb strings.Builder
	for i, line := range lines {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString("            ")
		sb.WriteString(line)
	}
	return sb.String()
}

// searchMatch 在 startIdx 附近查找匹配位置，先尝试原位，再向前后扩展搜索
func searchMatch(lines, pattern []string, startIdx int, matcher func([]string, []string, int) bool) int {
	if matcher(lines, pattern, startIdx) {
		return startIdx
	}
	// 设置搜索上界，避免无效的越界尝试
	maxPos := len(lines) - len(pattern)
	for offset := 1; offset <= maxFuzz; offset++ {
		if pos := startIdx - offset; pos >= 0 && matcher(lines, pattern, pos) {
			return pos
		}
		if pos := startIdx + offset; pos <= maxPos && matcher(lines, pattern, pos) {
			return pos
		}
	}
	return -1
}

// matchAt 检查 lines[pos:] 是否与 pattern 精确匹配
func matchAt(lines, pattern []string, pos int) bool {
	if pos < 0 || pos+len(pattern) > len(lines) {
		return false
	}
	for i, p := range pattern {
		if lines[pos+i] != p {
			return false
		}
	}
	return true
}

// matchAtLoose 检查 lines[pos:] 是否与 pattern 宽松匹配（忽略行尾空白）
// 用于处理 LLM 生成的 patch 中空白字符微小差异的情况
func matchAtLoose(lines, pattern []string, pos int) bool {
	if pos < 0 || pos+len(pattern) > len(lines) {
		return false
	}
	for i, p := range pattern {
		if strings.TrimRight(lines[pos+i], " \t") != strings.TrimRight(p, " \t") {
			return false
		}
	}
	return true
}

// matchAtNormalized 归一化匹配（忽略前导和尾部空白）
// 处理 LLM 缩进不一致的场景（如 tab/space 混用、缩进级别偏差）
func matchAtNormalized(lines, pattern []string, pos int) bool {
	if pos < 0 || pos+len(pattern) > len(lines) {
		return false
	}
	for i, p := range pattern {
		if strings.TrimSpace(lines[pos+i]) != strings.TrimSpace(p) {
			return false
		}
	}
	return true
}

// ApplyFileDiff 将单个文件 diff 应用到文本内容
func ApplyFileDiff(content string, fd *FileDiff) (string, error) {
	if fd == nil || len(fd.Hunks) == 0 {
		return content, nil
	}

	lines := splitLines(content)
	newLines, err := ApplyHunks(lines, fd.Hunks)
	if err != nil {
		// 处理部分应用：返回部分修改后的内容 + 错误
		if pae, ok := err.(*PartialApplyError); ok {
			pae.File = fd.OldName
			for i := range pae.Errors {
				pae.Errors[i].File = fd.OldName
			}
			result := strings.Join(newLines, "\n")
			if len(content) > 0 && content[len(content)-1] == '\n' {
				result += "\n"
			}
			return result, pae
		}

		if ae, ok := err.(*ApplyError); ok {
			ae.File = fd.OldName
		}
		return "", err
	}

	result := strings.Join(newLines, "\n")
	// 保留原始文件的尾部换行
	if len(content) > 0 && content[len(content)-1] == '\n' {
		result += "\n"
	}

	return result, nil
}

// FileAccessor 文件访问接口，用于多文件 patch 操作
type FileAccessor interface {
	ReadFile(path string) (string, error)
	WriteFile(path string, content string) error
	DeleteFile(path string) error
}

// FileResult 单个文件的 patch 结果
type FileResult struct {
	Path    string // 文件路径
	Success bool   // 是否成功
	Error   string // 错误信息（如果失败）
	Warning string // 警告信息（如部分应用成功）
}

// PatchResult 多文件 patch 的总结果
type PatchResult struct {
	Results    []FileResult // 每个文件的结果
	TotalFiles int          // 总文件数
	Succeeded  int          // 成功数
	Failed     int          // 失败数
}

// ApplyMultiFileDiff 将多文件 diff 应用到文件系统
func ApplyMultiFileDiff(mfd *MultiFileDiff, accessor FileAccessor) *PatchResult {
	if mfd == nil || len(mfd.Files) == 0 {
		return &PatchResult{}
	}

	pr := &PatchResult{
		TotalFiles: len(mfd.Files),
	}

	for _, fd := range mfd.Files {
		filePath := fd.NewName
		if filePath == "" || filePath == "/dev/null" {
			filePath = fd.OldName
		}

		// 处理文件删除（新文件为 /dev/null）
		if fd.NewName == "/dev/null" || fd.NewName == "" {
			if err := accessor.DeleteFile(fd.OldName); err != nil {
				pr.Results = append(pr.Results, FileResult{
					Path:  fd.OldName,
					Error: fmt.Sprintf("failed to delete file: %v", err),
				})
				pr.Failed++
			} else {
				pr.Results = append(pr.Results, FileResult{
					Path:    fd.OldName,
					Success: true,
				})
				pr.Succeeded++
			}
			continue
		}

		// 处理新文件创建（旧文件为 /dev/null）
		if fd.OldName == "/dev/null" || fd.OldName == "" {
			// 从 hunks 中提取所有插入行
			var newContent []string
			for _, h := range fd.Hunks {
				for _, dl := range h.Lines {
					if dl.Kind == OpInsert {
						newContent = append(newContent, dl.Text)
					}
				}
			}
			content := strings.Join(newContent, "\n") + "\n"
			if err := accessor.WriteFile(filePath, content); err != nil {
				pr.Results = append(pr.Results, FileResult{
					Path:  filePath,
					Error: fmt.Sprintf("failed to create file: %v", err),
				})
				pr.Failed++
			} else {
				pr.Results = append(pr.Results, FileResult{
					Path:    filePath,
					Success: true,
				})
				pr.Succeeded++
			}
			continue
		}

		// 读取现有文件
		content, err := accessor.ReadFile(filePath)
		if err != nil {
			pr.Results = append(pr.Results, FileResult{
				Path:  filePath,
				Error: fmt.Sprintf("failed to read file: %v", err),
			})
			pr.Failed++
			continue
		}

		// 应用 diff
		newContent, err := ApplyFileDiff(content, &fd)
		if err != nil {
			// 部分应用成功：写入部分修改后的内容，标记为成功+警告
			if pae, ok := err.(*PartialApplyError); ok {
				if writeErr := accessor.WriteFile(filePath, newContent); writeErr != nil {
					pr.Results = append(pr.Results, FileResult{
						Path:  filePath,
						Error: fmt.Sprintf("failed to write partial result: %v", writeErr),
					})
					pr.Failed++
					continue
				}
				failedHunks := make([]string, 0, len(pae.Errors))
				for _, ae := range pae.Errors {
					failedHunks = append(failedHunks, fmt.Sprintf("hunk #%d", ae.HunkIdx+1))
				}
				pr.Results = append(pr.Results, FileResult{
					Path:    filePath,
					Success: true,
					Warning: fmt.Sprintf("partially applied: %d/%d hunks succeeded, failed: %s",
						pae.Applied, pae.Total, strings.Join(failedHunks, ", ")),
				})
				pr.Succeeded++
				continue
			}

			pr.Results = append(pr.Results, FileResult{
				Path:  filePath,
				Error: fmt.Sprintf("failed to apply patch: %v", err),
			})
			pr.Failed++
			continue
		}

		// 写回文件
		if err := accessor.WriteFile(filePath, newContent); err != nil {
			pr.Results = append(pr.Results, FileResult{
				Path:  filePath,
				Error: fmt.Sprintf("failed to write file: %v", err),
			})
			pr.Failed++
			continue
		}

		pr.Results = append(pr.Results, FileResult{
			Path:    filePath,
			Success: true,
		})
		pr.Succeeded++
	}

	return pr
}

// splitLines 将文本按行分割
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	// 只移除末尾一个换行符（行终止符），保留空行
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}
