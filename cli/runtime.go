package cli

import (
	"context"
	"errors"
	"fkteams/agents"
	"fkteams/agenttool"
	"fkteams/config"
	"fkteams/fkevent"
	"fkteams/runner"
	"fkteams/tui"
	"fkteams/version"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

const (
	runtimeExitConfirmWindow = 2 * time.Second
	runtimeExitConfirmTick   = time.Second
	runtimeSelectionNotice   = 2 * time.Second
)

type Runtime struct {
	ctx         context.Context
	session     *Session
	runner      *adk.Runner
	executor    *QueryExecutor
	cmdHandler  *CommandHandler
	exitSignals chan os.Signal
	program     *tea.Program
}

func NewRuntime(ctx context.Context, session *Session, r *adk.Runner, exitSignals chan os.Signal) *Runtime {
	executor := NewQueryExecutor(r, session.queryState)
	modeSwitcher := &sessionModeSwitcher{session: session, ctx: ctx, executor: executor}
	return &Runtime{
		ctx:         ctx,
		session:     session,
		runner:      r,
		executor:    executor,
		cmdHandler:  NewCommandHandler(modeSwitcher),
		exitSignals: exitSignals,
	}
}

func (r *Runtime) Run() error {
	defer resetTerminalModes()

	view := &runtimeQueryView{}
	r.executor.SetApproveStores(r.session.ApproveStores)
	r.executor.SetView(view)

	model := newRuntimeModel(r)
	p := tea.NewProgram(model)
	r.program = p
	view.program = p

	_, err := p.Run()
	return err
}

func resetTerminalModes() {
	_, _ = fmt.Fprint(os.Stdout, "\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?1015l\x1b[?1049l\x1b[?25h")
}

func (r *Runtime) submitQuery(input string) tea.Cmd {
	return func() tea.Msg {
		if err := r.executor.Execute(r.ctx, input); err != nil {
			return runtimeInternalErrorMsg{err: err}
		}
		return nil
	}
}

func (r *Runtime) requestCancel() tea.Cmd {
	return func() tea.Msg {
		if r.session.queryState.Cancel() {
			return runtimeCancellingMsg{}
		}
		return nil
	}
}

func (r *Runtime) requestExit() {
	select {
	case r.exitSignals <- syscall.SIGTERM:
	default:
	}
}

func (r *Runtime) switchAgent(agentName string) (string, error) {
	agentInfo := agents.GetAgentByName(agentName)
	if agentInfo == nil {
		return "", fmt.Errorf("agent not found: %s", agentName)
	}
	newAgent := agentInfo.Creator(r.ctx)
	newRunner := runner.CreateAgentRunner(r.ctx, newAgent)
	r.executor.SetRunner(newRunner)
	r.session.currentAgent = agentName
	return fmt.Sprintf("已切换到智能体: %s (%s)", agentName, agentInfo.Description), nil
}

func (r *Runtime) runLegacyCommand(input string) tea.Cmd {
	cmd := strings.TrimPrefix(input, "/")
	return tea.Exec(runtimeFuncCommand{run: func() error {
		result := r.cmdHandler.Handle(cmd)
		if result == ResultExit {
			r.requestExit()
		}
		return nil
	}}, func(err error) tea.Msg {
		if err != nil {
			return runtimeInternalErrorMsg{err: err}
		}
		return runtimeLegacyCommandDoneMsg{command: cmd}
	})
}

type runtimeFuncCommand struct {
	run func() error
}

func (c runtimeFuncCommand) Run() error {
	if c.run == nil {
		return nil
	}
	return c.run()
}

func (c runtimeFuncCommand) SetStdin(_ io.Reader)  {}
func (c runtimeFuncCommand) SetStdout(_ io.Writer) {}
func (c runtimeFuncCommand) SetStderr(_ io.Writer) {}

type runtimeModel struct {
	runtime      *Runtime
	input        textinput.Model
	width        int
	height       int
	blocks       []runtimeBlock
	activeOutput int
	activeReason int
	historyIndex int
	savedInput   string
	pastes       []string
	picker       *runtimePicker
	scrollOffset int
	selection    tui.TextSelection
	running      bool
	cancelling   bool
	status       string
	exitUntil    time.Time
	copiedUntil  time.Time
	welcome      tui.WelcomeInfo
}

type runtimeSelectionCopiedTickMsg time.Time

type runtimeBlockKind string

const (
	runtimeBlockUser      runtimeBlockKind = "user"
	runtimeBlockAssistant runtimeBlockKind = "assistant"
	runtimeBlockReasoning runtimeBlockKind = "reasoning"
	runtimeBlockTool      runtimeBlockKind = "tool"
	runtimeBlockSystem    runtimeBlockKind = "system"
	runtimeBlockError     runtimeBlockKind = "error"
	runtimeBlockDone      runtimeBlockKind = "done"
	runtimeBlockMeta      runtimeBlockKind = "meta"
	runtimeBlockBanner    runtimeBlockKind = "banner"
	runtimeBlockWelcome   runtimeBlockKind = "welcome"
)

type runtimeBlock struct {
	Kind    runtimeBlockKind
	Title   string
	Content string
}

type runtimePickerKind string

const (
	runtimePickerAgent   runtimePickerKind = "agent"
	runtimePickerCommand runtimePickerKind = "command"
	runtimePickerFile    runtimePickerKind = "file"
)

type runtimePicker struct {
	kind       runtimePickerKind
	title      string
	items      []runtimePickerItem
	matches    []int
	cursor     int
	offset     int
	height     int
	filter     string
	baseDir    string
	currentDir string
}

type runtimePickerItem struct {
	Label string
	Value string
}

func newAgentPicker() *runtimePicker {
	items := make([]runtimePickerItem, 0)
	for _, a := range agents.GetRegistry() {
		items = append(items, runtimePickerItem{
			Label: fmt.Sprintf("%s - %s", a.Name, a.Description),
			Value: a.Name,
		})
	}
	return newRuntimePicker(runtimePickerAgent, "选择智能体", items, 10)
}

func newCommandPicker() *runtimePicker {
	items := make([]runtimePickerItem, 0, len(allCommands))
	for _, c := range allCommands {
		items = append(items, runtimePickerItem{
			Label: fmt.Sprintf("%s - %s", c.Name, c.Desc),
			Value: c.Name,
		})
	}
	return newRuntimePicker(runtimePickerCommand, "选择命令", items, 10)
}

func newFilePicker(baseDir string) (*runtimePicker, error) {
	p := newRuntimePicker(runtimePickerFile, "选择文件/目录", nil, 12)
	p.baseDir = baseDir
	p.currentDir = baseDir
	if err := p.reloadFiles(); err != nil {
		return nil, err
	}
	return p, nil
}

func newRuntimePicker(kind runtimePickerKind, title string, items []runtimePickerItem, height int) *runtimePicker {
	if height <= 0 {
		height = 10
	}
	p := &runtimePicker{kind: kind, title: title, items: items, height: height}
	p.updateMatches()
	return p
}

func (p *runtimePicker) updateMatches() {
	p.matches = nil
	filter := strings.ToLower(strings.TrimSpace(p.filter))
	for i, item := range p.items {
		if filter == "" || strings.Contains(strings.ToLower(item.Label), filter) {
			p.matches = append(p.matches, i)
		}
	}
	if p.cursor >= len(p.matches) {
		p.cursor = max(0, len(p.matches)-1)
	}
	p.adjustOffset()
}

func (p *runtimePicker) adjustOffset() {
	if len(p.matches) <= p.height {
		p.offset = 0
		return
	}
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+p.height {
		p.offset = p.cursor - p.height + 1
	}
}

func (p *runtimePicker) move(delta int) {
	if len(p.matches) == 0 {
		return
	}
	p.cursor = (p.cursor + delta + len(p.matches)) % len(p.matches)
	p.adjustOffset()
}

func (p *runtimePicker) backspace() {
	if p.filter == "" {
		return
	}
	runes := []rune(p.filter)
	p.filter = string(runes[:len(runes)-1])
	p.updateMatches()
}

func (p *runtimePicker) selected() (runtimePickerItem, bool) {
	if len(p.matches) == 0 || p.cursor < 0 || p.cursor >= len(p.matches) {
		return runtimePickerItem{}, false
	}
	return p.items[p.matches[p.cursor]], true
}

func (p *runtimePicker) reloadFiles() error {
	entries, err := os.ReadDir(p.currentDir)
	if err != nil {
		return err
	}
	items := make([]runtimePickerItem, 0, len(entries)+2)
	if rel, _ := filepath.Rel(p.baseDir, p.currentDir); rel != "." {
		items = append(items, runtimePickerItem{Label: "← 返回上级目录", Value: ".."})
		items = append(items, runtimePickerItem{Label: "✓ 选择当前目录", Value: "."})
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		label := name
		if entry.IsDir() {
			label += "/"
		}
		items = append(items, runtimePickerItem{Label: label, Value: name})
	}
	p.items = items
	p.cursor = 0
	p.offset = 0
	p.filter = ""
	p.updateMatches()
	return nil
}

func (p *runtimePicker) enterDir(dir string) error {
	p.currentDir = dir
	return p.reloadFiles()
}

func (p *runtimePicker) enterParent() {
	parent := filepath.Dir(p.currentDir)
	if strings.HasPrefix(parent, p.baseDir) {
		p.currentDir = parent
		_ = p.reloadFiles()
	}
}

func (p *runtimePicker) currentRel() string {
	rel, err := filepath.Rel(p.baseDir, p.currentDir)
	if err != nil {
		return "."
	}
	return filepath.ToSlash(rel)
}

func newRuntimeModel(r *Runtime) runtimeModel {
	ti := textinput.New()
	ti.Prompt = "❯ "
	s := ti.Styles()
	s.Focused.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	ti.SetStyles(s)
	ti.SetWidth(80)
	ti.Focus()
	model := runtimeModel{
		runtime:      r,
		input:        ti,
		activeOutput: -1,
		activeReason: -1,
		historyIndex: len(r.session.InputHistory),
		status:       "就绪",
		welcome:      runtimeWelcomeInfo(r.session),
	}
	model.appendBlock(runtimeBlockWelcome, "欢迎", "")
	return model
}

func (m runtimeModel) Init() tea.Cmd { return textinput.Blink }

func (m runtimeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.SetWidth(max(20, msg.Width-8))
		return m, nil
	case runtimeExitTickMsg:
		if !m.isExitConfirming() {
			m.exitUntil = time.Time{}
			return m, nil
		}
		return m, runtimeExitTickCmd()
	case runtimeSelectionCopiedTickMsg:
		if !m.isCopiedNoticeVisible() {
			m.copiedUntil = time.Time{}
		}
		return m, nil
	case runtimeQueryStartedMsg:
		m.running = true
		m.cancelling = false
		m.status = "思考中..."
		m.activeOutput = -1
		m.activeReason = -1
		return m, nil
	case runtimeAgentEventMsg:
		m.applyEvent(msg.event)
		return m, nil
	case runtimeCancellingMsg:
		m.cancelling = true
		m.status = "正在取消当前任务..."
		return m, nil
	case runtimeQueryInterruptedMsg:
		m.running = false
		m.cancelling = false
		m.status = "任务已取消"
		m.appendBlock(runtimeBlockSystem, "任务", "已取消当前任务")
		return m, nil
	case runtimeQueryDoneMsg:
		m.running = false
		m.cancelling = false
		m.status = fmt.Sprintf("完成 · %s", msg.elapsed)
		m.activeOutput = -1
		m.activeReason = -1
		m.appendBlock(runtimeBlockDone, "完成", msg.elapsed.String())
		return m, nil
	case runtimeQueryErrorMsg:
		m.running = false
		m.cancelling = false
		m.status = "执行出错"
		m.appendBlock(runtimeBlockError, "错误", msg.err.Error())
		return m, nil
	case runtimeStatusMsg:
		m.status = msg.text
		return m, nil
	case runtimeLegacyCommandDoneMsg:
		m.status = "命令已完成"
		return m, nil
	case runtimeInternalErrorMsg:
		if msg.err != nil && !errors.Is(msg.err, context.Canceled) {
			m.appendBlock(runtimeBlockError, "错误", msg.err.Error())
		}
		return m, nil
	case tea.MouseWheelMsg:
		mouse := msg.Mouse()
		switch mouse.Button {
		case tea.MouseWheelUp:
			m.scrollTranscript(3)
		case tea.MouseWheelDown:
			m.scrollTranscript(-3)
		}
		return m, nil
	case tea.MouseClickMsg:
		mouse := msg.Mouse()
		if mouse.Button == tea.MouseLeft && mouse.Y < m.bodyHeight() {
			point := tui.TextPoint{X: mouse.X, Y: mouse.Y}
			m.selection = tui.NewTextSelection(point)
		}
		return m, nil
	case tea.MouseMotionMsg:
		if m.selection.Active {
			mouse := msg.Mouse()
			m.selection.Cursor = tui.TextPoint{X: mouse.X, Y: min(mouse.Y, max(0, m.bodyHeight()-1))}
		}
		return m, nil
	case tea.MouseReleaseMsg:
		if m.selection.Active {
			mouse := msg.Mouse()
			m.selection.Cursor = tui.TextPoint{X: mouse.X, Y: min(mouse.Y, max(0, m.bodyHeight()-1))}
			selected := strings.TrimRight(m.selectedVisibleText(), "\n")
			m.selection.Active = false
			if strings.TrimSpace(selected) != "" {
				m.selection.Copied = selected
				m.copiedUntil = time.Now().Add(runtimeSelectionNotice)
				_ = clipboard.WriteAll(selected)
				return m, runtimeSelectionCopiedTickCmd()
			}
		}
		return m, nil
	case tea.PasteMsg:
		content := msg.Content
		if strings.ContainsAny(content, "\n\r") {
			m.exitUntil = time.Time{}
			return m.insertPaste(strings.TrimRight(content, "\n\r")), nil
		}
	case tea.KeyPressMsg:
		if m.picker != nil {
			return m.updatePicker(msg)
		}
		if isRuntimeShiftEnter(msg) {
			m.exitUntil = time.Time{}
			m.insertText(tui.InlineLineBreakTag)
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c":
			if m.running {
				if m.cancelling {
					return m, nil
				}
				m.cancelling = true
				m.status = "正在取消当前任务..."
				return m, m.runtime.requestCancel()
			}
			if m.isExitConfirming() {
				m.runtime.requestExit()
				return m, tea.Quit
			}
			m.exitUntil = time.Now().Add(runtimeExitConfirmWindow)
			return m, runtimeExitTickCmd()
		case "esc":
			m.input.SetValue("")
			m.savedInput = ""
			m.pastes = nil
			m.historyIndex = len(m.runtime.session.InputHistory)
			m.exitUntil = time.Time{}
			return m, nil
		case "ctrl+v":
			if text, err := clipboard.ReadAll(); err == nil && strings.ContainsAny(text, "\n\r") {
				m.exitUntil = time.Time{}
				return m.insertPaste(strings.TrimRight(text, "\n\r")), nil
			}
		case "backspace":
			m.exitUntil = time.Time{}
			if newM, ok := m.backspacePasteTag(); ok {
				return newM, nil
			}
		case "pgup":
			m.scrollTranscript(max(1, m.bodyHeight()/2))
			return m, nil
		case "pgdown":
			m.scrollTranscript(-max(1, m.bodyHeight()/2))
			return m, nil
		case "home":
			m.scrollOffset = tui.LineCount(m.transcriptText())
			return m, nil
		case "end":
			m.scrollOffset = 0
			return m, nil
		case "up":
			if !m.running {
				m.historyPrev()
			}
			return m, nil
		case "down":
			if !m.running {
				m.historyNext()
			}
			return m, nil
		case "enter":
			if m.running {
				return m, nil
			}
			input := strings.TrimSpace(m.expandInput())
			m.input.SetValue("")
			m.pastes = nil
			m.savedInput = ""
			m.historyIndex = len(m.runtime.session.InputHistory)
			m.exitUntil = time.Time{}
			return m.handleSubmit(input)
		}
		if msg.Text != "" {
			m.exitUntil = time.Time{}
			switch msg.Text {
			case "#":
				picker, err := newFilePicker(GetWorkspaceDir())
				if err != nil {
					m.appendBlock(runtimeBlockError, "文件选择失败", err.Error())
					return m, nil
				}
				m.picker = picker
				return m, nil
			case "@":
				if m.input.Value() == "" {
					m.picker = newAgentPicker()
					return m, nil
				}
			case "/":
				if m.input.Value() == "" {
					m.picker = newCommandPicker()
					return m, nil
				}
			}
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *runtimeModel) scrollTranscript(delta int) {
	if delta == 0 {
		return
	}
	total := tui.LineCount(m.transcriptText())
	maxOffset := max(0, total-m.bodyHeight())
	m.scrollOffset += delta
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
	if m.scrollOffset > maxOffset {
		m.scrollOffset = maxOffset
	}
}

func (m runtimeModel) updatePicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.picker = nil
		return m, nil
	case "up":
		m.picker.move(-1)
		return m, nil
	case "down":
		m.picker.move(1)
		return m, nil
	case "backspace":
		m.picker.backspace()
		return m, nil
	case "enter":
		return m.acceptPicker()
	default:
		if msg.Text != "" {
			m.picker.filter += msg.Text
			m.picker.updateMatches()
		}
		return m, nil
	}
}

