package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

var ansiSeqRe = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\a]*(?:\a|\x1b\\))`)

const toolArgsSummaryCacheLimit = 512

var toolArgsSummaryCache = struct {
	sync.Mutex
	values map[string]string
}{
	values: make(map[string]string),
}

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

func toolLabel(name, args string, maxWidth int) string {
	return toolLabelWithPending(name, args, false, maxWidth)
}

func toolLabelWithPending(name, args string, argsPending bool, maxWidth int) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "tool"
	}
	summary := ""
	if argsPending {
		summary = toolArgsPendingText
	} else {
		summary = toolArgsSummary(args)
	}
	if summary == "" {
		return truncateRunes(name, maxWidth)
	}
	label := fmt.Sprintf("%s(%s)", name, summary)
	return truncateRunes(label, maxWidth)
}

func toolArgsSummary(args string) string {
	args = strings.TrimSpace(args)
	if args == "" {
		return ""
	}
	toolArgsSummaryCache.Lock()
	if summary, ok := toolArgsSummaryCache.values[args]; ok {
		toolArgsSummaryCache.Unlock()
		return summary
	}
	toolArgsSummaryCache.Unlock()
	summary := computeToolArgsSummary(args)
	toolArgsSummaryCache.Lock()
	if len(toolArgsSummaryCache.values) >= toolArgsSummaryCacheLimit {
		toolArgsSummaryCache.values = make(map[string]string)
	}
	toolArgsSummaryCache.values[args] = summary
	toolArgsSummaryCache.Unlock()
	return summary
}

func computeToolArgsSummary(args string) string {
	var payload map[string]any
	if err := json.Unmarshal([]byte(args), &payload); err == nil {
		for _, key := range []string{"query", "url", "path", "file_path", "request", "task", "command"} {
			if v, ok := payload[key]; ok {
				if s := stringifyArgValue(v); s != "" {
					return s
				}
			}
		}
		keys := make([]string, 0, len(payload))
		for key := range payload {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if s := stringifyArgValue(payload[key]); s != "" {
				return s
			}
		}
	}
	return strings.Join(strings.Fields(args), " ")
}

func stableToolArgsSummary(args string) string {
	args = strings.TrimSpace(args)
	if args == "" {
		return ""
	}
	if strings.HasPrefix(args, "{") || strings.HasPrefix(args, "[") {
		var payload any
		if err := json.Unmarshal([]byte(args), &payload); err != nil {
			return ""
		}
	}
	return toolArgsSummary(args)
}

func stringifyArgValue(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64, bool:
		return fmt.Sprint(t)
	default:
		return ""
	}
}
