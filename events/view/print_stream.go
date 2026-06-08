package eventview

import (
	fktui "fkteams/tui"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/mattn/go-isatty"
	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

// streamBuf 流式预览普通 Markdown，代码块内容折叠到状态行，最终正文一次性渲染。
type streamBuf struct {
	buf        strings.Builder
	agent      string
	path       string
	lastRender time.Time
	status     liveResponseStatus
}

type terminalToolFlow struct {
	Key       string
	AgentName string
	Name      string
	Status    string
	Args      string
	Result    string
	Streamed  bool
}

const renderInterval = 100 * time.Millisecond

type liveResponseStatus struct {
	enabled   bool
	checked   bool
	active    bool
	lastLines int
	lastView  string
}

func (s *streamBuf) reset() {
	s.buf.Reset()
	s.agent = ""
	s.path = ""
	s.lastRender = time.Time{}
}

func (s *streamBuf) addChunk(content string) {
	s.buf.WriteString(content)
	if s.lastRender.IsZero() || time.Since(s.lastRender) >= renderInterval {
		s.status.update(streamPreviewMarkdown(s.buf.String()))
		s.lastRender = time.Now()
	}
}

func (s *streamBuf) content() string {
	return s.buf.String()
}

func (s *streamBuf) discard() {
	s.status.finish()
	s.reset()
}

func (s *streamBuf) flush() {
	content := s.buf.String()
	if content == "" {
		s.reset()
		return
	}
	s.status.finish()
	rendered := fktui.RenderMarkdown(content)
	if rendered == "" {
		s.reset()
		return
	}
	lipgloss.Print(formatAssistantOutput(rendered))
	fmt.Print("\n")
	s.reset()
}

func formatAssistantOutput(rendered string) string {
	rendered = strings.TrimRight(rendered, "\n")
	if rendered == "" {
		return ""
	}
	lines := strings.Split(rendered, "\n")
	for i, line := range lines {
		if i == 0 {
			lines[i] = "\033[1;36m╰─▶\033[0m " + strings.TrimLeft(line, " \t")
		} else {
			lines[i] = line
		}
	}
	return strings.Join(lines, "\n")
}

func (s *liveResponseStatus) ensure() {
	if s.checked {
		return
	}
	s.checked = true
	s.enabled = isatty.IsTerminal(os.Stdout.Fd())
}

func (s *liveResponseStatus) update(content string) {
	s.ensure()
	if !s.enabled {
		return
	}
	view := fktui.RenderMarkdown(content)
	if view == "" {
		s.finish()
		return
	}
	view = normalizeLivePreview(view)
	if view == s.lastView {
		return
	}
	if s.lastLines > 0 {
		fmt.Printf("\033[%dF\033[J", s.lastLines)
	}
	fmt.Print(view)
	s.active = true
	s.lastLines = strings.Count(view, "\n")
	s.lastView = view
}

func (s *liveResponseStatus) finish() {
	if !s.active {
		return
	}
	if s.lastLines > 0 {
		fmt.Printf("\033[%dF\033[J", s.lastLines)
	}
	s.active = false
	s.lastLines = 0
	s.lastView = ""
}

func normalizeLivePreview(view string) string {
	width, height := terminalSize()
	if width < 20 {
		width = 80
	}
	if height < 8 {
		height = 24
	}

	wrapWidth := max(20, width-1)
	maxLines := max(4, min(18, height-6))

	var physical []string
	for _, line := range strings.Split(strings.TrimRight(view, "\n"), "\n") {
		wrapped := fktui.WrapStyledLine(line, wrapWidth)
		if len(wrapped) == 0 {
			physical = append(physical, "")
			continue
		}
		physical = append(physical, wrapped...)
	}
	if len(physical) > maxLines {
		hidden := len(physical) - maxLines + 1
		physical = append([]string{fmt.Sprintf("\033[90m... 省略预览 %d 行\033[0m", hidden)}, physical[len(physical)-maxLines+1:]...)
	}
	return strings.Join(physical, "\n") + "\n"
}

func terminalSize() (int, int) {
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 || h <= 0 {
		return fktui.TermWidth(), 24
	}
	return w, h
}

func responseStatusView(content string, width int) string {
	if width < 40 {
		width = 80
	}
	content = strings.TrimRight(content, "\n")
	runes := []rune(content)
	lineCount := 0
	if content != "" {
		lineCount = strings.Count(content, "\n") + 1
	}
	text := fmt.Sprintf("> **代码生成中** · %d 字 · %d 行", len(runes), lineCount)
	return truncateVisible(text, width-1) + "\n"
}

func streamPreviewMarkdown(content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	return foldCodeBlocksForStream(content)
}

func foldCodeBlocksForStream(content string) string {
	var out strings.Builder
	var code strings.Builder
	inCode := false
	fence := ""
	lines := strings.SplitAfter(content, "\n")

	for _, line := range lines {
		if inCode {
			code.WriteString(line)
			if isClosingFenceLine(line, fence) {
				out.WriteString(responseStatusView(codeProgressContent(code.String()), fktui.TermWidth()))
				code.Reset()
				inCode = false
				fence = ""
			}
			continue
		}
		if nextFence, ok := openingFence(line); ok {
			inCode = true
			fence = nextFence
			code.WriteString(line)
			continue
		}
		out.WriteString(line)
	}

	if inCode {
		out.WriteString(responseStatusView(codeProgressContent(code.String()), fktui.TermWidth()))
	}
	return out.String()
}

func codeProgressContent(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return ""
	}
	if _, ok := openingFence(lines[0]); ok {
		lines = lines[1:]
	}
	if len(lines) > 0 && isClosingFenceLine(lines[len(lines)-1], "```") {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > 0 && isClosingFenceLine(lines[len(lines)-1], "~~~") {
		lines = lines[:len(lines)-1]
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

func truncateVisible(s string, maxWidth int) string {
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

func sameStreamMessage(streamContent, messageContent string) bool {
	streamContent = strings.TrimSpace(streamContent)
	messageContent = strings.TrimSpace(messageContent)
	if streamContent == "" || messageContent == "" {
		return false
	}
	return streamContent == messageContent ||
		strings.Contains(messageContent, streamContent)
}

func openingFence(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(trimmed, "```"):
		return "```", true
	case strings.HasPrefix(trimmed, "~~~"):
		return "~~~", true
	default:
		return "", false
	}
}

func isClosingFenceLine(line, fence string) bool {
	if fence == "" {
		return false
	}
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, fence)
}

func unclosedFenceRange(content string) (start int, bodyStart int, inFence bool) {
	lineStart := 0
	for lineStart <= len(content) {
		lineEnd := strings.IndexByte(content[lineStart:], '\n')
		if lineEnd < 0 {
			lineEnd = len(content)
		} else {
			lineEnd += lineStart
		}

		line := strings.TrimSpace(content[lineStart:lineEnd])
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			if !inFence {
				start = lineStart
				bodyStart = lineEnd
				if bodyStart < len(content) && content[bodyStart] == '\n' {
					bodyStart++
				}
				inFence = true
			} else {
				inFence = false
			}
		}

		if lineEnd == len(content) {
			break
		}
		lineStart = lineEnd + 1
	}
	return start, bodyStart, inFence
}

func isMemberEvent(event Event) bool {
	return event.MemberCallID != ""
}

func agentDisplayName(name string) string {
	normalized := strings.TrimPrefix(strings.TrimSpace(name), agentToolPrefix)
	normalized = strings.ReplaceAll(normalized, "-", "_")
	return titleIdentifier(normalized)
}

func titleIdentifier(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-'
	})
	for i, part := range parts {
		if part == "" {
			continue
		}
		runes := []rune(strings.ToLower(part))
		runes[0] = unicode.ToUpper(runes[0])
		parts[i] = string(runes)
	}
	return strings.Join(parts, " ")
}