func (m runtimeModel) acceptPicker() (tea.Model, tea.Cmd) {
	selected, ok := m.picker.selected()
	if !ok {
		return m, nil
	}
	switch m.picker.kind {
	case runtimePickerAgent:
		m.picker = nil
		msg, err := m.runtime.switchAgent(selected.Value)
		if err != nil {
			m.appendBlock(runtimeBlockError, "智能体切换失败", err.Error())
			return m, nil
		}
		m.appendBlock(runtimeBlockSystem, "智能体", msg)
		return m, nil
	case runtimePickerCommand:
		m.picker = nil
		return m.handleSubmit(selected.Value)
	case runtimePickerFile:
		if selected.Value == ".." {
			m.picker.enterParent()
			return m, nil
		}
		if selected.Value == "." {
			value := m.picker.currentRel()
			m.picker = nil
			m.insertFileReference(value)
			return m, nil
		}
		fullPath := filepath.Join(m.picker.currentDir, selected.Value)
		info, err := os.Stat(fullPath)
		if err != nil {
			m.appendBlock(runtimeBlockError, "文件选择失败", err.Error())
			m.picker = nil
			return m, nil
		}
		if info.IsDir() {
			if err := m.picker.enterDir(fullPath); err != nil {
				m.appendBlock(runtimeBlockError, "文件选择失败", err.Error())
				m.picker = nil
			}
			return m, nil
		}
		rel, _ := filepath.Rel(m.picker.baseDir, fullPath)
		m.picker = nil
		m.insertFileReference(filepath.ToSlash(rel))
		return m, nil
	default:
		m.picker = nil
		return m, nil
	}
}

