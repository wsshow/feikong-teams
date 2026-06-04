package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

type TextPoint struct {
	X int
	Y int
}

type TextSelection struct {
	Active bool
	Anchor TextPoint
	Cursor TextPoint
	Copied string
}

func NewTextSelection(point TextPoint) TextSelection {
	return TextSelection{Active: true, Anchor: point, Cursor: point}
}

func (s TextSelection) RenderLine(y int, line string) string {
	start, end := s.normalized()
	if y < start.Y || y > end.Y {
		return line
	}
	plain := StripANSI(line)
	startX, endX := lineSelectionRange(y, start, end, CellWidth(plain))
	if startX == endX {
		return line
	}
	before := SliceCells(plain, 0, startX)
	selected := SliceCells(plain, startX, endX)
	after := SliceCells(plain, endX, CellWidth(plain))
	return before + selectionStyle().Render(selected) + after
}

func (s TextSelection) SelectedText(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	start, end := s.normalized()
	if start.Y >= len(lines) {
		return ""
	}
	if end.Y >= len(lines) {
		end.Y = len(lines) - 1
	}
	var selected []string
	for y := start.Y; y <= end.Y; y++ {
		plain := StripANSI(lines[y])
		startX, endX := lineSelectionRange(y, start, end, CellWidth(plain))
		if startX == endX {
			selected = append(selected, "")
			continue
		}
		selected = append(selected, SliceCells(plain, startX, endX))
	}
	return strings.Join(selected, "\n")
}

func (s TextSelection) normalized() (TextPoint, TextPoint) {
	start := s.Anchor
	end := s.Cursor
	if end.Y < start.Y || (end.Y == start.Y && end.X < start.X) {
		start, end = end, start
	}
	if start.X < 0 {
		start.X = 0
	}
	if start.Y < 0 {
		start.Y = 0
	}
	if end.X < 0 {
		end.X = 0
	}
	if end.Y < 0 {
		end.Y = 0
	}
	return start, end
}

func lineSelectionRange(y int, start, end TextPoint, lineWidth int) (int, int) {
	startX := 0
	endX := lineWidth
	if y == start.Y {
		startX = min(start.X, lineWidth)
	}
	if y == end.Y {
		endX = min(end.X, lineWidth)
	}
	if startX > endX {
		startX, endX = endX, startX
	}
	return startX, endX
}

func selectionStyle() lipgloss.Style {
	return lipgloss.NewStyle().Reverse(true)
}
