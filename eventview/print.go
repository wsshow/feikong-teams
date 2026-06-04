package eventview

import (
	"encoding/json"
	"fkteams/agenttool"
	"fkteams/eventlog"
	"fkteams/fkevent"
	fktui "fkteams/tui"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/cloudwego/eino/schema"
	"github.com/mattn/go-isatty"
	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

const agentToolPrefix = agenttool.AgentToolPrefix

type Event = fkevent.Event

const (
	EventReasoningChunk     = fkevent.EventReasoningChunk
	EventStreamChunk        = fkevent.EventStreamChunk
	EventMessage            = fkevent.EventMessage
	EventToolResult         = fkevent.EventToolResult
	EventToolResultChunk    = fkevent.EventToolResultChunk
	EventToolCallsPreparing = fkevent.EventToolCallsPreparing
	EventToolCalls          = fkevent.EventToolCalls
	EventToolCallsArgsDelta = fkevent.EventToolCallsArgsDelta
	EventAction             = fkevent.EventAction
	EventUsage              = fkevent.EventUsage
	EventError              = fkevent.EventError

	ActionTransfer             = fkevent.ActionTransfer
	ActionContextCompressStart = fkevent.ActionContextCompressStart
	ActionContextCompress      = fkevent.ActionContextCompress
)

func isInternalToolName(name string) bool {
	return fkevent.IsInternalToolName(name)
}

func isInternalContinueContent(content string) bool {
	return fkevent.IsInternalContinueContent(content)
}

func FormatToolDisplay(name string) agenttool.ToolDisplay {
	return agenttool.FormatToolDisplay(name)
}

var (
	PrintEvent      func(Event)
	FlushPrintEvent func() // 刷新流式缓冲
)

func init() {
	PrintEvent, FlushPrintEvent = newPrintEvent()
}

// CLIEventCallback 创建 CLI 模式的事件回调，同时记录和打印。
func CLIEventCallback(recorder *eventlog.HistoryRecorder) func(Event) error {
	return func(event Event) error {
		recorder.RecordEvent(event)
		PrintEvent(event)
		return nil
	}
}

// JSONEventCallback 创建 JSON 格式的事件回调，将事件序列化为 JSON 输出。
func JSONEventCallback(recorder *eventlog.HistoryRecorder) func(Event) error {
	return func(event Event) error {
		recorder.RecordEvent(event)
		data, err := json.Marshal(event)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
}

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

var ansiSeqRe = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\a]*(?:\a|\x1b\\))`)

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
	// 预览不能触发终端自动换行，否则无法精确擦除。
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

func renderedScreenLines(s string, width int) int {
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
		visibleWidth := runewidth.StringWidth(ansiSeqRe.ReplaceAllString(line, ""))
		if visibleWidth <= 0 {
			total++
			continue
		}
		total += (visibleWidth-1)/width + 1
	}
	return total
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

func newPrintEvent() (func(Event), func()) {
	agentName := ""
	lastToolName := ""
	toolNamesByID := map[string]string{}
	toolFlows := map[string]*terminalToolFlow{}
	toolKeysByIndex := map[int]string{}
	toolPanel := fktui.NewToolPanel()
	toolPending := map[string]bool{}
	memberPanel := fktui.NewMemberPanel()
	activePanel := ""
	memberNamesByToolID := map[string]string{}
	memberKeysByToolID := map[string]string{}
	memberKeysByIndex := map[int]string{}
	memberPending := map[string]bool{}
	memberStarted := map[string]bool{}
	memberResultChunks := map[string]string{}
	var deferredEvents []Event
	replayingDeferred := false
	inReasoning := false
	var sb streamBuf
	var rw *reasoningWriter

	tryFlush := func() {
		if sb.buf.Len() > 0 {
			sb.flush()
		}
	}

	finishToolPanel := func() {
		toolPanel.Finish()
		if activePanel == "tool" {
			activePanel = ""
		}
	}

	finishMemberPanel := func() {
		memberPanel.Finish()
		if activePanel == "member" {
			activePanel = ""
		}
	}

	activateMemberPanel := func() {
		if activePanel == "tool" {
			finishToolPanel()
			toolPending = map[string]bool{}
			toolFlows = map[string]*terminalToolFlow{}
			toolKeysByIndex = map[int]string{}
		} else if activePanel == "" {
			fmt.Println()
		}
		activePanel = "member"
	}

	sendMemberPanel := func(e fktui.MemberEvent) bool {
		activateMemberPanel()
		return memberPanel.Send(e)
	}

	activateToolPanel := func() {
		if activePanel == "member" {
			finishMemberPanel()
		} else if activePanel == "" {
			fmt.Println()
		}
		activePanel = "tool"
	}

	ensureMember := func(key, name string) {
		if key == "" {
			return
		}
		if name == "" {
			name = agentDisplayName(key)
		}
		if !memberStarted[key] {
			memberStarted[key] = true
			memberPending[key] = true
			sendMemberPanel(fktui.MemberEvent{Key: key, Name: name, Type: "start"})
			return
		}
		sendMemberPanel(fktui.MemberEvent{Key: key, Name: name, Type: "meta"})
	}

	memberFromEvent := func(event Event) (string, string) {
		key := event.MemberCallID
		if key == "" {
			key = agentKey(event.AgentName)
		}
		name := event.MemberName
		if name == "" {
			name = agentDisplayName(event.AgentName)
		}
		ensureMember(key, name)
		return key, name
	}

	memberToolKey := func(event Event, tool schema.ToolCall, fallbackIndex int) string {
		if tool.ID != "" {
			return "id:" + tool.ID
		}
		if event.ToolCallID != "" {
			return "id:" + event.ToolCallID
		}
		if tool.Index != nil {
			return fmt.Sprintf("idx:%d", *tool.Index)
		}
		if event.ToolCallIndex != nil {
			return fmt.Sprintf("idx:%d", *event.ToolCallIndex)
		}
		if event.Detail != "" {
			return "idx:" + event.Detail
		}
		return fmt.Sprintf("fallback:%d", fallbackIndex)
	}

	regularToolKey := func(event Event, tool schema.ToolCall, fallbackIndex int) string {
		if tool.ID != "" {
			return "id:" + tool.ID
		}
		if event.ToolCallID != "" {
			return "id:" + event.ToolCallID
		}
		if tool.Index != nil {
			return fmt.Sprintf("idx:%d", *tool.Index)
		}
		if event.ToolCallIndex != nil {
			return fmt.Sprintf("idx:%d", *event.ToolCallIndex)
		}
		if event.Detail != "" {
			return "idx:" + event.Detail
		}
		return fmt.Sprintf("fallback:%d", fallbackIndex)
	}

	ensureToolFlow := func(key, name string) *terminalToolFlow {
		if key == "" {
			key = "last"
		}
		flow := toolFlows[key]
		if flow == nil {
			flow = &terminalToolFlow{Key: key, Name: name, Status: "参数准备中"}
			toolFlows[key] = flow
		}
		if name != "" {
			flow.Name = name
		}
		return flow
	}

	sendToolPanel := func(key string, flow *terminalToolFlow, eventType string, content string, appendContent bool) bool {
		if flow == nil {
			return false
		}
		name := flow.Name
		if name == "" || (name == key && strings.HasPrefix(key, "id:")) {
			return false
		}
		panelType := eventType
		switch eventType {
		case "content":
			panelType = "result"
		case "op":
			panelType = "args"
		}
		activateToolPanel()
		sent := toolPanel.Send(fktui.ToolEvent{
			Key:     key,
			Name:    name,
			Type:    panelType,
			Content: content,
			Append:  appendContent,
		})
		if sent {
			toolPending[key] = eventType != "done" && eventType != "error"
		}
		return sent
	}

	finishToolsIfIdle := func() {
		for _, pending := range toolPending {
			if pending {
				return
			}
		}
		finishToolPanel()
		toolPending = map[string]bool{}
		toolFlows = map[string]*terminalToolFlow{}
		toolKeysByIndex = map[int]string{}
	}

	printToolFlow := func(flow *terminalToolFlow) {
		if flow == nil {
			return
		}
		name := flow.Name
		if name == "" {
			name = lastToolName
		}
		display := FormatToolDisplay(name)
		title := display.DisplayName
		if title == "" {
			title = name
		}
		fmt.Printf("\n\033[1;35m[%s] 工具: \033[1m%s\033[0m \033[90m(%s)\033[0m\n", flow.AgentName, title, flow.Status)
		if flow.Args != "" {
			fmt.Printf("  参数: %s\n", truncateString(flow.Args, 240))
		}
		if flow.Result != "" {
			fmt.Printf("  结果:\n")
			formatted := formatToolResultForPrint(name, flow.Result)
			if formatted != "" {
				fmt.Print(formatted)
			} else {
				printPlainResult(flow.Result)
			}
		}
		fmt.Println()
	}

	hasPendingMembers := func() bool {
		if len(memberPending) == 0 {
			return false
		}
		for _, pending := range memberPending {
			if pending {
				return true
			}
		}
		return false
	}

	var printFn func(Event)

	resetMemberState := func() {
		memberPending = map[string]bool{}
		memberStarted = map[string]bool{}
		memberNamesByToolID = map[string]string{}
		memberKeysByToolID = map[string]string{}
		memberKeysByIndex = map[int]string{}
		memberResultChunks = map[string]string{}
	}

	finalizeChunkedMemberResults := func() {
		for callID, content := range memberResultChunks {
			key := memberKeysByToolID[callID]
			if key == "" || !memberPending[key] {
				delete(memberResultChunks, callID)
				continue
			}
			memberName := memberNamesByToolID[callID]
			if isErrorContent(content) {
				sendMemberPanel(fktui.MemberEvent{Key: key, Name: memberName, Type: "error", Content: content, ToolKey: "id:" + callID, ToolName: memberName})
			} else {
				sendMemberPanel(fktui.MemberEvent{Key: key, Name: memberName, Type: "done"})
			}
			memberPending[key] = false
			delete(memberResultChunks, callID)
		}
	}

	finishMembersIfIdle := func() {
		finalizeChunkedMemberResults()
		if hasPendingMembers() {
			return
		}
		finishMemberPanel()
		resetMemberState()
		if len(deferredEvents) > 0 && !replayingDeferred {
			events := deferredEvents
			deferredEvents = nil
			replayingDeferred = true
			for _, deferred := range events {
				printFn(deferred)
			}
			replayingDeferred = false
		}
	}

	flushDeferred := func() {
		finalizeChunkedMemberResults()
		finishMemberPanel()
		resetMemberState()
		if len(deferredEvents) > 0 && !replayingDeferred {
			events := deferredEvents
			deferredEvents = nil
			replayingDeferred = true
			for _, deferred := range events {
				printFn(deferred)
			}
			replayingDeferred = false
		}
	}

	finishMembersBeforeParentOutput := func(event Event) bool {
		if len(memberPending) == 0 || isMemberEvent(event) {
			return false
		}
		finalizeChunkedMemberResults()
		if hasPendingMembers() && !replayingDeferred {
			deferredEvents = append(deferredEvents, event)
			return true
		}
		finishMemberPanel()
		resetMemberState()
		return false
	}

	migrateMemberKey := func(oldKey, newKey, name string) {
		if oldKey == "" || newKey == "" || oldKey == newKey {
			return
		}
		sendMemberPanel(fktui.MemberEvent{Key: oldKey, NewKey: newKey, Name: name, Type: "rename"})
		if pending, ok := memberPending[oldKey]; ok {
			delete(memberPending, oldKey)
			memberPending[newKey] = pending
		}
		if memberStarted[oldKey] {
			delete(memberStarted, oldKey)
			memberStarted[newKey] = true
		}
	}

	registerAgentToolCall := func(tool schema.ToolCall, fallbackIndex int) (string, string) {
		key, memberName, _ := agentToolKey(tool.Function.Name)
		if tool.ID != "" {
			key = tool.ID
			if tool.Index != nil && memberKeysByIndex[*tool.Index] != "" {
				migrateMemberKey(memberKeysByIndex[*tool.Index], key, memberName)
			}
		} else if tool.Index != nil {
			key = fmt.Sprintf("pending:%d", *tool.Index)
		} else if key == "" {
			key = fmt.Sprintf("pending:%d", fallbackIndex)
		}
		if tool.ID != "" {
			memberNamesByToolID[tool.ID] = memberName
			memberKeysByToolID[tool.ID] = key
		}
		if tool.Index != nil {
			memberKeysByIndex[*tool.Index] = key
		}
		if tool.ID != "" {
			toolNamesByID[tool.ID] = tool.Function.Name
			delete(toolFlows, "id:"+tool.ID)
			delete(toolPending, "id:"+tool.ID)
		}
		ensureMember(key, memberName)
		return key, memberName
	}

	splitAgentToolCalls := func(toolCalls []schema.ToolCall) (agents, others []schema.ToolCall) {
		for _, tool := range toolCalls {
			if isInternalToolName(tool.Function.Name) {
				continue
			}
			display := FormatToolDisplay(tool.Function.Name)
			if display.Kind == "agent" {
				agents = append(agents, tool)
			} else {
				others = append(others, tool)
			}
		}
		return agents, others
	}

	printFn = func(event Event) {
		switch event.Type {
		case EventReasoningChunk:
			if isMemberEvent(event) {
				key, name := memberFromEvent(event)
				sendMemberPanel(fktui.MemberEvent{Key: key, Name: name, Type: "content", Content: event.Content})
				return
			}
			if finishMembersBeforeParentOutput(event) {
				return
			}
			tryFlush()
			if agentName != event.AgentName {
				agentName = event.AgentName
				fmt.Printf("\n\033[1;36m╭─ [%s] %s\033[0m\n", agentName, event.RunPath)
			}
			if !inReasoning {
				inReasoning = true
				rw = newReasoningWriter()
				fmt.Printf("%s\033[90m[思考] \033[0m%s", reasoningPrefix, "\033[3;90m")
				rw.col = 6 // "[思考] " 占 6 列
			}
			rw.writeChunk(event.Content)

		case EventStreamChunk:
			if isMemberEvent(event) {
				key, name := memberFromEvent(event)
				sendMemberPanel(fktui.MemberEvent{Key: key, Name: name, Type: "content", Content: event.Content})
				return
			}
			if finishMembersBeforeParentOutput(event) {
				return
			}
			wasReasoning := inReasoning
			if inReasoning {
				inReasoning = false
				fmt.Printf("\033[0m\n")
			}
			if agentName != event.AgentName {
				tryFlush()
				agentName = event.AgentName
				fmt.Printf("\n\033[1;36m╭─ [%s] %s\033[0m\n", agentName, event.RunPath)
				sb.agent = agentName
				sb.path = event.RunPath
			} else if wasReasoning && sb.agent == "" {
				sb.agent = agentName
				sb.path = event.RunPath
			}
			if sb.agent == "" {
				sb.agent = event.AgentName
				sb.path = event.RunPath
			}
			sb.addChunk(event.Content)

		case EventMessage:
			if isMemberEvent(event) {
				key, name := memberFromEvent(event)
				if event.ReasoningContent != "" {
					sendMemberPanel(fktui.MemberEvent{Key: key, Name: name, Type: "content", Content: event.ReasoningContent})
				}
				if event.Content != "" {
					sendMemberPanel(fktui.MemberEvent{Key: key, Name: name, Type: "content", Content: event.Content})
				}
				return
			}
			if finishMembersBeforeParentOutput(event) {
				return
			}
			printContent := event.Content
			if event.Content != "" && sameStreamMessage(sb.content(), event.Content) {
				sb.discard()
				printContent = ""
			} else {
				tryFlush()
			}
			if inReasoning {
				inReasoning = false
				fmt.Printf("\033[0m\n")
			}
			if event.ReasoningContent != "" {
				fmt.Printf("\n\033[90m[%s] 思考:\033[0m \033[3;90m%s\033[0m\n", event.AgentName, event.ReasoningContent)
			}
			if printContent != "" {
				fmt.Printf("\n\033[1;32m✓ [%s]\033[0m\n", event.AgentName)
				lipgloss.Println(formatAssistantOutput(fktui.RenderMarkdown(printContent)))
			}

		case EventToolResult:
			if isMemberEvent(event) {
				key, name := memberFromEvent(event)
				if event.Content != "" {
					toolKey := memberToolKey(event, schema.ToolCall{}, 0)
					sendMemberPanel(fktui.MemberEvent{Key: key, Name: name, Type: "tool_result", ToolKey: toolKey, ToolName: event.ToolName, Content: event.Content})
				}
				return
			}
			toolName := lastToolName
			if event.ToolName != "" {
				toolName = event.ToolName
			}
			if event.ToolCallID != "" {
				if name, ok := toolNamesByID[event.ToolCallID]; ok {
					toolName = name
				}
			}
			if isInternalToolName(toolName) || isInternalContinueContent(event.Content) {
				return
			}
			display := FormatToolDisplay(toolName)
			if display.Kind == "agent" {
				key, memberName, ok := agentToolKey(toolName)
				if !ok {
					key = agentKey(display.Target)
					memberName = display.Target
				}
				if event.ToolCallID != "" && memberNamesByToolID[event.ToolCallID] != "" {
					memberName = memberNamesByToolID[event.ToolCallID]
				}
				if event.ToolCallID != "" && memberKeysByToolID[event.ToolCallID] != "" {
					key = memberKeysByToolID[event.ToolCallID]
				}
				if isErrorContent(event.Content) {
					sendMemberPanel(fktui.MemberEvent{Key: key, Name: memberName, Type: "error", Content: event.Content, ToolKey: memberToolKey(event, schema.ToolCall{}, 0), ToolName: toolName})
				} else {
					sendMemberPanel(fktui.MemberEvent{Key: key, Name: memberName, Type: "done"})
				}
				memberPending[key] = false
				finishMembersIfIdle()
				return
			}
			if finishMembersBeforeParentOutput(event) {
				return
			}
			tryFlush()
			key := regularToolKey(event, schema.ToolCall{}, 0)
			flow := ensureToolFlow(key, toolName)
			flow.AgentName = event.AgentName
			flow.Status = "已完成"
			if event.Content != "" {
				if flow.Result != "" && !strings.Contains(flow.Result, event.Content) {
					flow.Result += event.Content
				} else if flow.Result == "" {
					flow.Result = event.Content
				}
			}
			doneContent := flow.Result
			if flow.Streamed {
				doneContent = ""
			}
			sent := sendToolPanel(key, flow, "done", doneContent, false)
			finishToolsIfIdle()
			if !sent {
				printToolFlow(flow)
			}
			delete(toolFlows, key)

		case EventToolResultChunk:
			if isMemberEvent(event) {
				key, name := memberFromEvent(event)
				toolKey := memberToolKey(event, schema.ToolCall{}, 0)
				sendMemberPanel(fktui.MemberEvent{Key: key, Name: name, Type: "tool_result", ToolKey: toolKey, ToolName: event.ToolName, Content: event.Content, Append: true})
				return
			}
			if event.ToolCallID != "" && memberKeysByToolID[event.ToolCallID] != "" {
				memberResultChunks[event.ToolCallID] += event.Content
				return
			}
			if finishMembersBeforeParentOutput(event) {
				return
			}
			key := regularToolKey(event, schema.ToolCall{}, 0)
			flow := ensureToolFlow(key, event.ToolName)
			flow.AgentName = event.AgentName
			flow.Status = "执行中"
			flow.Result += event.Content
			flow.Streamed = true
			sendToolPanel(key, flow, "content", event.Content, true)

		case EventToolCallsPreparing:
			if isMemberEvent(event) {
				key, name := memberFromEvent(event)
				for i, tool := range event.ToolCalls {
					if tool.Function.Name != "" {
						display := FormatToolDisplay(tool.Function.Name)
						sendMemberPanel(fktui.MemberEvent{Key: key, Name: name, Type: "tool_prepare", ToolKey: memberToolKey(event, tool, i), ToolName: display.DisplayName})
					}
				}
				return
			}
			agentTools, otherTools := splitAgentToolCalls(event.ToolCalls)
			if len(agentTools) > 0 {
				if inReasoning {
					inReasoning = false
					fmt.Printf("\033[0m\n")
				}
				tryFlush()
			}
			for i, tool := range agentTools {
				key, memberName := registerAgentToolCall(tool, i)
				sendMemberPanel(fktui.MemberEvent{Key: key, Name: memberName, Type: "op", Content: "任务准备中"})
			}
			if len(agentTools) > 0 && len(otherTools) == 0 {
				return
			}
			if len(agentTools) > 0 {
				event.ToolCalls = otherTools
				if hasPendingMembers() && !replayingDeferred {
					deferredEvents = append(deferredEvents, event)
					return
				}
			}
			if finishMembersBeforeParentOutput(event) {
				return
			}
			tryFlush()
			if inReasoning {
				inReasoning = false
				fmt.Printf("\033[0m\n")
			}
			for i, tool := range event.ToolCalls {
				if isInternalToolName(tool.Function.Name) {
					continue
				}
				if tool.Function.Name != "" {
					if tool.ID != "" {
						toolNamesByID[tool.ID] = tool.Function.Name
					}
					if tool.Index != nil {
						toolKeysByIndex[*tool.Index] = regularToolKey(event, tool, i)
					}
					key := regularToolKey(event, tool, i)
					flow := ensureToolFlow(key, tool.Function.Name)
					flow.AgentName = event.AgentName
					flow.Status = "参数准备中"
					flow.Name = tool.Function.Name
					lastToolName = tool.Function.Name
					sendToolPanel(key, flow, "start", "", false)
				}
			}

		case EventToolCalls:
			if isMemberEvent(event) {
				key, name := memberFromEvent(event)
				for i, tool := range event.ToolCalls {
					if tool.Function.Name == "" {
						continue
					}
					display := FormatToolDisplay(tool.Function.Name)
					sendMemberPanel(fktui.MemberEvent{Key: key, Name: name, Type: "tool_args", ToolKey: memberToolKey(event, tool, i), ToolName: display.DisplayName, Content: tool.Function.Arguments})
				}
				return
			}
			agentTools, otherTools := splitAgentToolCalls(event.ToolCalls)
			if len(agentTools) > 0 {
				if inReasoning {
					inReasoning = false
					fmt.Printf("\033[0m\n")
				}
				tryFlush()
			}
			for i, tool := range agentTools {
				key, memberName := registerAgentToolCall(tool, i)
				op := "任务已分配"
				if tool.Function.Arguments != "" {
					op = "任务: " + tool.Function.Arguments
				}
				sendMemberPanel(fktui.MemberEvent{Key: key, Name: memberName, Type: "op", Content: op})
			}
			if len(agentTools) > 0 && len(otherTools) == 0 {
				return
			}
			if len(agentTools) > 0 {
				event.ToolCalls = otherTools
				if hasPendingMembers() && !replayingDeferred {
					deferredEvents = append(deferredEvents, event)
					return
				}
			}
			if finishMembersBeforeParentOutput(event) {
				return
			}
			tryFlush()
			for i, tool := range event.ToolCalls {
				if isInternalToolName(tool.Function.Name) {
					continue
				}
				if tool.ID != "" {
					toolNamesByID[tool.ID] = tool.Function.Name
				}
				key := regularToolKey(event, tool, i)
				if tool.ID != "" && tool.Index != nil {
					oldKey := fmt.Sprintf("idx:%d", *tool.Index)
					if existing := toolFlows[oldKey]; existing != nil && toolFlows[key] == nil {
						toolFlows[key] = existing
						delete(toolFlows, oldKey)
					}
				}
				flow := ensureToolFlow(key, tool.Function.Name)
				flow.AgentName = event.AgentName
				flow.Status = "已调用"
				if tool.Function.Arguments != "" {
					flow.Args = tool.Function.Arguments
					sendToolPanel(key, flow, "op", tool.Function.Arguments, false)
				}
				if i == len(event.ToolCalls)-1 {
					lastToolName = tool.Function.Name
				}
			}

		case EventAction:
			if finishMembersBeforeParentOutput(event) {
				return
			}
			tryFlush()
			switch event.ActionType {
			case ActionContextCompressStart:
				fmt.Printf("\n\033[1;33m~ [%s] %s\033[0m", event.AgentName, event.Content)
			case ActionContextCompress:
				fmt.Printf("\n\033[1;33m✓ [%s] %s\033[0m\n", event.AgentName, event.Content)
			default:
				fmt.Printf("\n\033[1;34m▸ [%s] 动作: %s\033[0m\n", event.AgentName, event.ActionType)
				if event.Content != "" {
					fmt.Printf("  详情: %s\n", event.Content)
				}
			}

		case EventUsage:
			return

		case EventError:
			if isMemberEvent(event) {
				key, name := memberFromEvent(event)
				sendMemberPanel(fktui.MemberEvent{Key: key, Name: name, Type: "error", Content: event.Error})
				memberPending[key] = false
				finishMembersIfIdle()
				return
			}
			if finishMembersBeforeParentOutput(event) {
				return
			}
			tryFlush()
			fmt.Printf("\n\033[1;31m✗ [%s] 错误:\033[0m\n", event.AgentName)
			fmt.Printf("  \033[31m%s\033[0m\n", event.Error)
			if event.RunPath != "" {
				fmt.Printf("  路径: %s\n", event.RunPath)
			}
			fmt.Println()

		case EventToolCallsArgsDelta:
			if isMemberEvent(event) {
				key, name := memberFromEvent(event)
				sendMemberPanel(fktui.MemberEvent{Key: key, Name: name, Type: "tool_args", ToolKey: memberToolKey(event, schema.ToolCall{}, 0), ToolName: event.ToolName, Content: event.Content, Append: true})
				return
			}
			toolName := event.ToolName
			if toolName == "" && event.ToolCallID != "" {
				toolName = toolNamesByID[event.ToolCallID]
			}
			if toolName != "" {
				if key, memberName, ok := agentToolKey(toolName); ok {
					if event.ToolCallID != "" {
						if mapped := memberKeysByToolID[event.ToolCallID]; mapped != "" {
							key = mapped
						} else {
							memberKeysByToolID[event.ToolCallID] = key
						}
						if memberNamesByToolID[event.ToolCallID] == "" {
							memberNamesByToolID[event.ToolCallID] = memberName
						}
					}
					sendMemberPanel(fktui.MemberEvent{Key: key, Name: memberName, Type: "op", Content: "任务参数接收中"})
					return
				}
			}
			key := regularToolKey(event, schema.ToolCall{}, 0)
			if event.ToolCallIndex != nil {
				if mapped := toolKeysByIndex[*event.ToolCallIndex]; mapped != "" && event.ToolCallID == "" {
					key = mapped
				}
			}
			flow := ensureToolFlow(key, toolName)
			flow.AgentName = event.AgentName
			flow.Status = "参数准备中"
			flow.Args += event.Content
			if toolName == "" {
				return
			}
			sendToolPanel(key, flow, "op", flow.Args, false)
		default:
			if finishMembersBeforeParentOutput(event) {
				return
			}
			fmt.Printf("\n\033[1;90m? 未知事件: %s\033[0m\n", event.Type)
			if event.AgentName != "" {
				fmt.Printf("  代理: %s\n", event.AgentName)
			}
			if event.Content != "" {
				fmt.Printf("  内容: %s\n", event.Content)
			}
		}
	}

	return printFn, func() {
		tryFlush()
		flushDeferred()
	}
}

func printPlainResult(content string) {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if i >= 30 {
			fmt.Printf("  │ ... 还有 %d 行\n", len(lines)-30)
			break
		}
		if line != "" {
			fmt.Printf("  │ %s\n", truncateString(line, 200))
		}
	}
}

func formatToolResultForPrint(toolName, content string) string {
	switch toolName {
	case "search":
		return formatSearchResults(content)
	case "execute":
		return formatCommandResult(content)
	case "file_read", "file_write", "file_edit", "file_list", "grep":
		return formatFileOpResult(content)
	case "file_patch":
		return formatFilePatchResult(content)
	case "ssh_execute", "ssh_upload", "ssh_download", "ssh_list_dir":
		return formatSSHResult(content, toolName)
	case "todo_add", "todo_list", "todo_update", "todo_delete", "todo_batch_add", "todo_batch_delete", "todo_clear":
		return formatTodoResult(content, toolName)
	case "schedule_add", "schedule_list", "schedule_cancel", "schedule_delete":
		return formatSchedulerResult(content, toolName)
	case "dispatch_tasks":
		return formatDispatchResult(content)
	default:
		return ""
	}
}

func formatSearchResults(content string) string {
	var result struct {
		Message string `json:"message"`
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Summary string `json:"summary"`
		} `json:"results"`
	}

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return ""
	}

	var output strings.Builder

	if result.Message != "" {
		fmt.Fprintf(&output, "  \033[32m✓ %s\033[0m\n\n", result.Message)
	}

	for i, r := range result.Results {
		if i >= 5 {
			fmt.Fprintf(&output, "  \033[90m... 还有 %d 条结果\033[0m\n", len(result.Results)-5)
			break
		}
		fmt.Fprintf(&output, "  \033[1;36m%d. %s\033[0m\n", i+1, r.Title)

		if r.URL != "" {
			fmt.Fprintf(&output, "     \033[90mURL: %s\033[0m\n", truncateString(r.URL, 80))
		}

		if r.Summary != "" {
			summary := strings.ReplaceAll(r.Summary, "\n", " ")
			summary = truncateString(summary, 120)
			fmt.Fprintf(&output, "     %s\n", summary)
		}

		if i < min(len(result.Results)-1, 4) {
			output.WriteString("\n")
		}
	}

	return output.String()
}

func formatCommandResult(content string) string {
	var result struct {
		Stdout         string `json:"stdout"`
		Stderr         string `json:"stderr"`
		ExitCode       int    `json:"exit_code"`
		ExecutionTime  string `json:"execution_time"`
		SecurityLevel  string `json:"security_level"`
		WarningMessage string `json:"warning_message"`
		ErrorMessage   string `json:"error_message"`
	}

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return ""
	}

	var output strings.Builder

	if result.ErrorMessage != "" {
		fmt.Fprintf(&output, "  \033[31m✗ %s\033[0m\n", result.ErrorMessage)
		return output.String()
	}

	if result.ExitCode == 0 {
		fmt.Fprintf(&output, "  \033[32m✓ 执行成功\033[0m (退出码: 0, 耗时: %s)\n", result.ExecutionTime)
	} else {
		fmt.Fprintf(&output, "  \033[31m✗ 执行失败\033[0m (退出码: %d, 耗时: %s)\n", result.ExitCode, result.ExecutionTime)
	}

	if result.WarningMessage != "" {
		fmt.Fprintf(&output, "  \033[33m⚠ %s\033[0m\n", result.WarningMessage)
	}

	if result.Stdout != "" {
		output.WriteString("\n")
		lines := strings.Split(result.Stdout, "\n")
		for i, line := range lines {
			if i >= 30 {
				fmt.Fprintf(&output, "  │ ... 还有 %d 行\n", len(lines)-30)
				break
			}
			if line != "" {
				fmt.Fprintf(&output, "  │ %s\n", truncateString(line, 200))
			}
		}
	}

	if result.Stderr != "" {
		output.WriteString("\n  \033[31m标准错误:\033[0m\n")
		lines := strings.Split(result.Stderr, "\n")
		for i, line := range lines {
			if i >= 20 {
				fmt.Fprintf(&output, "  │ ... 还有 %d 行\n", len(lines)-20)
				break
			}
			if line != "" {
				fmt.Fprintf(&output, "  │ %s\n", truncateString(line, 200))
			}
		}
	}

	return output.String()
}

func formatFileOpResult(content string) string {
	var result struct {
		Success      bool     `json:"success"`
		Message      string   `json:"message"`
		FilePath     string   `json:"file_path"`
		Content      string   `json:"content"`
		Files        []string `json:"files"`
		Size         int64    `json:"size"`
		ErrorMessage string   `json:"error_message"`
	}

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return ""
	}

	result.Success = result.ErrorMessage == ""

	var output strings.Builder

	if result.Success {
		output.WriteString("  \033[32m✓ 操作成功\033[0m\n")
	} else {
		output.WriteString("  \033[31m✗ 操作失败\033[0m\n")
	}

	if result.Message != "" {
		fmt.Fprintf(&output, "  %s\n", result.Message)
	}

	if result.FilePath != "" {
		fmt.Fprintf(&output, "  \033[90m路径: %s\033[0m\n", result.FilePath)
	}

	if result.Size > 0 {
		fmt.Fprintf(&output, "  大小: %s\n", formatFileSize(result.Size))
	}

	if len(result.Files) > 0 {
		output.WriteString("\n  \033[1m文件列表:\033[0m\n")
		for i, file := range result.Files {
			if i < 20 { // 限制显示数量
				fmt.Fprintf(&output, "  │ %s\n", file)
			} else if i == 20 {
				fmt.Fprintf(&output, "  │ ... 还有 %d 个文件\n", len(result.Files)-20)
				break
			}
		}
	}

	if result.Content != "" {
		output.WriteString("\n  \033[1m内容:\033[0m\n")
		lines := strings.Split(result.Content, "\n")
		for i, line := range lines {
			if i < 30 {
				fmt.Fprintf(&output, "  %3d │ %s\n", i+1, line)
			} else if i == 30 {
				fmt.Fprintf(&output, "  ... 还有 %d 行\n", len(lines)-30)
				break
			}
		}
	}

	if result.ErrorMessage != "" {
		fmt.Fprintf(&output, "  \033[31m错误: %s\033[0m\n", result.ErrorMessage)
	}

	return output.String()
}

func formatSSHResult(content string, toolName string) string {
	var result map[string]any
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return ""
	}

	var output strings.Builder

	if execTime, ok := result["execution_time"].(string); ok && execTime != "" {
		fmt.Fprintf(&output, "  执行时间: %s\n", execTime)
	}

	if errMsg, ok := result["error_message"].(string); ok && errMsg != "" {
		fmt.Fprintf(&output, "  \033[31m✗ %s\033[0m\n", errMsg)
		return output.String()
	}

	output.WriteString("  \033[32m✓ 操作成功\033[0m\n\n")

	switch toolName {
	case "ssh_execute":
		if out, ok := result["output"].(string); ok && out != "" {
			output.WriteString("  \033[1m输出:\033[0m\n")
			lines := strings.Split(out, "\n")
			for i, line := range lines {
				if i >= 30 {
					fmt.Fprintf(&output, "  │ ... 还有 %d 行\n", len(lines)-30)
					break
				}
				if line != "" {
					fmt.Fprintf(&output, "  │ %s\n", truncateString(line, 200))
				}
			}
		}

	case "ssh_upload", "ssh_download":
		if msg, ok := result["message"].(string); ok {
			fmt.Fprintf(&output, "  %s\n", msg)
		}
		if size, ok := result["bytes_transferred"].(float64); ok {
			fmt.Fprintf(&output, "  传输大小: %s\n", formatFileSize(int64(size)))
		}

	case "ssh_list_dir":
		if files, ok := result["files"].([]any); ok && len(files) > 0 {
			output.WriteString("  \033[1m文件列表:\033[0m\n")
			for i, f := range files {
				if i < 20 {
					fmt.Fprintf(&output, "  │ %v\n", f)
				} else if i == 20 {
					fmt.Fprintf(&output, "  │ ... 还有 %d 个文件\n", len(files)-20)
					break
				}
			}
		}
	}

	return output.String()
}

func formatFileSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

func formatTodoResult(content string, toolName string) string {
	var result map[string]any
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return ""
	}

	var output strings.Builder

	success, _ := result["success"].(bool)
	errorMsg, _ := result["error_message"].(string)

	if !success || errorMsg != "" {
		fmt.Fprintf(&output, "  \033[31m✗ 操作失败: %s\033[0m\n", errorMsg)
		return output.String()
	}

	output.WriteString("  \033[32m✓ 操作成功\033[0m\n")

	if msg, ok := result["message"].(string); ok && msg != "" {
		fmt.Fprintf(&output, "  %s\n", msg)
	}

	switch toolName {
	case "todo_add", "todo_update":
		if todoData, ok := result["todo"].(map[string]any); ok {
			output.WriteString("\n")
			output.WriteString(formatSingleTodo(todoData))
		}

	case "todo_batch_add":
		if todosData, ok := result["added_todos"].([]any); ok {
			addedCount, _ := result["added_count"].(float64)
			fmt.Fprintf(&output, "\n  \033[1m已添加 %d 个待办事项:\033[0m\n\n", int(addedCount))

			limit := min(len(todosData), 10)
			for i, todoItem := range todosData[:limit] {
				if todoMap, ok := todoItem.(map[string]any); ok {
					output.WriteString(formatSingleTodo(todoMap))
					if i < limit-1 {
						output.WriteString("  \033[90m────────────────────────────────────────\033[0m\n")
					}
				}
			}
			if len(todosData) > 10 {
				fmt.Fprintf(&output, "  \033[90m... 还有 %d 个待办事项\033[0m\n", len(todosData)-10)
			}
		}

	case "todo_list":
		if todosData, ok := result["todos"].([]any); ok {
			totalCount, _ := result["total_count"].(float64)
			fmt.Fprintf(&output, "\n  \033[1m共 %d 个待办事项:\033[0m\n\n", int(totalCount))

			if len(todosData) == 0 {
				output.WriteString("  \033[90m（暂无待办事项）\033[0m\n")
			} else {
				limit := min(len(todosData), 10)
				for i, todoItem := range todosData[:limit] {
					if todoMap, ok := todoItem.(map[string]any); ok {
						output.WriteString(formatSingleTodo(todoMap))
						if i < limit-1 {
							output.WriteString("  \033[90m────────────────────────────────────────\033[0m\n")
						}
					}
				}
				if len(todosData) > 10 {
					fmt.Fprintf(&output, "  \033[90m... 还有 %d 个待办事项\033[0m\n", len(todosData)-10)
				}
			}
		}

	case "todo_delete":

	case "todo_batch_delete":
		if deletedCount, ok := result["deleted_count"].(float64); ok {
			fmt.Fprintf(&output, "\n  已删除 %d 个待办事项\n", int(deletedCount))
		}
		if notFoundIDs, ok := result["not_found_ids"].([]any); ok && len(notFoundIDs) > 0 {
			fmt.Fprintf(&output, "  \033[33m注意: %d 个 ID 未找到\033[0m\n", len(notFoundIDs))
			if len(notFoundIDs) <= 5 {
				for _, id := range notFoundIDs {
					if idStr, ok := id.(string); ok {
						fmt.Fprintf(&output, "    - %s\n", idStr)
					}
				}
			}
		}

	case "todo_clear":
		if clearedCount, ok := result["cleared_count"].(float64); ok {
			fmt.Fprintf(&output, "\n  已清空 %d 个待办事项\n", int(clearedCount))
		}
	}

	return output.String()
}

func formatSingleTodo(todo map[string]any) string {
	var output strings.Builder

	id, _ := todo["id"].(string)
	title, _ := todo["title"].(string)
	description, _ := todo["description"].(string)
	status, _ := todo["status"].(string)
	priority, _ := todo["priority"].(string)

	statusIcon := "○"
	statusColor := "\033[90m"
	statusText := status

	switch status {
	case "pending":
		statusIcon = "○"
		statusColor = "\033[90m"
		statusText = "待处理"
	case "in_progress":
		statusIcon = "◐"
		statusColor = "\033[36m"
		statusText = "进行中"
	case "completed":
		statusIcon = "●"
		statusColor = "\033[32m"
		statusText = "已完成"
	case "cancelled":
		statusIcon = "✕"
		statusColor = "\033[31m"
		statusText = "已取消"
	}

	priorityColor := "\033[0m"
	priorityText := priority

	switch priority {
	case "low":
		priorityColor = "\033[90m"
		priorityText = "低"
	case "medium":
		priorityColor = "\033[33m"
		priorityText = "中"
	case "high":
		priorityColor = "\033[35m"
		priorityText = "高"
	case "urgent":
		priorityColor = "\033[31m"
		priorityText = "紧急"
	}

	fmt.Fprintf(&output, "  %s%s\033[0m \033[1m%s\033[0m", statusColor, statusIcon, title)
	if priority != "" {
		fmt.Fprintf(&output, " %s[%s]\033[0m", priorityColor, priorityText)
	}
	output.WriteString("\n")

	fmt.Fprintf(&output, "  │ 状态: %s%s\033[0m\n", statusColor, statusText)

	if id != "" {
		fmt.Fprintf(&output, "  │ \033[90mID: %s\033[0m\n", truncateString(id, 30))
	}

	if description != "" {
		fmt.Fprintf(&output, "  │ 描述: %s\n", truncateString(description, 120))
	}

	if createdAt, ok := todo["created_at"].(string); ok && createdAt != "" {
		fmt.Fprintf(&output, "  │ \033[90m创建时间: %s\033[0m\n", formatTime(createdAt))
	}
	if completedAt, ok := todo["completed_at"].(string); ok && completedAt != "" {
		fmt.Fprintf(&output, "  │ \033[90m完成时间: %s\033[0m\n", formatTime(completedAt))
	}

	output.WriteString("\n")

	return output.String()
}

func formatTime(timeStr string) string {
	t, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		return timeStr
	}
	return t.Format("2006-01-02 15:04:05")
}

func formatFilePatchResult(content string) string {
	var result struct {
		Message string `json:"message"`
		Results []struct {
			Path    string `json:"path"`
			Success bool   `json:"success"`
			Error   string `json:"error"`
		} `json:"results"`
		TotalFiles   int    `json:"total_files"`
		Succeeded    int    `json:"succeeded"`
		Failed       int    `json:"failed"`
		ErrorMessage string `json:"error_message"`
	}

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return ""
	}

	var output strings.Builder

	if result.ErrorMessage != "" {
		fmt.Fprintf(&output, "  \033[31m✗ %s\033[0m\n", result.ErrorMessage)
		return output.String()
	}

	if result.Failed == 0 {
		fmt.Fprintf(&output, "  \033[32m✓ %s\033[0m\n", result.Message)
	} else {
		fmt.Fprintf(&output, "  \033[33m⚠ %s\033[0m\n", result.Message)
	}

	for i, r := range result.Results {
		if i >= 20 {
			fmt.Fprintf(&output, "  │ ... 还有 %d 个文件\n", len(result.Results)-20)
			break
		}
		if r.Success {
			fmt.Fprintf(&output, "  │ \033[32m✓\033[0m %s\n", r.Path)
		} else {
			fmt.Fprintf(&output, "  │ \033[31m✗\033[0m %s: %s\n", r.Path, r.Error)
		}
	}

	return output.String()
}

// NewMarkdownCollector 创建事件 Markdown 收集器，供后台任务使用
func NewMarkdownCollector() (callback func(Event) error, getResult func() string) {
	var buf strings.Builder
	lastAgent := ""
	lastToolName := ""
	toolNamesByID := map[string]string{}
	inStream := false

	flushStream := func() {
		if inStream {
			buf.WriteString("\n")
			inStream = false
		}
	}

	callback = func(event Event) error {
		switch event.Type {
		case EventStreamChunk:
			if lastAgent != event.AgentName {
				flushStream()
				lastAgent = event.AgentName
				fmt.Fprintf(&buf, "\n\n**[%s]**\n\n", event.AgentName)
			}
			buf.WriteString(event.Content)
			inStream = true

		case EventMessage:
			if event.Content != "" {
				flushStream()
				if lastAgent != event.AgentName {
					lastAgent = event.AgentName
					fmt.Fprintf(&buf, "\n\n**[%s]**\n\n", event.AgentName)
				}
				buf.WriteString(event.Content)
			}

		case EventToolCallsPreparing:
			for _, tc := range event.ToolCalls {
				if isInternalToolName(tc.Function.Name) {
					continue
				}
				if tc.Function.Name != "" {
					lastToolName = tc.Function.Name
					if tc.ID != "" {
						toolNamesByID[tc.ID] = tc.Function.Name
					}
				}
			}

		case EventToolCalls:
			flushStream()
			for i, tc := range event.ToolCalls {
				if isInternalToolName(tc.Function.Name) {
					continue
				}
				if tc.ID != "" {
					toolNamesByID[tc.ID] = tc.Function.Name
				}
				if i == len(event.ToolCalls)-1 {
					lastToolName = tc.Function.Name
				}
				args := truncateString(tc.Function.Arguments, 100)
				display := FormatToolDisplay(tc.Function.Name)
				if args != "" {
					fmt.Fprintf(&buf, "\n\n> **[%s]** 调用: `%s`\n> 参数: `%s`", event.AgentName, display.DisplayName, args)
				} else {
					fmt.Fprintf(&buf, "\n\n> **[%s]** 调用: `%s`", event.AgentName, display.DisplayName)
				}
			}
			lastAgent = ""

		case EventToolResult:
			if event.Content != "" && !isInternalContinueContent(event.Content) {
				toolName := lastToolName
				if event.ToolCallID != "" {
					if name, ok := toolNamesByID[event.ToolCallID]; ok {
						toolName = name
					}
				}
				if formatted := formatToolResultMarkdown(event.Content, toolName); formatted != "" {
					buf.WriteString(formatted)
				}
			}
			lastAgent = ""

		case EventAction:
			switch event.ActionType {
			case ActionTransfer:
				flushStream()
				fmt.Fprintf(&buf, "\n\n> **[%s]** → %s", event.AgentName, event.Content)
				lastAgent = ""
			case ActionContextCompressStart, ActionContextCompress:
				// 跳过上下文压缩提示
			}

		case EventError:
			flushStream()
			fmt.Fprintf(&buf, "\n\n**错误 [%s]**: %s", event.AgentName, event.Error)
		}
		return nil
	}

	getResult = func() string {
		if inStream {
			buf.WriteString("\n")
		}
		return strings.TrimSpace(buf.String())
	}

	return
}

func formatToolResultMarkdown(content string, toolName string) string {
	switch toolName {
	case "search":
		return formatSearchResultMarkdown(content)
	case "dispatch_tasks":
		return formatDispatchResultMarkdown(content)
	default:
		var raw struct {
			Message      string `json:"message"`
			ErrorMessage string `json:"error_message"`
		}
		if err := json.Unmarshal([]byte(content), &raw); err == nil {
			if raw.ErrorMessage != "" {
				return fmt.Sprintf("\n\n> ✗ %s", raw.ErrorMessage)
			}
			if raw.Message != "" {
				return fmt.Sprintf("\n\n> ✓ %s", raw.Message)
			}
		}
		return ""
	}
}

func formatSearchResultMarkdown(content string) string {
	var result struct {
		Message string `json:"message"`
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Summary string `json:"summary"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return ""
	}

	var output strings.Builder
	if result.Message != "" {
		fmt.Fprintf(&output, "\n\n> ✓ %s", result.Message)
	}
	for i, r := range result.Results {
		if i >= 5 {
			fmt.Fprintf(&output, "\n>\n> _...还有 %d 条结果_", len(result.Results)-5)
			break
		}
		fmt.Fprintf(&output, "\n>\n> %d. **%s**", i+1, r.Title)
		if r.URL != "" {
			fmt.Fprintf(&output, " <%s>", r.URL)
		}
		if r.Summary != "" {
			summary := truncateString(strings.ReplaceAll(r.Summary, "\n", " "), 100)
			fmt.Fprintf(&output, "\n>    %s", summary)
		}
	}
	return output.String()
}