func (m *runtimeModel) insertFileReference(path string) {
	path = strings.TrimSpace(path)
	if path == "" || path == "." {
		path = "."
	}
	current := strings.TrimSpace(m.input.Value())
	ref := "#" + path
	if current == "" {
		m.input.SetValue(ref + " ")
	} else {
		m.input.SetValue(current + " " + ref + " ")
	}
	m.input.CursorEnd()
}

func (m *runtimeModel) insertText(text string) {
	pos := m.input.Position()
	runes := []rune(m.input.Value())
	before := runes[:pos]
	after := runes[pos:]
	newRunes := make([]rune, 0, len(runes)+len([]rune(text)))
	newRunes = append(newRunes, before...)
	newRunes = append(newRunes, []rune(text)...)
	newRunes = append(newRunes, after...)
	m.input.SetValue(string(newRunes))
	m.input.SetCursor(pos + len([]rune(text)))
}

func (m runtimeModel) insertPaste(content string) runtimeModel {
	value, cursor, pastes := tui.InsertInlinePaste(m.input.Value(), m.input.Position(), m.pastes, content)
	m.pastes = pastes
	m.input.SetValue(value)
	m.input.SetCursor(cursor)
	return m
}

func (m runtimeModel) backspacePasteTag() (runtimeModel, bool) {
	value, cursor, pastes, ok := tui.DeleteInlinePasteBeforeCursor(m.input.Value(), m.input.Position(), m.pastes)
	if !ok {
		return m, false
	}
	m.pastes = pastes
	m.input.SetValue(value)
	m.input.SetCursor(cursor)
	return m, true
}

