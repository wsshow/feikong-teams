package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"
)

func LineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func VisibleLines(s string, maxLines int, offsetFromBottom int) string {
	if s == "" || maxLines <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= maxLines {
		return s
	}
	if offsetFromBottom < 0 {
		offsetFromBottom = 0
	}
	maxOffset := max(0, len(lines)-maxLines)
	if offsetFromBottom > maxOffset {
		offsetFromBottom = maxOffset
	}
	end := len(lines) - offsetFromBottom
	start := end - maxLines
	if start < 0 {
		start = 0
	}
	return strings.Join(lines[start:end], "\n")
}

func WrapLines(content string, width int) string {
	if content == "" || width <= 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		lines[i] = lipgloss.Wrap(line, width, "")
	}
	return strings.Join(lines, "\n")
}

func TrimRenderedIndent(content string) string {
	if content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	commonIndent := 0
	hasContent := false
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := leadingSpaces(line)
		if !hasContent || indent < commonIndent {
			commonIndent = indent
		}
		hasContent = true
	}
	if commonIndent == 0 {
		return content
	}
	for i, line := range lines {
		if len(line) >= commonIndent {
			lines[i] = line[commonIndent:]
		}
	}
	return strings.Join(lines, "\n")
}

func StripANSI(s string) string {
	return stripANSI(s)
}

func CellWidth(s string) int {
	return runewidth.StringWidth(s)
}

func SliceCells(s string, start, end int) string {
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		w := runewidth.RuneWidth(r)
		if w < 1 {
			w = 1
		}
		next := col + w
		if next > start && col < end {
			b.WriteRune(r)
		}
		if col >= end {
			break
		}
		col = next
	}
	return b.String()
}

func CopiedNotice(text string) string {
	lines := 0
	chars := 0
	if text != "" {
		lines = len(strings.Split(text, "\n"))
		for _, line := range strings.Split(text, "\n") {
			chars += len([]rune(line))
		}
	}
	return fmt.Sprintf("已复制 %d 行 · %d 字符", lines, chars)
}

func leadingSpaces(s string) int {
	count := 0
	for _, r := range s {
		if r != ' ' {
			break
		}
		count++
	}
	return count
}
