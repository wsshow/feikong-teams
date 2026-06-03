package tui

import (
	"os"
	"regexp"
	"strings"

	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

var ansiSeqRe = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\a]*(?:\a|\x1b\\))`)

func terminalWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	return 80
}

func renderedLineCount(s string, width int) int {
	if s == "" {
		return 0
	}
	if width < 20 {
		width = 80
	}

	total := 0
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if i == len(lines)-1 && line == "" {
			continue
		}
		visibleWidth := runewidth.StringWidth(stripANSI(line))
		if visibleWidth <= 0 {
			total++
			continue
		}
		total += (visibleWidth-1)/width + 1
	}
	return total
}

func stripANSI(s string) string {
	return ansiSeqRe.ReplaceAllString(s, "")
}

func runewidthStringWidth(s string) int {
	return runewidth.StringWidth(stripANSI(s))
}

func truncateRunes(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= maxWidth {
		return s
	}
	ellipsis := "..."
	limit := maxWidth - runewidth.StringWidth(ellipsis)
	if limit <= 0 {
		return ellipsis
	}
	var b strings.Builder
	width := 0
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if width+rw > limit {
			break
		}
		b.WriteRune(r)
		width += rw
	}
	return b.String() + ellipsis
}