func (m runtimeModel) expandInput() string {
	return tui.ExpandInlineInput(m.input.Value(), m.pastes)
}

func (m *runtimeModel) historyPrev() {
	history := m.runtime.session.InputHistory
	if len(history) == 0 {
		return
	}
	if m.historyIndex == 0 {
		return
	}
	if m.historyIndex == len(history) {
		m.savedInput = m.input.Value()
	}
	if m.historyIndex <= 0 || m.historyIndex > len(history) {
		m.historyIndex = len(history)
	}
	m.historyIndex--
	m.input.SetValue(history[m.historyIndex])
	m.pastes = nil
	m.input.CursorEnd()
}

func (m *runtimeModel) historyNext() {
	history := m.runtime.session.InputHistory
	if len(history) == 0 || m.historyIndex >= len(history) {
		return
	}
	m.historyIndex++
	if m.historyIndex == len(history) {
		m.input.SetValue(m.savedInput)
	} else {
		m.input.SetValue(history[m.historyIndex])
	}
	m.pastes = nil
	m.input.CursorEnd()
}

func (m runtimeModel) handleSubmit(input string) (tea.Model, tea.Cmd) {
	if input == "" {
		return m, nil
	}
	m.runtime.session.InputHistory = append(m.runtime.session.InputHistory, input)

	command := strings.TrimPrefix(input, "/")
	switch command {
	case "q", "quit":
		m.runtime.requestExit()
		return m, tea.Quit
	case "help":
		m.appendBlock(runtimeBlockSystem, "帮助", runtimeHelpMarkdown())
		return m, nil
	}
	if command == "list_agents" {
		m.appendBlock(runtimeBlockSystem, "可用智能体", runtimeAgentsMarkdown())
		return m, nil
	}
	if command == "switch_work_mode" {
		modeSwitcher := &sessionModeSwitcher{session: m.runtime.session, ctx: m.runtime.ctx, executor: m.runtime.executor}
		newMode, err := modeSwitcher.SwitchMode()
		if err != nil {
			m.appendBlock(runtimeBlockError, "模式切换失败", err.Error())
			return m, nil
		}
		m.appendBlock(runtimeBlockSystem, "模式", "已切换到工作模式: "+newMode)
		return m, nil
	}
	if strings.HasPrefix(command, "load_chat_history") ||
		strings.HasPrefix(command, "delete_memory") ||
		strings.HasPrefix(command, "cancel_schedule") ||
		strings.HasPrefix(command, "delete_schedule") {
		m.appendBlock(runtimeBlockSystem, "兼容命令", "该命令会临时释放 TUI 控制权执行，后续会迁移为原生面板。")
		return m, m.runtime.runLegacyCommand(command)
	}

	if agentName, query := ExtractAgentMention(input); agentName != "" {
		msg, err := m.runtime.switchAgent(agentName)
		if err != nil {
			m.appendBlock(runtimeBlockError, "智能体切换失败", err.Error())
			return m, nil
		}
		m.appendBlock(runtimeBlockSystem, "智能体", msg)
		if strings.TrimSpace(query) == "" {
			return m, nil
		}
		input = query
	}

	m.appendBlock(runtimeBlockUser, "用户", input)
	return m, m.runtime.submitQuery(input)
}

