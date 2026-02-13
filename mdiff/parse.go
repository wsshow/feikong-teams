package mdiff

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseMultiFileDiff 解析 unified diff 格式文本，返回多文件 diff 结构
func ParseMultiFileDiff(text string) (*MultiFileDiff, error) {
	if strings.TrimSpace(text) == "" {
		return &MultiFileDiff{}, nil
	}

	lines := strings.Split(text, "\n")
	// 移除末尾空行
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	var files []FileDiff
	i := 0

	for i < len(lines) {
		// 跳过空行
		if strings.TrimSpace(lines[i]) == "" {
			i++
			continue
		}

		// 查找 --- 行
		if !strings.HasPrefix(lines[i], "--- ") {
			i++
			continue
		}

		if i+1 >= len(lines) || !strings.HasPrefix(lines[i+1], "+++ ") {
			i++
			continue
		}

		fd, consumed, err := parseOneFileDiff(lines, i)
		if err != nil {
			return nil, fmt.Errorf("parse error at line %d: %w", i+1, err)
		}

		files = append(files, *fd)
		i += consumed
	}

	return &MultiFileDiff{Files: files}, nil
}

// ParseFileDiff 解析单个文件的 unified diff
func ParseFileDiff(text string) (*FileDiff, error) {
	mfd, err := ParseMultiFileDiff(text)
	if err != nil {
		return nil, err
	}
	if len(mfd.Files) == 0 {
		return &FileDiff{}, nil
	}
	return &mfd.Files[0], nil
}

// parseOneFileDiff 从 lines[start] 开始解析一个文件的 diff
// 返回解析结果和消耗的行数
func parseOneFileDiff(lines []string, start int) (*FileDiff, int, error) {
	if start+1 >= len(lines) {
		return nil, 0, fmt.Errorf("unexpected end of input")
	}

	// 解析 --- 行
	oldName := parseFileName(lines[start], "--- ")
	// 解析 +++ 行
	newName := parseFileName(lines[start+1], "+++ ")

	fd := &FileDiff{
		OldName: oldName,
		NewName: newName,
	}

	i := start + 2
	for i < len(lines) {
		// 如果遇到新的 --- 行，说明是下一个文件
		if strings.HasPrefix(lines[i], "--- ") && i+1 < len(lines) && strings.HasPrefix(lines[i+1], "+++ ") {
			break
		}

		// 解析 @@ 行
		if !strings.HasPrefix(lines[i], "@@") {
			i++
			continue
		}

		hunk, consumed, err := parseHunk(lines, i)
		if err != nil {
			return nil, 0, err
		}

		fd.Hunks = append(fd.Hunks, *hunk)
		i += consumed
	}

	return fd, i - start, nil
}

// parseFileName 从 "--- filename" 或 "+++ filename" 中提取文件名
func parseFileName(line, prefix string) string {
	name := strings.TrimPrefix(line, prefix)
	// 移除时间戳（如果有）
	if idx := strings.Index(name, "\t"); idx >= 0 {
		name = name[:idx]
	}
	// 移除 a/ b/ 前缀
	name = strings.TrimPrefix(name, "a/")
	name = strings.TrimPrefix(name, "b/")
	return name
}

// parseHunk 解析一个 hunk 块
func parseHunk(lines []string, start int) (*Hunk, int, error) {
	header := lines[start]

	oldStart, oldLines, newStart, newLines, err := parseHunkHeader(header)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid hunk header at line %d: %w", start+1, err)
	}

	hunk := &Hunk{
		OldStart: oldStart,
		OldLines: oldLines,
		NewStart: newStart,
		NewLines: newLines,
	}

	i := start + 1
	oldRemaining := oldLines
	newRemaining := newLines

	for i < len(lines) && (oldRemaining > 0 || newRemaining > 0) {
		line := lines[i]

		if len(line) == 0 {
			// 空行视为上下文行（仅在还有剩余空间时）
			if oldRemaining > 0 && newRemaining > 0 {
				hunk.Lines = append(hunk.Lines, DiffLine{Kind: OpEqual, Text: ""})
				oldRemaining--
				newRemaining--
				i++
				continue
			}
			// 没有剩余空间，hunk 结束
			break
		}

		prefix := line[0]
		content := line[1:]

		done := false
		switch prefix {
		case ' ':
			hunk.Lines = append(hunk.Lines, DiffLine{Kind: OpEqual, Text: content})
			oldRemaining--
			newRemaining--
		case '-':
			hunk.Lines = append(hunk.Lines, DiffLine{Kind: OpDelete, Text: content})
			oldRemaining--
		case '+':
			hunk.Lines = append(hunk.Lines, DiffLine{Kind: OpInsert, Text: content})
			newRemaining--
		case '\\':
			// "\ No newline at end of file" - 跳过此行
			i++
			continue
		default:
			// 遇到非预期字符，hunk 结束
			done = true
		}
		if done {
			break
		}

		i++
	}

	return hunk, i - start, nil
}

// parseHunkHeader 解析 "@@ -oldStart,oldLines +newStart,newLines @@" 格式
func parseHunkHeader(header string) (oldStart, oldLines, newStart, newLines int, err error) {
	// 移除 @@ 标记
	header = strings.TrimPrefix(header, "@@")
	// 找到第二个 @@
	idx := strings.Index(header, "@@")
	if idx >= 0 {
		header = header[:idx]
	}
	header = strings.TrimSpace(header)

	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 {
		return 0, 0, 0, 0, fmt.Errorf("invalid hunk header format: %q", header)
	}

	// 解析 -oldStart,oldLines
	oldPart := strings.TrimPrefix(parts[0], "-")
	oldStart, oldLines, err = parseRange(oldPart)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("invalid old range: %w", err)
	}

	// 解析 +newStart,newLines
	newPart := strings.TrimPrefix(parts[1], "+")
	newStart, newLines, err = parseRange(newPart)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("invalid new range: %w", err)
	}

	return oldStart, oldLines, newStart, newLines, nil
}

// parseRange 解析 "start,count" 或 "start" 格式
func parseRange(s string) (start, count int, err error) {
	parts := strings.SplitN(s, ",", 2)
	start, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid start: %w", err)
	}
	if len(parts) == 2 {
		count, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid count: %w", err)
		}
	} else {
		count = 1
	}
	return start, count, nil
}
