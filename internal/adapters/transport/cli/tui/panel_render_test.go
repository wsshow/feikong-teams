package tui

import (
	"strings"
	"testing"
)

func TestPanelRenderHelpers(t *testing.T) {
	if renderedLineCount(strings.Repeat("a", 21), 20) != 2 {
		t.Fatalf("renderedLineCount returned unexpected value")
	}
	if stripANSI("\x1b[31mred\x1b[0m") != "red" {
		t.Fatalf("stripANSI failed")
	}
	if runewidthStringWidth("你好") != 4 {
		t.Fatalf("runewidthStringWidth returned unexpected value")
	}
	if truncateRunes("你好abc", 5) != "你..." {
		t.Fatalf("truncateRunes returned %q", truncateRunes("你好abc", 5))
	}
	if toolLabel("", `{"query":"search text"}`, 40) != "tool(search text)" {
		t.Fatalf("toolLabel returned unexpected value")
	}

	tests := []struct {
		args string
		want string
	}{
		{args: `{"query":" search "}`, want: "search"},
		{args: `{"url":"https://example.com"}`, want: "https://example.com"},
		{args: `{"flag":true}`, want: "true"},
		{args: `{"count":2}`, want: "2"},
		{args: `not   json`, want: "not json"},
	}
	for _, tt := range tests {
		if got := toolArgsSummary(tt.args); got != tt.want {
			t.Fatalf("toolArgsSummary(%q) = %q, want %q", tt.args, got, tt.want)
		}
	}
	if got := stringifyArgValue([]string{"x"}); got != "" {
		t.Fatalf("unsupported stringify value = %q", got)
	}
	if !strings.Contains(toolLabel("verylongtool", `{"path":"abcdef"}`, 8), "...") {
		t.Fatalf("expected long tool label to be truncated")
	}
}