func (m runtimeModel) View() tea.View {
	bottom := m.renderBottom()
	bottomLines := tui.LineCount(bottom)
	available := m.bodyHeightForBottom(bottomLines)
	body := m.renderVisibleTranscript(available)
	var sb strings.Builder
	if body != "" {
		sb.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			sb.WriteString("\n")
		}
	}
	bodyLines := tui.LineCount(body)
	for i := bodyLines; i < available; i++ {
		sb.WriteString("\n")
	}
	sb.WriteString(bottom)
	view := tea.NewView(sb.String())
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

func (m runtimeModel) bodyHeightForBottom(bottomLines int) int {
	height := m.height
	if height <= 0 {
		height = 40
	}
	available := height - bottomLines
	if available < 0 {
		return 0
	}
	return available
}

func (m runtimeModel) visibleTranscriptLines(maxLines int) []string {
	if maxLines <= 0 {
		return nil
	}
	transcript := strings.TrimRight(m.transcriptText(), "\n")
	if transcript == "" {
		return nil
	}
	return strings.Split(tui.VisibleLines(transcript, maxLines, m.scrollOffset), "\n")
}

func (m runtimeModel) renderVisibleTranscript(maxLines int) string {
	lines := m.visibleTranscriptLines(maxLines)
	if len(lines) == 0 {
		return ""
	}
	if !m.selection.Active {
		return strings.Join(lines, "\n")
	}
	for i, line := range lines {
		lines[i] = m.selection.RenderLine(i, line)
	}
	return strings.Join(lines, "\n")
}