func formatSchedulerResult(content string, toolName string) string {
	var output strings.Builder

	switch toolName {
	case "schedule_add":
		var result struct {
			Success      bool   `json:"success"`
			Message      string `json:"message"`
			ErrorMessage string `json:"error_message"`
			Task         *struct {
				ID        string `json:"id"`
				Task      string `json:"task"`
				CronExpr  string `json:"cron_expr"`
				OneTime   bool   `json:"one_time"`
				NextRunAt string `json:"next_run_at"`
				Status    string `json:"status"`
			} `json:"task"`
		}
		if err := json.Unmarshal([]byte(content), &result); err != nil {
			return ""
		}
		if result.ErrorMessage != "" {
			fmt.Fprintf(&output, "  \033[31m✗ %s\033[0m\n", result.ErrorMessage)
		} else if result.Task != nil {
			fmt.Fprintf(&output, "  \033[32m✓ %s\033[0m\n", result.Message)
			fmt.Fprintf(&output, "  \033[90mID: %s\033[0m\n", result.Task.ID)
			fmt.Fprintf(&output, "  任务: %s\n", result.Task.Task)
			if result.Task.CronExpr != "" {
				fmt.Fprintf(&output, "  Cron: %s\n", result.Task.CronExpr)
			}
			fmt.Fprintf(&output, "  下次执行: %s\n", formatTime(result.Task.NextRunAt))
		}

	case "schedule_list":
		var result struct {
			Success      bool   `json:"success"`
			TotalCount   int    `json:"total_count"`
			ErrorMessage string `json:"error_message"`
			Tasks        []struct {
				ID        string `json:"id"`
				Task      string `json:"task"`
				CronExpr  string `json:"cron_expr"`
				OneTime   bool   `json:"one_time"`
				NextRunAt string `json:"next_run_at"`
				Status    string `json:"status"`
				LastRunAt string `json:"last_run_at"`
				Result    string `json:"result"`
			} `json:"tasks"`
		}
		if err := json.Unmarshal([]byte(content), &result); err != nil {
			return ""
		}
		if result.ErrorMessage != "" {
			fmt.Fprintf(&output, "  \033[31m✗ %s\033[0m\n", result.ErrorMessage)
		} else {
			fmt.Fprintf(&output, "  \033[32m✓ 共 %d 个定时任务\033[0m\n", result.TotalCount)
			for i, t := range result.Tasks {
				var statusIcon string
				switch t.Status {
				case "completed":
					statusIcon = "[完成]"
				case "running":
					statusIcon = "[运行]"
				case "failed":
					statusIcon = "[失败]"
				case "cancelled":
					statusIcon = "[取消]"
				default:
					statusIcon = "[等待]"
				}
				fmt.Fprintf(&output, "\n  %s \033[1m%d. %s\033[0m\n", statusIcon, i+1, t.Task)
				fmt.Fprintf(&output, "     \033[90mID: %s | 状态: %s\033[0m\n", t.ID, t.Status)
				if t.CronExpr != "" {
					fmt.Fprintf(&output, "     Cron: %s\n", t.CronExpr)
				}
				fmt.Fprintf(&output, "     下次执行: %s\n", formatTime(t.NextRunAt))
			}
		}

	case "schedule_cancel", "schedule_delete":
		var result struct {
			Success      bool   `json:"success"`
			Message      string `json:"message"`
			ErrorMessage string `json:"error_message"`
		}
		if err := json.Unmarshal([]byte(content), &result); err != nil {
			return ""
		}
		if result.ErrorMessage != "" {
			fmt.Fprintf(&output, "  \033[31m✗ %s\033[0m\n", result.ErrorMessage)
		} else {
			fmt.Fprintf(&output, "  \033[32m✓ %s\033[0m\n", result.Message)
		}
	}

	return output.String()
}

