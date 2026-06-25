package tui

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
)

const InlineLineBreakTag = "[换行]"

var (
	InlinePasteTagRe       = regexp.MustCompile(`\[粘贴\d+行内容\]`)
	InlinePasteTagSuffixRe = regexp.MustCompile(`\s?\[粘贴\d+行内容\]\s?$`)
	InlineLineBreakTagRe   = regexp.MustCompile(regexp.QuoteMeta(InlineLineBreakTag))
	InlineMentionTokenRe   = regexp.MustCompile(`(^|[\s]|\x1b\[[0-9;]*m)(@[^\s\x1b]+)`)
	InlineFileTokenRe      = regexp.MustCompile(`(^|[\s]|\x1b\[[0-9;]*m)(#[^\s\x1b]+)`)
)

func InsertInlinePaste(value string, cursor int, pastes []string, content string) (string, int, []string) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	lines := strings.Split(content, "\n")
	placeholder := fmt.Sprintf("[粘贴%d行内容]", max(len(lines), 2))

	runes := []rune(value)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	before := runes[:cursor]
	after := runes[cursor:]

	pastesBefore := len(InlinePasteTagRe.FindAllString(string(before), -1))
	newPastes := make([]string, len(pastes)+1)
	copy(newPastes[:pastesBefore], pastes[:pastesBefore])
	newPastes[pastesBefore] = content
	copy(newPastes[pastesBefore+1:], pastes[pastesBefore:])

	padded := " " + placeholder + " "
	if cursor == 0 {
		padded = placeholder + " "
	}
	pRunes := []rune(padded)
	newRunes := make([]rune, 0, len(runes)+len(pRunes))
	newRunes = append(newRunes, before...)
	newRunes = append(newRunes, pRunes...)
	newRunes = append(newRunes, after...)
	return string(newRunes), cursor + len(pRunes), newPastes
}

func DeleteInlinePasteBeforeCursor(value string, cursor int, pastes []string) (string, int, []string, bool) {
	runes := []rune(value)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	before := string(runes[:cursor])
	loc := InlinePasteTagSuffixRe.FindStringIndex(before)
	if loc == nil {
		return value, cursor, pastes, false
	}
	pasteIdx := len(InlinePasteTagRe.FindAllString(before[:loc[0]], -1))
	after := string(runes[cursor:])
	value = before[:loc[0]] + after
	cursor = len([]rune(before[:loc[0]]))
	if pasteIdx < len(pastes) {
		pastes = append(pastes[:pasteIdx], pastes[pasteIdx+1:]...)
	}
	return value, cursor, pastes, true
}

func DeleteInlineTokenNearCursor(value string, cursor int) (string, int, bool) {
	runes := []rune(value)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	if cursor == 0 {
		return value, cursor, false
	}

	anchor := cursor
	for anchor > 0 && unicode.IsSpace(runes[anchor-1]) {
		anchor--
	}
	if anchor == 0 {
		return value, cursor, false
	}

	start := anchor - 1
	for start > 0 && !unicode.IsSpace(runes[start-1]) {
		start--
	}
	end := anchor
	for end < len(runes) && !unicode.IsSpace(runes[end]) {
		end++
	}

	token := string(runes[start:end])
	if len([]rune(token)) < 2 || (runes[start] != '@' && runes[start] != '#') {
		return value, cursor, false
	}

	deleteStart := start
	if deleteStart > 0 && unicode.IsSpace(runes[deleteStart-1]) {
		deleteStart--
	}
	deleteEnd := end
	if cursor > end {
		deleteEnd = cursor
	}

	newRunes := make([]rune, 0, len(runes)-(deleteEnd-deleteStart))
	newRunes = append(newRunes, runes[:deleteStart]...)
	newRunes = append(newRunes, runes[deleteEnd:]...)
	return string(newRunes), deleteStart, true
}

func ExpandInlineInput(text string, pastes []string) string {
	if len(pastes) > 0 {
		idx := 0
		text = InlinePasteTagRe.ReplaceAllStringFunc(text, func(match string) string {
			if idx >= len(pastes) {
				return match
			}
			content := pastes[idx]
			idx++
			return content
		})
	}
	return InlineLineBreakTagRe.ReplaceAllString(text, "\n")
}

func RenderInlineInputValue(view string) string {
	tagStyle := inlineTokenStyle("178")
	mentionStyle := inlineTokenStyle("6")
	fileStyle := inlineTokenStyle("10")
	view = InlinePasteTagRe.ReplaceAllStringFunc(view, func(match string) string {
		return tagStyle.Render(match)
	})
	view = InlineMentionTokenRe.ReplaceAllStringFunc(view, func(match string) string {
		return renderInlinePrefixedToken(match, "@", mentionStyle)
	})
	view = InlineFileTokenRe.ReplaceAllStringFunc(view, func(match string) string {
		return renderInlinePrefixedToken(match, "#", fileStyle)
	})
	return InlineLineBreakTagRe.ReplaceAllStringFunc(view, func(match string) string {
		return "\n  "
	})
}

func RenderInlineInputValueAtCursor(value string, cursor int) string {
	runes := []rune(value)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}

	before := RenderInlineInputValue(string(runes[:cursor]))
	if cursor >= len(runes) {
		return before + inputCursorStyle().Render(" ")
	}
	current := string(runes[cursor])
	after := RenderInlineInputValue(string(runes[cursor+1:]))
	return before + inputCursorStyle().Render(current) + after
}

func inlineTokenStyle(foreground string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(foreground)).Bold(true)
}

func inputCursorStyle() lipgloss.Style {
	return lipgloss.NewStyle().Reverse(true)
}

func renderInlinePrefixedToken(match string, prefix string, style lipgloss.Style) string {
	index := strings.Index(match, prefix)
	if index < 0 {
		return match
	}
	return match[:index] + style.Render(match[index:])
}