func (m runtimeModel) selectedVisibleText() string {
	lines := m.visibleTranscriptLines(m.bodyHeight())
	return m.selection.SelectedText(lines)
}

func (m runtimeModel) transcriptText() string {
	var transcript strings.Builder
	for i, block := range m.blocks {
		if i > 0 && shouldSpaceBeforeBlock(block.Kind) {
			transcript.WriteString("\n")
		}
		fmt.Fprintf(&transcript, "%s\n", m.renderBlock(block))
	}
	return tui.WrapLines(transcript.String(), m.contentWidth())
}

func shouldSpaceBeforeBlock(kind runtimeBlockKind) bool {
	switch kind {
	case runtimeBlockUser, runtimeBlockReasoning, runtimeBlockDone:
		return true
	default:
		return false
	}
}

func (m runtimeModel) bodyHeight() int {
	height := m.height
	if height <= 0 {
		height = 40
	}
	available := height - tui.LineCount(m.renderBottom())
	if available < 0 {
		return 0
	}
	return available
}

func (m runtimeModel) renderBottom() string {
	var sb strings.Builder
	if m.picker != nil {
		sb.WriteString(m.renderPicker())
		if !strings.HasSuffix(sb.String(), "\n") {
			sb.WriteString("\n")
		}
	}
	if m.scrollOffset > 0 {
		fmt.Fprintf(&sb, "%s\n", tui.Dim(fmt.Sprintf("已上翻 %d 行 · 滚轮向下或 End 回到底部", m.scrollOffset)))
	}
	if m.selection.Active {
		fmt.Fprintf(&sb, "%s\n", tui.Dim("松开鼠标复制选中文本"))
	} else if m.isCopiedNoticeVisible() {
		fmt.Fprintf(&sb, "%s\n", tui.Dim(tui.CopiedNotice(m.selection.Copied)))
	}
	if m.running {
		fmt.Fprintf(&sb, "%s\n", tui.Status(m.status))
	}
	if m.isExitConfirming() {
		seconds := int(time.Until(m.exitUntil).Seconds())
		if seconds < 1 {
			seconds = 1
		}
		fmt.Fprintf(&sb, "%s\n", tui.Dim("再按 ")+tui.Key("Ctrl+C")+tui.Dim(" 退出 · ")+tui.Dim(fmt.Sprintf("%ds", seconds)))
	}
	sb.WriteString(m.renderInputBox())
	return sb.String()
}

func (m runtimeModel) renderInputBox() string {
	content := m.renderInputValue()
	width := m.width
	if width <= 0 {
		width = 100
	}
	return tui.RenderRuntimeInputBox(width, content, m.inputHint())
}

func (m runtimeModel) renderInputValue() string {
	return tui.RenderInlineInputValue(m.input.View())
}

func (m runtimeModel) inputHint() string {
	return strings.Join([]string{
		runtimeModeName(m.runtime.session.CurrentMode),
		"@ 智能体",
		"# 文件",
		"/ 命令",
	}, " · ")
}