func agentKey(name string) string {
	normalized := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(name)), agentToolPrefix)
	normalized = strings.ReplaceAll(normalized, "-", "_")
	if normalized == "" {
		return "member"
	}
	return normalized
}

func agentToolKey(name string) (string, string, bool) {
	display := FormatToolDisplay(name)
	if display.Kind != "agent" {
		return "", "", false
	}
	target := display.Target
	if target == "" {
		target = strings.TrimPrefix(display.DisplayName, "指派给 ")
	}
	if target == "" {
		target = name
	}
	return agentKey(target), target, true
}

func isErrorContent(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(content, "执行出错") ||
		strings.Contains(lower, "error") ||
		strings.Contains(lower, "failed") ||
		strings.Contains(content, "失败")
}

// reasoningWriter 按终端宽度换行，每行补 │ 前缀
type reasoningWriter struct {
	col      int
	maxWidth int
}

const reasoningPrefix = "\033[1;36m│\033[0m  \033[3;90m"

func newReasoningWriter() *reasoningWriter {
	w := fktui.TermWidth() - 4
	if w < 20 {
		w = 20
	}
	return &reasoningWriter{maxWidth: w}
}

func (rw *reasoningWriter) writeChunk(content string) {
	for _, r := range content {
		if r == '\n' {
			fmt.Printf("\033[0m\n%s", reasoningPrefix)
			rw.col = 0
			continue
		}
		cw := runewidth.RuneWidth(r)
		if rw.col+cw > rw.maxWidth {
			fmt.Printf("\033[0m\n%s", reasoningPrefix)
			rw.col = 0
		}
		fmt.Printf("%c", r)
		rw.col += cw
	}
}
