package mdiff

import (
	"fmt"
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

// maxFuzz 模糊搜索的最大偏移行数
const maxFuzz = 100

// ApplyHunks 将一个文件的 hunks 应用到原始行上
// 支持模糊匹配：当精确行号不匹配时，在附近搜索上下文
func ApplyHunks(oldLines []string, hunks []Hunk) ([]string, error) {
	if len(hunks) == 0 {
		return oldLines, nil
	}

	// 复制原始行，避免修改输入
	result := make([]string, len(oldLines))
	copy(result, oldLines)

	// 从后往前应用 hunks，避免行号偏移问题
	for i := len(hunks) - 1; i >= 0; i-- {
		var err error
		result, err = applyOneHunk(result, &hunks[i], i)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

// applyOneHunk 应用一个 hunk 到行数组
func applyOneHunk(lines []string, hunk *Hunk, hunkIdx int) ([]string, error) {
	// 提取 hunk 中旧文件的行（Equal + Delete）
	oldHunkLines := make([]string, 0, hunk.OldLines)
	for _, dl := range hunk.Lines {
		if dl.Kind == OpEqual || dl.Kind == OpDelete {
			oldHunkLines = append(oldHunkLines, dl.Text)
		}
	}

	// 尝试精确位置匹配
	startIdx := hunk.OldStart - 1 // 转为0-based
	if startIdx < 0 {
		startIdx = 0
	}

	// 先精确匹配，再宽松匹配（忽略行尾空白，处理 LLM 生成的空白差异）
	matchPos := searchMatch(lines, oldHunkLines, startIdx, matchAt)
	if matchPos < 0 && len(oldHunkLines) > 0 {
		matchPos = searchMatch(lines, oldHunkLines, startIdx, matchAtLoose)
	}

	if matchPos < 0 {
		// 构建错误信息
		context := ""
		if len(oldHunkLines) > 0 {
			showLines := oldHunkLines
			if len(showLines) > 3 {
				showLines = showLines[:3]
			}
			context = fmt.Sprintf(", expected lines:\n%s", strings.Join(showLines, "\n"))
		}
		totalLines := len(lines)
		return nil, &ApplyError{
			HunkIdx: hunkIdx,
			Message: fmt.Sprintf("context mismatch at line %d (file has %d lines, searched +/-%d lines)%s",
				hunk.OldStart, totalLines, maxFuzz, context),
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

// searchMatch 在 startIdx 附近查找匹配位置，先尝试原位，再向前后扩展搜索
func searchMatch(lines, pattern []string, startIdx int, matcher func([]string, []string, int) bool) int {
	if matcher(lines, pattern, startIdx) {
		return startIdx
	}
	for offset := 1; offset <= maxFuzz; offset++ {
		if pos := startIdx - offset; pos >= 0 && matcher(lines, pattern, pos) {
			return pos
		}
		if pos := startIdx + offset; matcher(lines, pattern, pos) {
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

// ApplyFileDiff 将单个文件 diff 应用到文本内容
func ApplyFileDiff(content string, fd *FileDiff) (string, error) {
	if fd == nil || len(fd.Hunks) == 0 {
		return content, nil
	}

	lines := splitLines(content)
	newLines, err := ApplyHunks(lines, fd.Hunks)
	if err != nil {
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