func (m runtimeModel) renderPicker() string {
	p := m.picker
	if p == nil {
		return ""
	}
	var sb strings.Builder
	title := p.title
	if p.kind == runtimePickerFile {
		displayDir := p.currentRel()
		if displayDir == "." {
			displayDir = "工作目录"
		}
		title = fmt.Sprintf("%s [%s]", p.title, displayDir)
	}
	fmt.Fprintf(&sb, "%s\n", tui.PickerTitle("? "+title))
	if p.filter != "" {
		fmt.Fprintf(&sb, "%s\n", tui.Status("  / "+p.filter))
	}
	if len(p.matches) == 0 {
		fmt.Fprintf(&sb, "%s\n", tui.Dim("  (无匹配项)"))
	} else {
		end := min(p.offset+p.height, len(p.matches))
		if p.offset > 0 {
			fmt.Fprintf(&sb, "%s\n", tui.Dim("  ↑ 更多..."))
		}
		for i := p.offset; i < end; i++ {
			item := p.items[p.matches[i]]
			if i == p.cursor {
				fmt.Fprintf(&sb, "%s\n", tui.PickerSelected("  > "+item.Label))
			} else {
				fmt.Fprintf(&sb, "    %s\n", item.Label)
			}
		}
		if end < len(p.matches) {
			fmt.Fprintf(&sb, "%s\n", tui.Dim("  ↓ 更多..."))
		}
	}
	fmt.Fprintf(&sb, "%s", tui.Dim("  ↑↓ 移动 | Enter 选择 | Esc 返回 | 输入过滤"))
	return tui.PickerBox(max(20, m.width-4), sb.String())
}

func (m runtimeModel) renderBlock(block runtimeBlock) string {
	switch block.Kind {
	case runtimeBlockUser:
		return tui.RenderUserMessageBlock(block.Content, m.width)
	case runtimeBlockWelcome:
		return tui.RenderWelcomePanel(m.welcome, m.width)
	case runtimeBlockReasoning:
		return tui.Reasoning(block.Content) + "\n"
	case runtimeBlockError:
		return tui.Error(block.Title + " " + block.Content)
	case runtimeBlockDone:
		return tui.DoneMarker() + tui.Dim(fmt.Sprintf("Worked for %s", block.Content))
	case runtimeBlockMeta:
		return tui.Dim(fmt.Sprintf("%s ID: %s", block.Title, block.Content))
	case runtimeBlockBanner:
		return tui.Banner(fmt.Sprintf("%s: %s", block.Title, block.Content))
	case runtimeBlockSystem:
		return tui.System(block.Title) + "\n" + m.runtimeRenderMarkdown(block.Content)
	case runtimeBlockTool:
		return tui.Tool(block.Content)
	default:
		return m.runtimeRenderMarkdown(block.Content)
	}
}

func (m *runtimeModel) appendBlock(kind runtimeBlockKind, title, content string) {
	m.blocks = append(m.blocks, runtimeBlock{Kind: kind, Title: title, Content: content})
	if len(m.blocks) > 200 {
		m.blocks = m.blocks[len(m.blocks)-200:]
	}
}

func (m *runtimeModel) appendOutput(agent, content string) {
	if content == "" {
		return
	}
	m.activeReason = -1
	if m.activeOutput >= 0 && m.activeOutput < len(m.blocks) && m.blocks[m.activeOutput].Kind == runtimeBlockAssistant {
		m.blocks[m.activeOutput].Content += content
		return
	}
	m.blocks = append(m.blocks, runtimeBlock{Kind: runtimeBlockAssistant, Title: agent, Content: content})
	m.activeOutput = len(m.blocks) - 1
}

func (m *runtimeModel) appendReasoning(agent, content string) {
	if content == "" {
		return
	}
	m.activeOutput = -1
	if m.activeReason >= 0 && m.activeReason < len(m.blocks) && m.blocks[m.activeReason].Kind == runtimeBlockReasoning {
		m.blocks[m.activeReason].Content += content
		return
	}
	m.blocks = append(m.blocks, runtimeBlock{Kind: runtimeBlockReasoning, Title: agent, Content: content})
	m.activeReason = len(m.blocks) - 1
}

