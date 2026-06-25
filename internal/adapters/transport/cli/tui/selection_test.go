package tui

import (
	"strings"
	"testing"
)

func TestTextSelectionSelectedTextAndRenderLine(t *testing.T) {
	selection := NewTextSelection(TextPoint{X: 4, Y: 2})
	selection.Cursor = TextPoint{X: 2, Y: 0}

	lines := []string{"abcdef", "\x1b[31mghijkl\x1b[0m", "mnopqr"}
	if got := selection.SelectedText(lines); got != "cdef\nghijkl\nmnop" {
		t.Fatalf("SelectedText = %q", got)
	}

	rendered := selection.RenderLine(1, lines[1])
	if StripANSI(rendered) != "ghijkl" {
		t.Fatalf("rendered visible line = %q", StripANSI(rendered))
	}
	if rendered == lines[1] {
		t.Fatal("expected selected line to be styled")
	}
	if got := selection.RenderLine(3, "outside"); got != "outside" {
		t.Fatalf("outside render = %q", got)
	}
}

func TestTextSelectionRanges(t *testing.T) {
	start, end := (TextSelection{
		Anchor: TextPoint{X: -3, Y: -1},
		Cursor: TextPoint{X: 8, Y: 2},
	}).normalized()
	if start != (TextPoint{X: 0, Y: 0}) || end != (TextPoint{X: 8, Y: 2}) {
		t.Fatalf("normalized = %#v %#v", start, end)
	}

	startX, endX := lineSelectionRange(0, TextPoint{X: 10, Y: 0}, TextPoint{X: 2, Y: 0}, 6)
	if startX != 2 || endX != 6 {
		t.Fatalf("same-line reversed range = %d,%d", startX, endX)
	}

	selection := TextSelection{Anchor: TextPoint{X: 10, Y: 10}, Cursor: TextPoint{X: 12, Y: 10}}
	if got := selection.SelectedText([]string{"a"}); got != "" {
		t.Fatalf("selection beyond lines = %q", got)
	}
	if got := selection.SelectedText(nil); got != "" {
		t.Fatalf("empty selection lines = %q", got)
	}
	if styled := selectionStyle().Render("x"); !strings.Contains(StripANSI(styled), "x") {
		t.Fatalf("selection style render = %q", styled)
	}
}
