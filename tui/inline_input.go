package tui

import (
	"fmt"
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
)

const InlineLineBreakTag = "[换行]"

var (
	InlinePasteTagRe       = regexp.MustCompile(`\[粘贴\d+行内容\]`)
	InlinePasteTagSuffixRe = regexp.MustCompile(`\s?\[粘贴\d+行内容\]\s?$`)
	InlineLineBreakTagRe   = regexp.MustCompile(regexp.QuoteMeta(InlineLineBreakTag))
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
	tagStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("178")).Bold(true)
	view = InlinePasteTagRe.ReplaceAllStringFunc(view, func(match string) string {
		return tagStyle.Render(match)
	})
	return InlineLineBreakTagRe.ReplaceAllStringFunc(view, func(match string) string {
		return "\n  "
	})
}