func (m *runtimeModel) applyEvent(event fkevent.Event) {
	if fkevent.IsInternalContinueContent(event.Content) {
		return
	}
	agent := event.AgentName
	if agent == "" {
		agent = "assistant"
	}
	switch event.Type {
	case fkevent.EventReasoningChunk:
		content := event.ReasoningContent
		if content == "" {
			content = event.Content
		}
		m.appendReasoning(agent, content)
	case fkevent.EventStreamChunk:
		m.appendOutput(agent, event.Content)
	case fkevent.EventMessage:
		if event.Content != "" {
			m.appendOutput(agent, event.Content)
		}
	case fkevent.EventToolCallsPreparing:
		m.activeOutput = -1
		m.activeReason = -1
		m.appendBlock(runtimeBlockTool, "工具", runtimeToolLine("准备工具", event.ToolName, event.Content))
	case fkevent.EventToolCalls, fkevent.EventToolCallsArgsDelta:
		m.activeOutput = -1
		m.activeReason = -1
		for _, tool := range event.ToolCalls {
			m.appendBlock(runtimeBlockTool, "工具", runtimeToolCallSummary(tool))
		}
	case fkevent.EventToolResult, fkevent.EventToolResultChunk:
		m.activeOutput = -1
		m.activeReason = -1
		content := event.Content
		if len([]rune(content)) > 180 {
			content = string([]rune(content)[:180]) + "..."
		}
		m.appendBlock(runtimeBlockTool, "工具结果", runtimeToolLine("工具结果", event.ToolName, content))
	case fkevent.EventAction:
		m.activeOutput = -1
		m.activeReason = -1
		m.appendBlock(runtimeBlockSystem, string(event.ActionType), event.Content)
	case fkevent.EventError:
		msg := event.Error
		if msg == "" {
			msg = event.Content
		}
		m.appendBlock(runtimeBlockError, agent, msg)
	}
}

func (m runtimeModel) isExitConfirming() bool {
	return !m.exitUntil.IsZero() && time.Now().Before(m.exitUntil)
}

func (m runtimeModel) isCopiedNoticeVisible() bool {
	return !m.copiedUntil.IsZero() && time.Now().Before(m.copiedUntil)
}

func isRuntimeShiftEnter(msg tea.KeyPressMsg) bool {
	key := msg.Key()
	return (key.Code == tea.KeyEnter && key.Mod&tea.ModShift != 0) || msg.Keystroke() == "shift+enter"
}

func runtimeExitTickCmd() tea.Cmd {
	return tea.Tick(runtimeExitConfirmTick, func(t time.Time) tea.Msg {
		return runtimeExitTickMsg(t)
	})
}

func runtimeSelectionCopiedTickCmd() tea.Cmd {
	return tea.Tick(runtimeSelectionNotice, func(t time.Time) tea.Msg {
		return runtimeSelectionCopiedTickMsg(t)
	})
}

func runtimeToolCallSummary(tool schema.ToolCall) string {
	display := agenttool.FormatToolDisplay(tool.Function.Name)
	return runtimeToolLine("工具调用", display.DisplayName, tool.Function.Arguments)
}

func runtimeToolLine(label, name, detail string) string {
	if name == "" {
		name = "tool"
	}
	if detail == "" {
		return fmt.Sprintf("• %s: %s", label, name)
	}
	return fmt.Sprintf("• %s: %s(%s)", label, name, truncateRuntimeText(detail, 120))
}

func truncateRuntimeText(s string, limit int) string {
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "..."
}

func runtimeHelpMarkdown() string {
	return `# fkteams

| 命令 | 说明 |
|------|------|
| help | 显示帮助 |
| q / quit | 退出 |
| list_agents | 列出智能体 |
| switch_work_mode | 切换工作模式 |
| @智能体名 查询 | 切换智能体并执行 |

直接输入问题即可与智能体团队对话。`
}

func runtimeAgentsMarkdown() string {
	var sb strings.Builder
	sb.WriteString("| 智能体 | 说明 |\n")
	sb.WriteString("|--------|------|\n")
	for _, info := range agents.GetRegistry() {
		fmt.Fprintf(&sb, "| %s | %s |\n", info.Name, strings.ReplaceAll(info.Description, "|", "\\|"))
	}
	return sb.String()
}

func runtimeWelcomeInfo(session *Session) tui.WelcomeInfo {
	modelName := ""
	if mc := config.Get().ResolveModel("default"); mc != nil {
		modelName = mc.Model
		if mc.Provider != "" {
			modelName = mc.Provider + "/" + modelName
		}
	}
	return tui.WelcomeInfo{
		Version:   fmt.Sprint(version.Get()),
		Mode:      runtimeModeName(session.CurrentMode),
		SessionID: activeSessionID,
		Workspace: runtimeShortPath(GetWorkspaceDir()),
		Model:     modelName,
	}
}

func runtimeModeName(mode WorkMode) string {
	switch mode {
	case ModeDeep:
		return "深度模式"
	case ModeGroup:
		return "多智能体讨论模式"
	case ModeCustom:
		return "自定义会议模式"
	default:
		return "团队模式"
	}
}

func runtimeShortPath(path string) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		if rel, relErr := filepath.Rel(home, path); relErr == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(filepath.Join("~", rel))
		}
	}
	return filepath.ToSlash(path)
}

func (m runtimeModel) runtimeRenderMarkdown(content string) string {
	return tui.TrimRenderedIndent(tui.RenderMarkdownWithWidth(content, m.contentWidth()))
}

func (m runtimeModel) contentWidth() int {
	width := m.width
	if width <= 0 {
		width = 100
	}
	return max(20, width-2)
}
