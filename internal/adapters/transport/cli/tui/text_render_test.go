package tui

import (
	"strings"
	"testing"
)

func TestTextRenderHelpers(t *testing.T) {
	if LineCount("") != 0 || LineCount("a\nb") != 2 {
		t.Fatalf("LineCount returned unexpected values")
	}
	if got := VisibleLines("a\nb\nc\nd", 2, 1); got != "b\nc" {
		t.Fatalf("VisibleLines = %q, want b\\nc", got)
	}
	if got := VisibleLines("a\nb", 5, 0); got != "a\nb" {
		t.Fatalf("VisibleLines short content = %q", got)
	}
	if got := WrapLines("abcdef", 3); got != "abc\ndef" {
		t.Fatalf("WrapLines = %q", got)
	}
	if got := TrimRenderedIndent("    a\n      b\n"); got != "a\n  b\n" {
		t.Fatalf("TrimRenderedIndent = %q", got)
	}
	if got := StripANSI("\x1b[31mred\x1b[0m"); got != "red" {
		t.Fatalf("StripANSI = %q", got)
	}
	if CellWidth("你好") != 4 {
		t.Fatalf("CellWidth for wide chars = %d, want 4", CellWidth("你好"))
	}
	if got := SliceCells("你好abc", 2, 5); got != "好a" {
		t.Fatalf("SliceCells = %q, want 好a", got)
	}
	if got := CopiedNotice("a\n你好"); !strings.Contains(got, "2 行") || !strings.Contains(got, "3 字符") {
		t.Fatalf("CopiedNotice = %q", got)
	}
}

func TestWrapStyledLineKeepsStyleTokens(t *testing.T) {
	lines := WrapStyledLine("\x1b[31mabcdef\x1b[0m", 3)
	if len(lines) != 2 {
		t.Fatalf("wrapped line count = %d, want 2: %#v", len(lines), lines)
	}
	if StripANSI(strings.Join(lines, "")) != "abcdef" {
		t.Fatalf("wrapped visible text = %q", StripANSI(strings.Join(lines, "")))
	}
	if !strings.Contains(lines[0], "\x1b[31m") {
		t.Fatalf("expected first line to keep style token: %#v", lines)
	}
	if !isANSIResetToken("\x1b[0m") || !isANSIResetToken("\x1b[m") || isANSIResetToken("\x1b[31m") {
		t.Fatal("ANSI reset token detection returned unexpected values")
	}
}
