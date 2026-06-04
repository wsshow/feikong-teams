package tui

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"
)

var ansiTokenRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

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

func WrapStyledLine(content string, width int) []string {
	if content == "" {
		return []string{""}
	}
	if width <= 0 {
		return []string{content}
	}
	var lines []string
	var current strings.Builder
	var pendingStyle strings.Builder
	col := 0
	for len(content) > 0 {
		if loc := ansiTokenRe.FindStringIndex(content); loc != nil && loc[0] == 0 {
			token := content[:loc[1]]
			if isANSIResetToken(token) {
				current.WriteString(token)
			} else {
				pendingStyle.WriteString(token)
			}
			content = content[loc[1]:]
			continue
		}
		r, size := utf8.DecodeRuneInString(content)
		if r == utf8.RuneError && size == 0 {
			break
		}
		content = content[size:]
		if r == '\n' {
			lines = append(lines, current.String())
			current.Reset()
			pendingStyle.Reset()
			col = 0
			continue
		}
		w := runewidth.RuneWidth(r)
		if w < 1 {
			w = 1
		}
		if col > 0 && col+w > width {
			lines = append(lines, current.String())
			current.Reset()
			col = 0
		}
		current.WriteString(pendingStyle.String())
		pendingStyle.Reset()
		current.WriteRune(r)
		col += w
	}
	lines = append(lines, current.String())
	return lines
}

func isANSIResetToken(token string) bool {
	return token == "\x1b[0m" || token == "\x1b[m"
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