func formatDispatchResult(content string) string {
	var data struct {
		Results []struct {
			TaskIndex   int      `json:"task_index"`
			Description string   `json:"description"`
			Status      string   `json:"status"`
			Error       string   `json:"error,omitempty"`
			Operations  []string `json:"operations,omitempty"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(content), &data); err != nil || len(data.Results) == 0 {
		return ""
	}

	var b strings.Builder
	success, failed := 0, 0
	for _, r := range data.Results {
		if r.Status == "success" {
			success++
		} else {
			failed++
		}
	}
	fmt.Fprintf(&b, "  \033[1m分发完成: %d 成功, %d 失败\033[0m\n", success, failed)

	for _, r := range data.Results {
		icon := "\033[32m✓\033[0m"
		if r.Status != "success" {
			icon = "\033[31m✗\033[0m"
		}
		fmt.Fprintf(&b, "  %s [%d] %s", icon, r.TaskIndex, truncateString(r.Description, 60))
		if r.Error != "" {
			fmt.Fprintf(&b, " \033[31m— %s\033[0m", truncateString(r.Error, 40))
		}
		if len(r.Operations) > 0 {
			fmt.Fprintf(&b, " \033[90m(%d 项操作)\033[0m", len(r.Operations))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func formatDispatchResultMarkdown(content string) string {
	var data struct {
		Results []struct {
			TaskIndex   int      `json:"task_index"`
			Description string   `json:"description"`
			Status      string   `json:"status"`
			Error       string   `json:"error,omitempty"`
			Operations  []string `json:"operations,omitempty"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(content), &data); err != nil || len(data.Results) == 0 {
		return ""
	}

	var b strings.Builder
	success, failed := 0, 0
	for _, r := range data.Results {
		if r.Status == "success" {
			success++
		} else {
			failed++
		}
	}
	fmt.Fprintf(&b, "\n\n> **分发完成**: %d 成功, %d 失败\n", success, failed)
	for _, r := range data.Results {
		icon := "✓"
		if r.Status != "success" {
			icon = "✗"
		}
		fmt.Fprintf(&b, "> %s [%d] %s", icon, r.TaskIndex, r.Description)
		if r.Error != "" {
			fmt.Fprintf(&b, " — %s", r.Error)
		}
		if len(r.Operations) > 0 {
			fmt.Fprintf(&b, " (%d 项操作)", len(r.Operations))
		}
		b.WriteString("\n")
	}
	return b.String()
}
