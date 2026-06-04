package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fkteams/agents"
	"fkteams/agenttool"
	"fkteams/config"
	"fkteams/eventlog"
	"fkteams/fkevent"
	"fkteams/g"
	"fkteams/memory"
	"fkteams/runner"
	"fkteams/tools/scheduler"
	"fkteams/tui"
	"fkteams/version"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	runtimeHorizontalGutter  = 1
	runtimeDefaultAgentName  = "assistant"
	runtimeDefaultToolName   = "tool"
	runtimeWheelLines        = 1
	runtimeFastWheelLines    = 5
)

type Runtime struct {
	ctx         context.Context
	session     *Session
	runner      *adk.Runner
	executor    *QueryExecutor
	exitSignals chan os.Signal
	program     *tea.Program
}

func NewRuntime(ctx context.Context, session *Session, r *adk.Runner, exitSignals chan os.Signal) *Runtime {
	executor := NewQueryExecutor(r, session.queryState)
	return &Runtime{
		ctx:         ctx,
		session:     session,
		runner:      r,
		executor:    executor,
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
	totalTokens  int
	exitUntil    time.Time
	copiedUntil  time.Time
	welcome      tui.WelcomeInfo
	members      map[string]*runtimeMemberState
	memberTools  map[string]string
	memberView   string
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
	runtimeBlockInterrupt runtimeBlockKind = "interrupt"
	runtimeBlockMember    runtimeBlockKind = "member"
)

type runtimeBlock struct {
	Kind          runtimeBlockKind
	Title         string
	Content       string
	ToolKey       string
	ToolName      string
	ToolArgs      string
	ToolResult    string
	ToolStatus    tui.ToolStatus
	ToolHasResult bool
	MemberKey     string
	MemberName    string
	MemberStatus  string
	MemberTask    string
	MemberTools   int
}

type runtimeMemberState struct {
	Key          string
	Name         string
	Status       string
	Task         string
	Blocks       []runtimeBlock
	ActiveOutput int
	ActiveReason int
	ToolCount    int
	ScrollOffset int
	RenderCache  string
	RenderDirty  bool
}

type runtimePickerKind string

const (
	runtimePickerAgent          runtimePickerKind = "agent"
	runtimePickerCommand        runtimePickerKind = "command"
	runtimePickerFile           runtimePickerKind = "file"
	runtimePickerSession        runtimePickerKind = "session"
	runtimePickerMemoryDelete   runtimePickerKind = "memory_delete"
	runtimePickerScheduleCancel runtimePickerKind = "schedule_cancel"
	runtimePickerScheduleDelete runtimePickerKind = "schedule_delete"
	runtimePickerConfirm        runtimePickerKind = "confirm"
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
	action     string
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
			Label: fmt.Sprintf("%s - %s", runtimeCommandSyntax(c), c.Desc),
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

func newSessionPicker() (*runtimePicker, error) {
	items, err := runtimeSessionPickerItems()
	if err != nil {
		return nil, err
	}
	return newRuntimePicker(runtimePickerSession, "加载聊天历史", items, 12), nil
}

func newMemoryDeletePicker() (*runtimePicker, error) {
	entries, err := runtimeMemoryEntries()
	if err != nil {
		return nil, err
	}
	items := make([]runtimePickerItem, 0, len(entries))
	for _, entry := range entries {
		items = append(items, runtimePickerItem{
			Label: fmt.Sprintf("[%s] %s - %s", entry.Type, entry.Summary, entry.Detail),
			Value: entry.Summary,
		})
	}
	return newRuntimePicker(runtimePickerMemoryDelete, "删除长期记忆", items, 12), nil
}

func newScheduleCancelPicker() (*runtimePicker, error) {
	tasks, err := runtimeScheduledTasks("pending")
	if err != nil {
		return nil, err
	}
	items := make([]runtimePickerItem, 0, len(tasks))
	for _, task := range tasks {
		items = append(items, runtimePickerItem{
			Label: fmt.Sprintf("%s - %s (下次: %s)", task.ID, task.Task, task.NextRunAt.Format("2006-01-02 15:04")),
			Value: task.ID,
		})
	}
	return newRuntimePicker(runtimePickerScheduleCancel, "取消定时任务", items, 12), nil
}

func newScheduleDeletePicker() (*runtimePicker, error) {
	tasks, err := runtimeScheduledTasks("")
	if err != nil {
		return nil, err
	}
	items := make([]runtimePickerItem, 0, len(tasks))
	for _, task := range tasks {
		if task.Status == "running" {
			continue
		}
		items = append(items, runtimePickerItem{
			Label: fmt.Sprintf("[%s] %s - %s", task.Status, task.ID, task.Task),
			Value: task.ID,
		})
	}
	return newRuntimePicker(runtimePickerScheduleDelete, "删除定时任务", items, 12), nil
}

func newConfirmPicker(title string, action string) *runtimePicker {
	p := newRuntimePicker(runtimePickerConfirm, title, []runtimePickerItem{
		{Label: "确认", Value: "yes"},
		{Label: "取消", Value: "no"},
	}, 2)
	p.action = action
	return p
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
		members:      make(map[string]*runtimeMemberState),
		memberTools:  make(map[string]string),
	}
	model.appendBlock(runtimeBlockWelcome, "欢迎", "")
	model.appendLoadedHistory()
	return model
}

func (m runtimeModel) Init() tea.Cmd { return textinput.Blink }

func (m runtimeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		oldContentWidth := m.contentWidth()
		m.width = msg.Width
		m.height = msg.Height
		m.input.SetWidth(max(20, m.contentWidth()-2))
		if oldContentWidth != m.contentWidth() {
			m.markMembersDirty()
		}
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
		m.appendBlock(runtimeBlockInterrupt, "打断", "Interrupted · 输入新的指令继续")
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
	case runtimeInternalErrorMsg:
		if msg.err != nil && !errors.Is(msg.err, context.Canceled) {
			m.appendBlock(runtimeBlockError, "错误", msg.err.Error())
		}
		return m, nil
	case tea.MouseWheelMsg:
		mouse := msg.Mouse()
		lines := runtimeWheelDeltaLines(mouse)
		switch mouse.Button {
		case tea.MouseWheelUp:
			m.scrollCurrentView(lines)
		case tea.MouseWheelDown:
			m.scrollCurrentView(-lines)
		}
		return m, nil
	case tea.MouseClickMsg:
		mouse := msg.Mouse()
		if mouse.Button == tea.MouseLeft && m.memberView == "" {
			if key := m.hitMemberSummary(mouse); key != "" {
				m.memberView = key
				if member := m.currentMember(); member != nil {
					member.ScrollOffset = 0
				}
				return m, nil
			}
		}
		if mouse.Button == tea.MouseLeft && m.hitJumpToBottom(mouse) {
			m.setCurrentScrollOffset(0)
			m.selection.Active = false
			return m, nil
		}
		if mouse.Button == tea.MouseLeft && mouse.Y >= 0 && mouse.Y < m.viewHeight() {
			m.selection = tui.NewTextSelection(m.mouseTextPoint(mouse))
		}
		return m, nil
	case tea.MouseMotionMsg:
		if m.selection.Active {
			mouse := msg.Mouse()
			m.selection.Cursor = m.mouseTextPoint(mouse)
		}
		return m, nil
	case tea.MouseReleaseMsg:
		if m.selection.Active {
			mouse := msg.Mouse()
			m.selection.Cursor = m.mouseTextPoint(mouse)
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
		if m.memberView != "" {
			return m, nil
		}
		content := msg.Content
		if strings.ContainsAny(content, "\n\r") {
			m.exitUntil = time.Time{}
			return m.insertPaste(strings.TrimRight(content, "\n\r")), nil
		}
	case tea.KeyPressMsg:
		if m.memberView != "" {
			switch msg.String() {
			case "esc", "backspace", "left":
				m.memberView = ""
				return m, nil
			case "up":
				m.scrollCurrentView(runtimeWheelLines)
				return m, nil
			case "down":
				m.scrollCurrentView(-runtimeWheelLines)
				return m, nil
			case "pgup":
				m.scrollCurrentView(max(1, m.bodyHeight()/2))
				return m, nil
			case "pgdown":
				m.scrollCurrentView(-max(1, m.bodyHeight()/2))
				return m, nil
			case "home":
				m.setCurrentScrollOffset(tui.LineCount(m.transcriptText()))
				return m, nil
			case "end":
				m.setCurrentScrollOffset(0)
				return m, nil
			}
			if msg.String() != "ctrl+c" {
				return m, nil
			}
		}
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
				return m.startRuntimeCancel()
			}
			if m.isExitConfirming() {
				m.runtime.requestExit()
				return m, tea.Quit
			}
			m.exitUntil = time.Now().Add(runtimeExitConfirmWindow)
			return m, runtimeExitTickCmd()
		case "esc":
			if m.running {
				return m.startRuntimeCancel()
			}
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
			if newM, ok := m.backspaceInlineToken(); ok {
				return newM, nil
			}
		case "pgup":
			m.scrollCurrentView(max(1, m.bodyHeight()/2))
			return m, nil
		case "pgdown":
			m.scrollCurrentView(-max(1, m.bodyHeight()/2))
			return m, nil
		case "home":
			m.setCurrentScrollOffset(tui.LineCount(m.transcriptText()))
			return m, nil
		case "end":
			m.setCurrentScrollOffset(0)
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

func (m runtimeModel) startRuntimeCancel() (tea.Model, tea.Cmd) {
	if m.cancelling {
		return m, nil
	}
	m.cancelling = true
	m.status = "正在取消当前任务..."
	m.exitUntil = time.Time{}
	return m, m.runtime.requestCancel()
}

func (m *runtimeModel) scrollTranscript(delta int) {
	if delta == 0 {
		return
	}
	total := tui.LineCount(m.transcriptText())
	maxOffset := max(0, total-m.bodyHeight())
	next := m.currentScrollOffset() + delta
	if next < 0 {
		next = 0
	}
	if next > maxOffset {
		next = maxOffset
	}
	m.setCurrentScrollOffset(next)
}

func (m *runtimeModel) scrollCurrentView(delta int) {
	m.scrollTranscript(delta)
}

func (m runtimeModel) currentScrollOffset() int {
	if member := m.currentMember(); member != nil {
		return member.ScrollOffset
	}
	return m.scrollOffset
}

func (m *runtimeModel) setCurrentScrollOffset(offset int) {
	if member := m.currentMember(); member != nil {
		member.ScrollOffset = max(0, offset)
		return
	}
	m.scrollOffset = max(0, offset)
}

func (m runtimeModel) currentMember() *runtimeMemberState {
	if m.memberView == "" || m.members == nil {
		return nil
	}
	return m.members[m.memberView]
}

func runtimeWheelDeltaLines(mouse tea.Mouse) int {
	if mouse.Mod&(tea.ModAlt|tea.ModCtrl|tea.ModMeta) != 0 {
		return runtimeFastWheelLines
	}
	return runtimeWheelLines
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
		m.input.SetValue("@" + selected.Value + " ")
		m.input.CursorEnd()
		return m, nil
	case runtimePickerCommand:
		m.picker = nil
		m.input.SetValue("/" + selected.Value)
		m.input.CursorEnd()
		return m, nil
	case runtimePickerSession:
		m.picker = nil
		return m.loadRuntimeSession(selected.Value), nil
	case runtimePickerMemoryDelete:
		m.picker = nil
		return m.deleteRuntimeMemory(selected.Value), nil
	case runtimePickerScheduleCancel:
		m.picker = nil
		return m.cancelRuntimeSchedule(selected.Value), nil
	case runtimePickerScheduleDelete:
		m.picker = nil
		return m.deleteRuntimeSchedule(selected.Value), nil
	case runtimePickerConfirm:
		action := m.picker.action
		m.picker = nil
		if selected.Value != "yes" {
			m.appendBlock(runtimeBlockSystem, "命令", "已取消")
			return m, nil
		}
		return m.acceptRuntimeConfirmation(action), nil
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

func (m runtimeModel) backspaceInlineToken() (runtimeModel, bool) {
	value, cursor, ok := tui.DeleteInlineTokenNearCursor(m.input.Value(), m.input.Position())
	if !ok {
		return m, false
	}
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

	isCommandInput := strings.HasPrefix(input, "/")
	command := ""
	args := ""
	if isCommandInput {
		command, args = parseRuntimeCommand(input)
	}
	if runtimeShouldRecordCommandInput(input, command) {
		m.appendBlock(runtimeBlockUser, "用户", input)
	}
	if isCommandInput {
		switch command {
		case "quit":
			m.runtime.requestExit()
			return m, tea.Quit
		case "help":
			m.appendBlock(runtimeBlockSystem, "帮助", runtimeHelpMarkdown())
			return m, nil
		case "list_agents":
			m.appendBlock(runtimeBlockSystem, "可用智能体", runtimeAgentsMarkdown())
			return m, nil
		case "list_chat_history":
			m.appendBlock(runtimeBlockSystem, "聊天历史", runtimeChatHistoryMarkdown(true))
			return m, nil
		case "load_chat_history":
			if args != "" {
				return m.loadRuntimeSession(args), nil
			}
			picker, err := newSessionPicker()
			return m.openRuntimePicker(picker, err, "加载聊天历史")
		case "save_chat_history":
			return m.saveRuntimeChatHistory(), nil
		case "clear_chat_history":
			m.picker = newConfirmPicker("清空当前聊天历史", "clear_chat_history")
			return m, nil
		case "save_chat_history_to_markdown":
			return m.saveRuntimeChatHistoryMarkdown(), nil
		case "save_chat_history_to_html":
			return m.saveRuntimeChatHistoryHTML(), nil
		case "switch_work_mode":
			newMode, err := m.switchRuntimeWorkMode(args)
			if err != nil {
				m.appendBlock(runtimeBlockError, "模式切换失败", err.Error())
				return m, nil
			}
			m.welcome.Mode = runtimeModeName(m.runtime.session.CurrentMode)
			m.appendBlock(runtimeBlockSystem, "模式", "已切换到工作模式: "+newMode)
			return m, nil
		case "list_schedule":
			m.appendBlock(runtimeBlockSystem, "定时任务", runtimeScheduleMarkdown())
			return m, nil
		case "cancel_schedule":
			if args != "" {
				return m.cancelRuntimeSchedule(args), nil
			}
			picker, err := newScheduleCancelPicker()
			return m.openRuntimePicker(picker, err, "取消定时任务")
		case "delete_schedule":
			if args != "" {
				return m.deleteRuntimeSchedule(args), nil
			}
			picker, err := newScheduleDeletePicker()
			return m.openRuntimePicker(picker, err, "删除定时任务")
		case "list_memory":
			m.appendBlock(runtimeBlockSystem, "长期记忆", runtimeMemoryMarkdown())
			return m, nil
		case "delete_memory":
			if args != "" {
				return m.deleteRuntimeMemory(args), nil
			}
			picker, err := newMemoryDeletePicker()
			return m.openRuntimePicker(picker, err, "删除长期记忆")
		case "clear_memory":
			m.picker = newConfirmPicker("清空所有长期记忆", "clear_memory")
			return m, nil
		}

		m.appendBlock(runtimeBlockError, "未知命令", command)
		return m, nil
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

func runtimeShouldRecordCommandInput(input string, command string) bool {
	if command == "" {
		return false
	}
	return strings.HasPrefix(input, "/")
}

func parseRuntimeCommand(input string) (string, string) {
	line := strings.TrimSpace(strings.TrimPrefix(input, "/"))
	if line == "" {
		return "", ""
	}
	name, args, found := strings.Cut(line, " ")
	if !found {
		return name, ""
	}
	return name, strings.TrimSpace(args)
}

func runtimeCommandUsageHint(value string, cursor int) string {
	runes := []rune(value)
	if cursor != len(runes) {
		return ""
	}
	if !strings.HasPrefix(value, "/") {
		return ""
	}
	line := strings.TrimPrefix(value, "/")
	if line == "" {
		return ""
	}
	name, args, hasSpace := strings.Cut(line, " ")
	info, ok := commandInfoByName(name)
	if !ok || info.Usage == "" {
		return ""
	}
	if strings.TrimSpace(args) != "" {
		return ""
	}
	if hasSpace {
		return info.Usage
	}
	return " " + info.Usage
}

func (m runtimeModel) switchRuntimeWorkMode(arg string) (string, error) {
	if arg == "" {
		modeSwitcher := &sessionModeSwitcher{session: m.runtime.session, ctx: m.runtime.ctx, executor: m.runtime.executor}
		return modeSwitcher.SwitchMode()
	}
	newMode := ParseWorkMode(strings.ToLower(strings.TrimSpace(arg)))
	if newMode.String() != strings.ToLower(strings.TrimSpace(arg)) {
		return "", fmt.Errorf("unknown work mode: %s", arg)
	}
	if m.runtime.session.createModeRunner == nil {
		return "", fmt.Errorf("mode runner factory is not configured")
	}
	newRunner, err := m.runtime.session.createModeRunner(m.runtime.ctx, newMode)
	if err != nil {
		return "", fmt.Errorf("failed to create runner for mode %s: %w", newMode, err)
	}
	if newRunner == nil {
		return "", fmt.Errorf("failed to create runner for mode: %s", newMode)
	}
	m.runtime.session.CurrentMode = newMode
	m.runtime.session.currentAgent = ""
	m.runtime.executor.SetRunner(newRunner)
	return newMode.String(), nil
}

func (m runtimeModel) openRuntimePicker(picker *runtimePicker, err error, title string) (tea.Model, tea.Cmd) {
	if err != nil {
		m.appendBlock(runtimeBlockError, title, err.Error())
		return m, nil
	}
	if picker == nil || len(picker.items) == 0 {
		m.appendBlock(runtimeBlockSystem, title, "暂无可选择的条目")
		return m, nil
	}
	m.picker = picker
	return m, nil
}

func (m runtimeModel) saveRuntimeChatHistory() runtimeModel {
	recorder := getCliRecorder()
	historyFile := filepath.Join(CLIHistoryDir, activeSessionID, eventlog.HistoryFileName)
	if err := recorder.SaveToFile(historyFile); err != nil {
		m.appendBlock(runtimeBlockError, "保存聊天历史失败", err.Error())
		return m
	}
	saveCliSessionMetadata(activeSessionID, cliSessionTitle)
	m.appendBlock(runtimeBlockSystem, "聊天历史", "已保存到: "+historyFile)
	return m
}

func (m runtimeModel) saveRuntimeChatHistoryMarkdown() runtimeModel {
	recorder := getCliRecorder()
	filePath, err := recorder.SaveToMarkdownWithTimestamp()
	if err != nil {
		m.appendBlock(runtimeBlockError, "导出 Markdown 失败", err.Error())
		return m
	}
	m.appendBlock(runtimeBlockSystem, "聊天历史", "已导出 Markdown: "+filePath)
	return m
}

func (m runtimeModel) saveRuntimeChatHistoryHTML() runtimeModel {
	htmlFilePath, err := SaveChatHistoryToHTML()
	if err != nil {
		m.appendBlock(runtimeBlockError, "导出 HTML 失败", err.Error())
		return m
	}
	m.appendBlock(runtimeBlockSystem, "聊天历史", "已导出 HTML: "+htmlFilePath)
	return m
}

func (m runtimeModel) clearRuntimeChatHistory() runtimeModel {
	eventlog.GlobalSessionManager.Clear(activeSessionID)
	m.appendBlock(runtimeBlockSystem, "聊天历史", "已清空当前聊天历史")
	return m
}

func (m runtimeModel) loadRuntimeSession(sessionID string) runtimeModel {
	historyFile := filepath.Join(CLIHistoryDir, sessionID, eventlog.HistoryFileName)
	if _, err := os.Stat(historyFile); os.IsNotExist(err) {
		m.appendBlock(runtimeBlockError, "加载聊天历史失败", "历史文件不存在: "+historyFile)
		return m
	}

	activeSessionID = sessionID
	recorder := getCliRecorder()
	if err := recorder.LoadFromFile(historyFile); err != nil {
		m.appendBlock(runtimeBlockError, "加载聊天历史失败", err.Error())
		return m
	}
	m.welcome.SessionID = sessionID
	m.blocks = nil
	m.appendBlock(runtimeBlockWelcome, "欢迎", "")
	m.appendLoadedHistory()
	m.appendBlock(runtimeBlockSystem, "聊天历史", "已加载会话: "+sessionID)
	return m
}

func (m runtimeModel) deleteRuntimeMemory(summary string) runtimeModel {
	if g.MemoryManager == nil {
		m.appendBlock(runtimeBlockError, "删除长期记忆失败", "长期记忆未启用，请在 config.toml 中设置 [memory] enabled = true")
		return m
	}
	deleted := g.MemoryManager.Delete(summary)
	if deleted == 0 {
		m.appendBlock(runtimeBlockSystem, "长期记忆", "未找到匹配的记忆条目: "+summary)
		return m
	}
	m.appendBlock(runtimeBlockSystem, "长期记忆", fmt.Sprintf("已删除 %d 条记忆: %s", deleted, summary))
	return m
}

func (m runtimeModel) clearRuntimeMemory() runtimeModel {
	if g.MemoryManager == nil {
		m.appendBlock(runtimeBlockError, "清空长期记忆失败", "长期记忆未启用，请在 config.toml 中设置 [memory] enabled = true")
		return m
	}
	count := g.MemoryManager.Count()
	if count == 0 {
		m.appendBlock(runtimeBlockSystem, "长期记忆", "当前没有记忆条目")
		return m
	}
	g.MemoryManager.Clear()
	m.appendBlock(runtimeBlockSystem, "长期记忆", fmt.Sprintf("已清空 %d 条长期记忆", count))
	return m
}

func (m runtimeModel) cancelRuntimeSchedule(taskID string) runtimeModel {
	s := scheduler.Global()
	if s == nil {
		m.appendBlock(runtimeBlockError, "取消定时任务失败", "定时任务调度器未初始化")
		return m
	}
	resp, _ := s.ScheduleCancel(context.Background(), &scheduler.ScheduleCancelRequest{TaskID: taskID})
	if resp.ErrorMessage != "" {
		m.appendBlock(runtimeBlockError, "取消定时任务失败", resp.ErrorMessage)
		return m
	}
	m.appendBlock(runtimeBlockSystem, "定时任务", "已取消: "+taskID)
	return m
}

func (m runtimeModel) deleteRuntimeSchedule(taskID string) runtimeModel {
	s := scheduler.Global()
	if s == nil {
		m.appendBlock(runtimeBlockError, "删除定时任务失败", "定时任务调度器未初始化")
		return m
	}
	resp, _ := s.ScheduleDelete(context.Background(), &scheduler.ScheduleDeleteRequest{TaskID: taskID})
	if resp.ErrorMessage != "" {
		m.appendBlock(runtimeBlockError, "删除定时任务失败", resp.ErrorMessage)
		return m
	}
	m.appendBlock(runtimeBlockSystem, "定时任务", "已删除: "+taskID)
	return m
}

func (m runtimeModel) acceptRuntimeConfirmation(action string) runtimeModel {
	switch action {
	case "clear_chat_history":
		return m.clearRuntimeChatHistory()
	case "clear_memory":
		return m.clearRuntimeMemory()
	default:
		m.appendBlock(runtimeBlockError, "未知确认操作", action)
		return m
	}
}

func (m runtimeModel) View() tea.View {
	content := m.screenContent()
	if m.selection.Active {
		content = m.renderSelection(content)
	}
	content = m.renderFloatingCopiedNotice(content)
	content = tui.RenderRuntimeScreen(content, m.screenWidth(), m.viewHeight(), runtimeHorizontalGutter)
	view := tea.NewView(content)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

func (m runtimeModel) screenContent() string {
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
	return sb.String()
}

func (m runtimeModel) renderSelection(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = m.selection.RenderLine(i, line)
	}
	return strings.Join(lines, "\n")
}

func (m runtimeModel) renderFloatingCopiedNotice(content string) string {
	if !m.isCopiedNoticeVisible() {
		return content
	}
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return content
	}
	notice := tui.CopiedNotice(m.selection.Copied)
	rendered := tui.CenterLine(tui.Dim(notice), m.contentWidth())
	target := max(0, len(lines)-tui.LineCount(m.renderInputBox())-1)
	lines[target] = rendered
	return strings.Join(lines, "\n")
}

func (m runtimeModel) mouseTextPoint(mouse tea.Mouse) tui.TextPoint {
	return tui.TextPoint{
		X: max(0, mouse.X-runtimeHorizontalGutter),
		Y: min(mouse.Y, max(0, m.viewHeight()-1)),
	}
}

func (m runtimeModel) hitMemberSummary(mouse tea.Mouse) string {
	if mouse.Y < 0 || mouse.Y >= m.viewHeight() {
		return ""
	}
	lines := m.screenLines()
	if mouse.Y >= len(lines) {
		return ""
	}
	line := tui.StripANSI(lines[mouse.Y])
	start := strings.Index(line, "[")
	end := strings.Index(line, "]")
	if start < 0 || end <= start+1 || !strings.Contains(line[:start], "›") {
		return ""
	}
	ordinal, err := strconv.Atoi(strings.TrimSpace(line[start+1 : end]))
	if err != nil || ordinal <= 0 {
		return ""
	}
	return m.memberKeyByOrdinal(ordinal)
}

func (m runtimeModel) hitJumpToBottom(mouse tea.Mouse) bool {
	if m.currentScrollOffset() <= 0 {
		return false
	}
	y, startX, endX := m.jumpToBottomBounds()
	return mouse.Y == y && mouse.X >= startX && mouse.X < endX
}

func (m runtimeModel) jumpToBottomBounds() (int, int, int) {
	label := tui.StripANSI(tui.JumpToBottomButton())
	labelWidth := tui.CellWidth(label)
	for y, line := range strings.Split(m.screenContent(), "\n") {
		x := strings.Index(tui.StripANSI(line), label)
		if x >= 0 {
			startX := runtimeHorizontalGutter + x
			return y, startX, startX + labelWidth
		}
	}
	return -1, -1, -1
}

func (m runtimeModel) screenLines() []string {
	return strings.Split(m.screenContent(), "\n")
}

func (m runtimeModel) viewHeight() int {
	height := m.height
	if height <= 0 {
		height = 40
	}
	return height
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
	return strings.Split(tui.VisibleLines(transcript, maxLines, m.currentScrollOffset()), "\n")
}

func (m runtimeModel) renderVisibleTranscript(maxLines int) string {
	lines := m.visibleTranscriptLines(maxLines)
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func (m runtimeModel) selectedVisibleText() string {
	lines := m.screenLines()
	return m.selection.SelectedText(lines)
}

func (m runtimeModel) transcriptText() string {
	if member := m.currentMember(); member != nil {
		header := tui.Banner("成员详情: "+member.Name) + "\n" + tui.Dim("Esc / Backspace 返回主界面")
		if member.Task != "" {
			header += "\n" + tui.Dim("目标: "+truncateRuntimeText(member.Task, max(20, m.contentWidth()-6)))
		}
		body := m.memberBlocksText(member)
		if strings.TrimSpace(body) == "" {
			return header
		}
		return header + "\n\n" + body
	}
	return m.blocksText(m.blocks)
}

func (m runtimeModel) memberBlocksText(member *runtimeMemberState) string {
	if member == nil {
		return ""
	}
	if !member.RenderDirty && member.RenderCache != "" {
		return member.RenderCache
	}
	member.RenderCache = m.blocksText(member.Blocks)
	member.RenderDirty = false
	return member.RenderCache
}

func (m runtimeModel) blocksText(blocks []runtimeBlock) string {
	var transcript strings.Builder
	for i, block := range blocks {
		if i > 0 && shouldSpaceBeforeBlock(blocks[i-1].Kind, block.Kind) {
			transcript.WriteString("\n")
		}
		fmt.Fprintf(&transcript, "%s\n", m.renderBlock(block))
	}
	return tui.WrapLines(transcript.String(), m.contentWidth())
}

func shouldSpaceBeforeBlock(prev runtimeBlockKind, current runtimeBlockKind) bool {
	switch current {
	case runtimeBlockUser, runtimeBlockReasoning, runtimeBlockDone:
		return true
	case runtimeBlockSystem, runtimeBlockError:
		return prev == runtimeBlockUser
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
	statusStarted := false
	writeStatusLine := func(line string) {
		if !statusStarted {
			if sb.Len() > 0 && !strings.HasSuffix(sb.String(), "\n") {
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
			statusStarted = true
		}
		fmt.Fprintf(&sb, "%s\n", line)
	}
	if m.picker != nil {
		sb.WriteString(m.renderPicker())
		if !strings.HasSuffix(sb.String(), "\n") {
			sb.WriteString("\n")
		}
	}
	if m.currentScrollOffset() > 0 {
		writeStatusLine(tui.CenterLine(tui.JumpToBottomButton(), m.contentWidth()))
	}
	if m.running {
		writeStatusLine(tui.Status(m.status))
	}
	if m.isExitConfirming() {
		seconds := int(time.Until(m.exitUntil).Seconds())
		if seconds < 1 {
			seconds = 1
		}
		writeStatusLine(tui.Dim("再按 ") + tui.Key("Ctrl+C") + tui.Dim(" 退出 · ") + tui.Dim(fmt.Sprintf("%ds", seconds)))
	}
	if tokenStatus := m.tokenStatus(); tokenStatus != "" {
		fmt.Fprintf(&sb, "%s\n", tui.RightLine(tui.Dim(tokenStatus), m.contentWidth()))
	}
	sb.WriteString(m.renderInputBox())
	return sb.String()
}

func (m runtimeModel) renderInputBox() string {
	if m.memberView != "" {
		return tui.RenderRuntimeInputBox(max(24, m.contentWidth()), m.renderMemberDetailInputValue(), m.memberDetailHint())
	}
	content := m.renderInputValue()
	return tui.RenderRuntimeInputBox(max(24, m.contentWidth()), content, m.inputHint())
}

func (m runtimeModel) renderMemberDetailInputValue() string {
	return tui.Dim("当前为成员详情，返回主界面后继续输入")
}

func (m runtimeModel) renderInputValue() string {
	value := m.input.Value()
	rendered := tui.PromptMarker() + tui.RenderInlineInputValueAtCursor(value, m.input.Position())
	if hint := runtimeCommandUsageHint(value, m.input.Position()); hint != "" {
		rendered += tui.Dim(hint)
	}
	return rendered
}

func (m runtimeModel) memberDetailHint() string {
	return strings.Join([]string{
		"成员详情",
		"Esc/Backspace 返回",
		"↑↓/PgUp/PgDn 滚动",
		"End 到底部",
		"Ctrl+C 退出",
	}, " · ")
}

func (m runtimeModel) inputHint() string {
	return strings.Join([]string{
		runtimeModeName(m.runtime.session.CurrentMode),
		"@ 智能体",
		"# 文件",
		"/ 命令",
	}, " · ")
}

func (m runtimeModel) tokenStatus() string {
	if m.totalTokens <= 0 {
		return ""
	}
	return fmt.Sprintf("%d tokens", m.totalTokens)
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
	return tui.PickerBox(max(20, m.contentWidth()), sb.String())
}

func (m runtimeModel) renderBlock(block runtimeBlock) string {
	switch block.Kind {
	case runtimeBlockUser:
		return tui.RenderUserMessageBlock(block.Content, m.contentWidth())
	case runtimeBlockWelcome:
		return tui.RenderWelcomePanel(m.welcome, m.contentWidth())
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
	case runtimeBlockInterrupt:
		return tui.Interrupted(block.Content)
	case runtimeBlockMember:
		return m.renderMemberSummary(block)
	case runtimeBlockSystem:
		return tui.System(block.Title) + "\n" + m.runtimeRenderMarkdown(block.Content)
	case runtimeBlockTool:
		return runtimeRenderToolBlock(block)
	default:
		return m.runtimeRenderMarkdown(block.Content)
	}
}

func (m runtimeModel) renderMemberSummary(block runtimeBlock) string {
	ordinal := m.memberOrdinal(block.MemberKey)
	status := runtimeMemberStatusText(block.MemberStatus)
	line := fmt.Sprintf("› [%d] %s  %s · 工具 %d · Enter/点击查看",
		ordinal,
		emptyRuntimeMemberName(block.MemberName),
		status,
		block.MemberTools,
	)
	if block.MemberStatus == "running" || block.MemberStatus == "error" {
		if member := m.members[block.MemberKey]; member != nil {
			for _, toolLine := range tui.RenderToolChainLines(runtimeMemberToolChainItems(member), max(20, m.contentWidth()-4)) {
				line += "\n" + tui.Dim(toolLine)
			}
		}
	}
	switch block.MemberStatus {
	case "done":
		return tui.System(line)
	case "error":
		return tui.Error(line)
	default:
		return tui.Status(line)
	}
}

func runtimeMemberStatusText(status string) string {
	switch status {
	case "done":
		return "已完成"
	case "error":
		return "失败"
	case "running":
		return "运行中"
	default:
		return "等待中"
	}
}

func (m runtimeModel) memberOrdinal(key string) int {
	ordinal := 0
	for _, block := range m.blocks {
		if block.Kind != runtimeBlockMember {
			continue
		}
		ordinal++
		if block.MemberKey == key {
			return ordinal
		}
	}
	return ordinal + 1
}

func (m runtimeModel) memberKeyByOrdinal(ordinal int) string {
	current := 0
	for _, block := range m.blocks {
		if block.Kind != runtimeBlockMember {
			continue
		}
		current++
		if current == ordinal {
			return block.MemberKey
		}
	}
	return ""
}

func runtimeRenderToolBlock(block runtimeBlock) string {
	if block.ToolName == "" {
		if block.Content != "" {
			return block.Content
		}
		block.ToolName = runtimeDefaultToolName
	}
	if block.ToolHasResult {
		return tui.ToolResult(block.ToolName, block.ToolArgs, block.ToolResult, block.ToolStatus)
	}
	return tui.ToolCall(block.ToolName, block.ToolArgs, block.ToolStatus)
}

func (m *runtimeModel) appendBlock(kind runtimeBlockKind, title, content string) {
	m.blocks = append(m.blocks, runtimeBlock{Kind: kind, Title: title, Content: content})
	m.trimBlocks()
}

func (m *runtimeModel) appendHistoryBlock(block runtimeBlock) {
	m.blocks = append(m.blocks, block)
	m.trimBlocks()
}

func (m *runtimeModel) trimBlocks() {
	if len(m.blocks) > 200 {
		m.blocks = m.blocks[len(m.blocks)-200:]
	}
}

func (m *runtimeModel) appendLoadedHistory() {
	recorder := getCliRecorder()
	messages := recorder.GetMessages()
	if len(messages) == 0 {
		return
	}
	for _, msg := range messages {
		m.appendHistoryMessage(msg)
	}
	m.activeOutput = -1
	m.activeReason = -1
}

func (m *runtimeModel) appendHistoryMessage(msg eventlog.AgentMessage) {
	if msg.IsMemberEvent || msg.MemberCallID != "" {
		m.appendHistoryMemberMessage(msg)
		return
	}
	agent := msg.AgentName
	if agent == "" {
		agent = runtimeDefaultAgentName
	}
	if msg.MemberName != "" {
		agent = msg.MemberName
	}
	if agent == "用户" {
		content := strings.TrimSpace(msg.GetTextContent())
		if content != "" {
			m.appendHistoryBlock(runtimeBlock{Kind: runtimeBlockUser, Title: "用户", Content: content})
		}
		return
	}
	for _, event := range msg.Events {
		switch event.Type {
		case eventlog.MsgTypeReasoning:
			if event.Content != "" {
				m.appendHistoryBlock(runtimeBlock{Kind: runtimeBlockReasoning, Title: agent, Content: event.Content})
			}
		case eventlog.MsgTypeText:
			if event.Content != "" {
				m.appendHistoryBlock(runtimeBlock{Kind: runtimeBlockAssistant, Title: agent, Content: event.Content})
			}
		case eventlog.MsgTypeToolCall:
			if event.ToolCall != nil {
				m.appendHistoryToolCall(event.ToolCall)
			}
		case eventlog.MsgTypeAction:
			if event.Action != nil && (event.Action.ActionType != "" || event.Action.Content != "") {
				m.appendHistoryBlock(runtimeBlock{Kind: runtimeBlockSystem, Title: string(event.Action.ActionType), Content: event.Action.Content})
			}
		case eventlog.MsgTypeError:
			if event.Content != "" {
				m.appendHistoryBlock(runtimeBlock{Kind: runtimeBlockError, Title: agent, Content: event.Content})
			}
		}
	}
}

func (m *runtimeModel) appendHistoryMemberMessage(msg eventlog.AgentMessage) {
	key := msg.MemberCallID
	if key == "" {
		key = msg.SpanID
	}
	if key == "" {
		key = msg.AgentName + "|" + msg.RunPath
	}
	name := msg.MemberName
	if name == "" {
		name = msg.AgentName
	}
	member := m.ensureHistoryMember(key, name, "", "done")
	if member == nil {
		return
	}
	for _, event := range msg.Events {
		switch event.Type {
		case eventlog.MsgTypeReasoning:
			if event.Content != "" {
				member.Blocks = append(member.Blocks, runtimeBlock{Kind: runtimeBlockReasoning, Title: name, Content: event.Content})
			}
		case eventlog.MsgTypeText:
			if event.Content != "" {
				member.Blocks = append(member.Blocks, runtimeBlock{Kind: runtimeBlockAssistant, Title: name, Content: event.Content})
			}
		case eventlog.MsgTypeToolCall:
			if event.ToolCall != nil {
				m.appendHistoryMemberToolCall(member, event.ToolCall)
			}
		case eventlog.MsgTypeAction:
			if event.Action != nil && (event.Action.ActionType != "" || event.Action.Content != "") {
				member.Blocks = append(member.Blocks, runtimeBlock{Kind: runtimeBlockSystem, Title: string(event.Action.ActionType), Content: event.Action.Content})
			}
		case eventlog.MsgTypeError:
			if event.Content != "" {
				member.Blocks = append(member.Blocks, runtimeBlock{Kind: runtimeBlockError, Title: name, Content: event.Content})
			}
		}
	}
	member.Status = "done"
	member.markDirty()
	m.syncMemberSummary(member)
}

func (m *runtimeModel) appendHistoryToolCall(tool *eventlog.ToolCallRecord) {
	name := tool.DisplayName
	if name == "" {
		name = tool.Name
	}
	if tool.Kind == agenttool.ToolKindAgent {
		key := tool.ID
		if key == "" {
			key = tool.Ref
		}
		if key == "" {
			key = tool.SpanID
		}
		member := m.ensureHistoryMember(key, tool.Target, runtimeAgentTaskFromCompleteArgs(tool.Arguments), "done")
		if member == nil {
			return
		}
		m.registerMemberTool(member.Key, tool.Ref, tool.ID, tool.SpanID, tool.Name)
		if strings.TrimSpace(tool.Result) != "" && len(member.Blocks) == 0 {
			member.Blocks = append(member.Blocks, runtimeBlock{Kind: runtimeBlockAssistant, Title: member.Name, Content: tool.Result})
			member.markDirty()
		}
		m.syncMemberSummary(member)
		return
	}
	block := runtimeBlock{
		Kind:       runtimeBlockTool,
		ToolKey:    tool.Ref,
		ToolName:   emptyRuntimeToolName(name),
		ToolArgs:   tool.Arguments,
		ToolStatus: tui.ToolStatusDone,
	}
	if tool.Result != "" {
		block.ToolResult = tool.Result
		block.ToolHasResult = true
	}
	m.appendHistoryBlock(block)
}

func (m *runtimeModel) appendHistoryMemberToolCall(member *runtimeMemberState, tool *eventlog.ToolCallRecord) {
	if member == nil || tool == nil {
		return
	}
	name := tool.DisplayName
	if name == "" {
		name = tool.Name
	}
	block := runtimeBlock{
		Kind:       runtimeBlockTool,
		ToolKey:    tool.Ref,
		ToolName:   emptyRuntimeToolName(name),
		ToolArgs:   tool.Arguments,
		ToolStatus: tui.ToolStatusDone,
	}
	if strings.TrimSpace(tool.Result) != "" {
		block.ToolResult = tool.Result
		block.ToolHasResult = true
	}
	member.Blocks = append(member.Blocks, block)
	member.ToolCount++
	member.markDirty()
}

func (m *runtimeModel) ensureHistoryMember(key, name, task, status string) *runtimeMemberState {
	if key == "" {
		return nil
	}
	if m.members == nil {
		m.members = make(map[string]*runtimeMemberState)
	}
	member := m.members[key]
	if member == nil {
		member = &runtimeMemberState{
			Key:          key,
			Name:         emptyRuntimeMemberName(name),
			Status:       status,
			Task:         task,
			ActiveOutput: -1,
			ActiveReason: -1,
			RenderDirty:  true,
		}
		if member.Status == "" {
			member.Status = "done"
		}
		m.members[key] = member
		m.syncMemberSummary(member)
		return member
	}
	if name != "" {
		member.Name = name
	}
	if shouldReplaceRuntimeMemberTask(member.Task, task) {
		member.Task = task
	}
	if status != "" {
		member.Status = status
	}
	member.markDirty()
	m.syncMemberSummary(member)
	return member
}

func shouldReplaceRuntimeMemberTask(current, next string) bool {
	next = strings.TrimSpace(next)
	if next == "" {
		return false
	}
	current = strings.TrimSpace(current)
	if current == "" {
		return true
	}
	return strings.HasPrefix(current, "{") || strings.HasPrefix(current, "[")
}

func (m *runtimeModel) ensureMember(event fkevent.Event) *runtimeMemberState {
	key := runtimeMemberKey(event)
	if mapped := m.memberKeyForAliases(runtimeMemberEventAliases(event)...); mapped != "" {
		key = mapped
	}
	if key == "" {
		return nil
	}
	if m.members == nil {
		m.members = make(map[string]*runtimeMemberState)
	}
	member := m.members[key]
	if member == nil {
		member = &runtimeMemberState{
			Key:          key,
			Name:         runtimeMemberName(event),
			Status:       "running",
			ActiveOutput: -1,
			ActiveReason: -1,
			RenderDirty:  true,
		}
		m.members[key] = member
		m.blocks = append(m.blocks, runtimeBlock{
			Kind:         runtimeBlockMember,
			MemberKey:    key,
			MemberName:   member.Name,
			MemberStatus: member.Status,
		})
	} else if name := runtimeMemberName(event); name != "" {
		if member.Name != name {
			member.markDirty()
		}
		member.Name = name
	}
	m.registerMemberTool(member.Key, runtimeMemberEventAliases(event)...)
	m.syncMemberSummary(member)
	return member
}

func (m *runtimeModel) ensureAgentToolMember(key, name, task string) *runtimeMemberState {
	if key == "" {
		return nil
	}
	if m.members == nil {
		m.members = make(map[string]*runtimeMemberState)
	}
	if mapped := m.memberKeyForAliases(key); mapped != "" {
		key = mapped
	}
	member := m.members[key]
	if member == nil {
		member = &runtimeMemberState{
			Key:          key,
			Name:         emptyRuntimeMemberName(name),
			Status:       "running",
			Task:         runtimeAgentTaskFromCompleteArgs(task),
			ActiveOutput: -1,
			ActiveReason: -1,
			RenderDirty:  true,
		}
		m.members[key] = member
	} else {
		if name != "" {
			if member.Name != name {
				member.markDirty()
			}
			member.Name = name
		}
		if parsed := runtimeAgentTaskFromCompleteArgs(task); shouldReplaceRuntimeMemberTask(member.Task, parsed) {
			member.Task = parsed
			member.markDirty()
		}
	}
	m.registerMemberTool(key, key)
	m.syncMemberSummary(member)
	return member
}

func (m *runtimeModel) registerMemberTool(memberKey string, aliases ...string) {
	if memberKey == "" {
		return
	}
	if m.memberTools == nil {
		m.memberTools = make(map[string]string)
	}
	for _, alias := range aliases {
		if alias == "" {
			continue
		}
		m.memberTools[alias] = memberKey
	}
}

func (m runtimeModel) memberKeyForAliases(aliases ...string) string {
	if m.memberTools == nil {
		return ""
	}
	for _, alias := range aliases {
		if alias == "" {
			continue
		}
		if key := m.memberTools[alias]; key != "" {
			return key
		}
	}
	return ""
}

func (m runtimeModel) memberForToolEvent(event fkevent.Event) (*runtimeMemberState, string) {
	if m.members == nil || m.memberTools == nil {
		return nil, ""
	}
	for _, alias := range runtimeDirectToolEventAliases(event) {
		if alias == "" {
			continue
		}
		if key := m.memberTools[alias]; key != "" {
			return m.members[key], key
		}
	}
	return nil, ""
}

func (m *runtimeModel) syncMemberSummary(member *runtimeMemberState) {
	if member == nil {
		return
	}
	for i := range m.blocks {
		if m.blocks[i].Kind == runtimeBlockMember && m.blocks[i].MemberKey == member.Key {
			m.blocks[i].MemberName = member.Name
			m.blocks[i].MemberStatus = member.Status
			m.blocks[i].MemberTask = member.Task
			m.blocks[i].MemberTools = member.ToolCount
			return
		}
	}
	m.blocks = append(m.blocks, runtimeBlock{
		Kind:         runtimeBlockMember,
		MemberKey:    member.Key,
		MemberName:   member.Name,
		MemberStatus: member.Status,
		MemberTask:   member.Task,
		MemberTools:  member.ToolCount,
	})
}

func runtimeMemberToolChainItems(member *runtimeMemberState) []tui.ToolChainItem {
	if member == nil {
		return nil
	}
	items := make([]tui.ToolChainItem, 0)
	for _, block := range member.Blocks {
		if block.Kind != runtimeBlockTool {
			continue
		}
		item := tui.ToolChainItem{
			Name:   block.ToolName,
			Args:   block.ToolArgs,
			Status: string(block.ToolStatus),
		}
		if block.ToolStatus == tui.ToolStatusError {
			item.Error = block.ToolResult
		}
		items = append(items, item)
	}
	return items
}

func (m *runtimeModel) upsertToolCall(key, name, args string, status tui.ToolStatus) {
	idx := m.findToolBlock(key)
	if idx < 0 {
		m.blocks = append(m.blocks, runtimeBlock{
			Kind:       runtimeBlockTool,
			ToolKey:    key,
			ToolName:   emptyRuntimeToolName(name),
			ToolArgs:   args,
			ToolStatus: status,
		})
		return
	}
	block := &m.blocks[idx]
	if name != "" {
		block.ToolName = name
	}
	if args != "" {
		block.ToolArgs = args
	}
	block.ToolStatus = status
}

func (m *runtimeModel) upsertToolResult(key, name, result string, status tui.ToolStatus, appendResult bool) {
	idx := m.findToolBlock(key)
	if idx < 0 {
		m.blocks = append(m.blocks, runtimeBlock{
			Kind:          runtimeBlockTool,
			ToolKey:       key,
			ToolName:      emptyRuntimeToolName(name),
			ToolResult:    result,
			ToolStatus:    status,
			ToolHasResult: true,
		})
		return
	}
	block := &m.blocks[idx]
	if name != "" {
		block.ToolName = name
	}
	if appendResult {
		block.ToolResult += result
	} else {
		block.ToolResult = result
	}
	block.ToolStatus = status
	block.ToolHasResult = true
}

func (m runtimeModel) findToolBlock(key string) int {
	if key == "" {
		return -1
	}
	for i := len(m.blocks) - 1; i >= 0; i-- {
		block := m.blocks[i]
		if block.Kind == runtimeBlockTool && block.ToolKey == key {
			return i
		}
	}
	return -1
}

func (s *runtimeMemberState) markDirty() {
	if s == nil {
		return
	}
	s.RenderDirty = true
}

func (m *runtimeModel) markMembersDirty() {
	for _, member := range m.members {
		member.markDirty()
	}
}

func (s *runtimeMemberState) appendOutput(agent, content string) {
	if content == "" {
		return
	}
	s.markDirty()
	s.ActiveReason = -1
	if s.ActiveOutput >= 0 && s.ActiveOutput < len(s.Blocks) && s.Blocks[s.ActiveOutput].Kind == runtimeBlockAssistant {
		s.Blocks[s.ActiveOutput].Content += content
		return
	}
	s.Blocks = append(s.Blocks, runtimeBlock{Kind: runtimeBlockAssistant, Title: agent, Content: content})
	s.ActiveOutput = len(s.Blocks) - 1
}

func (s *runtimeMemberState) appendReasoning(agent, content string) {
	if content == "" {
		return
	}
	s.markDirty()
	s.ActiveOutput = -1
	if s.ActiveReason >= 0 && s.ActiveReason < len(s.Blocks) && s.Blocks[s.ActiveReason].Kind == runtimeBlockReasoning {
		s.Blocks[s.ActiveReason].Content += content
		return
	}
	s.Blocks = append(s.Blocks, runtimeBlock{Kind: runtimeBlockReasoning, Title: agent, Content: content})
	s.ActiveReason = len(s.Blocks) - 1
}

func (s *runtimeMemberState) upsertToolCall(key, name, args string, status tui.ToolStatus) {
	s.markDirty()
	idx := s.findToolBlock(key)
	if idx < 0 {
		s.Blocks = append(s.Blocks, runtimeBlock{
			Kind:       runtimeBlockTool,
			ToolKey:    key,
			ToolName:   emptyRuntimeToolName(name),
			ToolArgs:   args,
			ToolStatus: status,
		})
		s.ToolCount++
		return
	}
	block := &s.Blocks[idx]
	if name != "" {
		block.ToolName = name
	}
	if args != "" {
		block.ToolArgs = args
	}
	block.ToolStatus = status
}

func (s *runtimeMemberState) upsertToolResult(key, name, result string, status tui.ToolStatus, appendResult bool) {
	s.markDirty()
	idx := s.findToolBlock(key)
	if idx < 0 {
		s.Blocks = append(s.Blocks, runtimeBlock{
			Kind:          runtimeBlockTool,
			ToolKey:       key,
			ToolName:      emptyRuntimeToolName(name),
			ToolResult:    result,
			ToolStatus:    status,
			ToolHasResult: true,
		})
		s.ToolCount++
		return
	}
	block := &s.Blocks[idx]
	if name != "" {
		block.ToolName = name
	}
	if appendResult {
		block.ToolResult += result
	} else {
		block.ToolResult = result
	}
	block.ToolStatus = status
	block.ToolHasResult = true
}

func (s runtimeMemberState) findToolBlock(key string) int {
	if key == "" {
		return -1
	}
	for i := len(s.Blocks) - 1; i >= 0; i-- {
		block := s.Blocks[i]
		if block.Kind == runtimeBlockTool && block.ToolKey == key {
			return i
		}
	}
	return -1
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
	if event.TotalTokens > 0 {
		m.totalTokens = event.TotalTokens
	}
	if event.IsMemberEvent || event.MemberCallID != "" || event.MemberName != "" {
		m.applyMemberEvent(event)
		return
	}
	agent := event.AgentName
	if agent == "" {
		agent = runtimeDefaultAgentName
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
		if _, ok := runtimeAgentToolDisplay(event.ToolName); ok {
			return
		}
		m.upsertToolCall(runtimeToolEventKey(event), event.ToolName, event.Content, tui.ToolStatusRunning)
	case fkevent.EventToolCalls, fkevent.EventToolCallsArgsDelta:
		m.activeOutput = -1
		m.activeReason = -1
		if event.Type == fkevent.EventToolCallsArgsDelta {
			if member, _ := m.memberForToolEvent(event); member != nil {
				member.Status = "running"
				m.syncMemberSummary(member)
				return
			}
			if _, ok := runtimeAgentToolDisplay(event.ToolName); ok {
				return
			}
			if event.ToolName == "" && runtimeLikelyPendingAgentToolArgs(event) {
				return
			}
			m.upsertToolCall(runtimeToolEventKey(event), event.ToolName, event.Content, tui.ToolStatusRunning)
			return
		}
		for _, tool := range event.ToolCalls {
			if display, ok := runtimeAgentToolDisplay(tool.Function.Name); ok {
				aliases := runtimeAgentToolCallAliases(event, tool)
				key := runtimeAgentToolCallKey(event, tool)
				if mapped := m.memberKeyForAliases(aliases...); mapped != "" {
					key = mapped
				}
				if member := m.ensureAgentToolMember(key, display.Target, tool.Function.Arguments); member != nil {
					m.registerMemberTool(member.Key, aliases...)
				}
				continue
			}
			display := agenttool.FormatToolDisplay(tool.Function.Name)
			key := runtimeToolCallKey(event, tool)
			m.upsertToolCall(key, display.DisplayName, tool.Function.Arguments, tui.ToolStatusRunning)
		}
	case fkevent.EventToolResult, fkevent.EventToolResultChunk:
		m.activeOutput = -1
		m.activeReason = -1
		if m.applyAgentToolResult(event) {
			return
		}
		m.upsertToolResult(runtimeToolEventKey(event), event.ToolName, event.Content, tui.ToolStatusDone, event.Type == fkevent.EventToolResultChunk)
	case fkevent.EventAction:
		m.activeOutput = -1
		m.activeReason = -1
		if event.ActionType != "" || event.Content != "" {
			m.appendBlock(runtimeBlockSystem, string(event.ActionType), event.Content)
		}
	case fkevent.EventError:
		msg := event.Error
		if msg == "" {
			msg = event.Content
		}
		if member, _ := m.memberForToolEvent(event); member != nil {
			member.Status = "error"
			if msg != "" {
				member.markDirty()
				member.Blocks = append(member.Blocks, runtimeBlock{Kind: runtimeBlockError, Title: member.Name, Content: msg})
			}
			m.syncMemberSummary(member)
			return
		}
		if event.ToolName != "" {
			m.upsertToolResult(runtimeToolEventKey(event), event.ToolName, msg, tui.ToolStatusError, false)
			return
		}
		m.appendBlock(runtimeBlockError, agent, msg)
	}
}

func (m *runtimeModel) applyMemberEvent(event fkevent.Event) {
	member := m.ensureMember(event)
	if member == nil {
		return
	}
	member.Status = "running"
	agent := event.AgentName
	if agent == "" {
		agent = member.Name
	}
	switch event.Type {
	case fkevent.EventReasoningChunk:
		content := event.ReasoningContent
		if content == "" {
			content = event.Content
		}
		member.appendReasoning(agent, content)
	case fkevent.EventStreamChunk:
		member.appendOutput(agent, event.Content)
	case fkevent.EventMessage:
		if event.Content != "" {
			member.appendOutput(agent, event.Content)
			member.Status = "done"
		}
	case fkevent.EventToolCallsPreparing:
		member.ActiveOutput = -1
		member.ActiveReason = -1
		member.upsertToolCall(runtimeToolEventKey(event), event.ToolName, event.Content, tui.ToolStatusRunning)
	case fkevent.EventToolCalls, fkevent.EventToolCallsArgsDelta:
		member.ActiveOutput = -1
		member.ActiveReason = -1
		if event.Type == fkevent.EventToolCallsArgsDelta {
			member.upsertToolCall(runtimeToolEventKey(event), event.ToolName, event.Content, tui.ToolStatusRunning)
			break
		}
		for _, tool := range event.ToolCalls {
			key := runtimeToolCallKey(event, tool)
			display := agenttool.FormatToolDisplay(tool.Function.Name)
			member.upsertToolCall(key, display.DisplayName, tool.Function.Arguments, tui.ToolStatusRunning)
		}
	case fkevent.EventToolResult, fkevent.EventToolResultChunk:
		member.ActiveOutput = -1
		member.ActiveReason = -1
		member.upsertToolResult(runtimeToolEventKey(event), event.ToolName, event.Content, tui.ToolStatusDone, event.Type == fkevent.EventToolResultChunk)
	case fkevent.EventError:
		msg := event.Error
		if msg == "" {
			msg = event.Content
		}
		member.Status = "error"
		if event.ToolName != "" {
			member.upsertToolResult(runtimeToolEventKey(event), event.ToolName, msg, tui.ToolStatusError, false)
		} else {
			member.markDirty()
			member.Blocks = append(member.Blocks, runtimeBlock{Kind: runtimeBlockError, Title: agent, Content: msg})
		}
	case fkevent.EventAction:
		if event.ActionType == fkevent.ActionExit {
			member.Status = "done"
		}
		if event.Content != "" {
			member.markDirty()
			member.Blocks = append(member.Blocks, runtimeBlock{Kind: runtimeBlockSystem, Title: string(event.ActionType), Content: event.Content})
		}
	}
	m.syncMemberSummary(member)
}

func (m *runtimeModel) applyAgentToolResult(event fkevent.Event) bool {
	member, _ := m.memberForToolEvent(event)
	if member == nil {
		display, ok := runtimeAgentToolDisplay(event.ToolName)
		if !ok {
			return false
		}
		aliases := runtimeAgentToolEventAliases(event)
		key := runtimeAgentToolEventKey(event)
		if mapped := m.memberKeyForAliases(aliases...); mapped != "" {
			key = mapped
		}
		if key == "" {
			return true
		}
		member = m.ensureAgentToolMember(key, display.Target, "")
		if member == nil {
			return true
		}
		m.registerMemberTool(member.Key, aliases...)
	}
	member.ActiveOutput = -1
	member.ActiveReason = -1
	if event.Type == fkevent.EventToolResultChunk {
		member.Status = "running"
	} else {
		member.Status = "done"
	}
	if event.Content != "" && len(member.Blocks) == 0 {
		member.appendOutput(member.Name, event.Content)
	}
	m.syncMemberSummary(member)
	return true
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

func emptyRuntimeToolName(name string) string {
	if name == "" {
		return runtimeDefaultToolName
	}
	return name
}

func emptyRuntimeMemberName(name string) string {
	if name == "" {
		return "member"
	}
	return name
}

func runtimeAgentToolDisplay(name string) (agenttool.ToolDisplay, bool) {
	if name == "" {
		return agenttool.ToolDisplay{}, false
	}
	display := agenttool.FormatToolDisplay(name)
	if display.Kind == agenttool.ToolKindAgent {
		return display, true
	}
	if strings.HasPrefix(name, agenttool.AgentToolPrefix) {
		target := strings.TrimPrefix(name, agenttool.AgentToolPrefix)
		return agenttool.ToolDisplay{
			Name:        name,
			DisplayName: "指派给 " + target,
			Kind:        agenttool.ToolKindAgent,
			Target:      target,
		}, true
	}
	return display, false
}

func runtimeAgentTaskFromArgs(args string) string {
	args = strings.TrimSpace(args)
	if args == "" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(args), &payload); err == nil {
		for _, key := range []string{"request", "task", "goal", "objective", "description"} {
			if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	return args
}

func runtimeAgentTaskFromCompleteArgs(args string) string {
	args = strings.TrimSpace(args)
	if args == "" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(args), &payload); err != nil {
		return ""
	}
	for _, key := range []string{"request", "task", "goal", "objective", "description"} {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func runtimeAgentToolEventAliases(event fkevent.Event) []string {
	aliases := []string{
		event.SpanID,
		event.ToolCallRef,
		event.ToolCallID,
	}
	if key := runtimeToolEventKey(event); isRuntimeStableToolAlias(key) {
		aliases = append(aliases, key)
	}
	return compactRuntimeAliases(aliases)
}

func runtimeAgentToolEventKey(event fkevent.Event) string {
	if event.SpanID != "" {
		return event.SpanID
	}
	if event.ToolCallRef != "" {
		return event.ToolCallRef
	}
	if event.ToolCallID != "" {
		return event.ToolCallID
	}
	if event.ToolCallIndex != nil && event.ParentSpanID != "" {
		return fmt.Sprintf("%s|idx:%d", event.ParentSpanID, *event.ToolCallIndex)
	}
	return ""
}

func runtimeLikelyPendingAgentToolArgs(event fkevent.Event) bool {
	if event.Type != fkevent.EventToolCallsArgsDelta {
		return false
	}
	return event.ToolCallRef != "" || event.ToolCallID != "" || event.SpanID != ""
}

func runtimeAgentToolCallAliases(event fkevent.Event, tool schema.ToolCall) []string {
	aliases := []string{
		tool.ID,
	}
	if tool.Index != nil && event.ToolCallSpanIDs != nil {
		aliases = append(aliases, event.ToolCallSpanIDs[*tool.Index])
	}
	if key := runtimeToolCallKey(event, tool); isRuntimeStableToolAlias(key) {
		aliases = append(aliases, key)
	}
	if tool.Index != nil {
		idx := *tool.Index
		if event.ToolCallRefs != nil {
			aliases = append(aliases, event.ToolCallRefs[idx])
		}
	}
	return compactRuntimeAliases(aliases)
}

func runtimeAgentToolCallKey(event fkevent.Event, tool schema.ToolCall) string {
	if tool.Index != nil && event.ToolCallSpanIDs != nil {
		if span := event.ToolCallSpanIDs[*tool.Index]; span != "" {
			return span
		}
	}
	if tool.ID != "" {
		return tool.ID
	}
	if tool.Index != nil && event.SpanID != "" {
		return fmt.Sprintf("%s|idx:%d", event.SpanID, *tool.Index)
	}
	return runtimeToolCallKey(event, tool)
}

func runtimeDirectToolEventAliases(event fkevent.Event) []string {
	return compactRuntimeAliases([]string{
		event.SpanID,
		event.ToolCallRef,
		event.ToolCallID,
	})
}

func isRuntimeStableToolAlias(alias string) bool {
	return alias != "" && !strings.HasPrefix(alias, "idx:") && !strings.HasPrefix(alias, "name:")
}

func runtimeMemberEventAliases(event fkevent.Event) []string {
	aliases := []string{
		event.ParentSpanID,
		event.SpanID,
		event.MemberCallID,
		event.ParentToolCallID,
	}
	return compactRuntimeAliases(aliases)
}

func compactRuntimeAliases(aliases []string) []string {
	seen := make(map[string]bool, len(aliases))
	result := aliases[:0]
	for _, alias := range aliases {
		if alias == "" || seen[alias] {
			continue
		}
		seen[alias] = true
		result = append(result, alias)
	}
	return result
}

func runtimeToolEventKey(event fkevent.Event) string {
	if event.SpanID != "" {
		return event.SpanID
	}
	if event.ToolCallRef != "" {
		return event.ToolCallRef
	}
	if event.ToolCallID != "" {
		return event.ToolCallID
	}
	if event.ToolCallIndex != nil {
		return fmt.Sprintf("idx:%d", *event.ToolCallIndex)
	}
	if event.ToolName != "" {
		return "name:" + event.ToolName
	}
	return ""
}

func runtimeToolCallKey(event fkevent.Event, tool schema.ToolCall) string {
	if tool.Index != nil && event.ToolCallSpanIDs != nil {
		if span := event.ToolCallSpanIDs[*tool.Index]; span != "" {
			return span
		}
	}
	if tool.Index != nil && event.ToolCallRefs != nil {
		if ref := event.ToolCallRefs[*tool.Index]; ref != "" {
			return ref
		}
	}
	if tool.ID != "" {
		return tool.ID
	}
	if tool.Index != nil {
		return fmt.Sprintf("idx:%d", *tool.Index)
	}
	return "name:" + tool.Function.Name
}

func runtimeMemberKey(event fkevent.Event) string {
	if event.IsMemberEvent {
		if event.ParentSpanID != "" {
			return event.ParentSpanID
		}
		if event.SpanID != "" {
			return event.SpanID
		}
	}
	if event.MemberCallID != "" {
		return event.MemberCallID
	}
	if event.ParentToolCallID != "" {
		return event.ParentToolCallID
	}
	return ""
}

func runtimeMemberName(event fkevent.Event) string {
	if event.MemberName != "" {
		return event.MemberName
	}
	if event.AgentName != "" {
		return event.AgentName
	}
	if event.MemberToolName != "" {
		return event.MemberToolName
	}
	return "member"
}

func truncateRuntimeText(s string, limit int) string {
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "..."
}

func runtimeHelpMarkdown() string {
	var sb strings.Builder
	sb.WriteString("常用命令按场景分组如下。\n\n")
	for _, category := range runtimeCommandCategories() {
		sb.WriteString("## " + category + "\n\n")
		for _, command := range allCommands {
			if command.Category != category {
				continue
			}
			fmt.Fprintf(&sb, "- `%s` %s\n", runtimeCommandSyntax(command), command.Desc)
		}
		sb.WriteString("\n")
	}
	sb.WriteString("## 输入\n\n")
	sb.WriteString("- `@agent` 指定智能体\n")
	sb.WriteString("- `#file` 引用文件\n")
	sb.WriteString("- `Shift+Enter` 输入换行\n")
	sb.WriteString("\n直接输入问题即可与智能体团队对话。")
	return sb.String()
}

func runtimeCommandCategories() []string {
	categories := make([]string, 0)
	seen := map[string]bool{}
	for _, command := range allCommands {
		if command.Category == "" || seen[command.Category] {
			continue
		}
		seen[command.Category] = true
		categories = append(categories, command.Category)
	}
	return categories
}

func runtimeCommandSyntax(command CommandInfo) string {
	syntax := "/" + command.Name
	if command.Usage != "" {
		syntax += " " + command.Usage
	}
	return syntax
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

func runtimeChatHistoryMarkdown(interactive bool) string {
	items, err := runtimeSessionPickerItems()
	if err != nil {
		return "读取历史目录失败: " + err.Error()
	}
	if len(items) == 0 {
		return "暂无聊天历史文件"
	}

	var sb strings.Builder
	sb.WriteString("| 会话 ID | 标题 |\n")
	sb.WriteString("|---------|------|\n")
	for _, item := range items {
		fmt.Fprintf(&sb, "| `%s` | %s |\n", markdownEscapeTable(item.Value), markdownEscapeTable(item.Label))
	}
	if interactive {
		fmt.Fprintf(&sb, "\n共 **%d** 个会话，使用 `load_chat_history` 加载。", len(items))
	} else {
		fmt.Fprintf(&sb, "\n共 **%d** 个会话，使用 `fkteams --resume <session_id>` 恢复。", len(items))
	}
	return sb.String()
}

func runtimeSessionPickerItems() ([]runtimePickerItem, error) {
	entries, err := os.ReadDir(CLIHistoryDir)
	if err != nil {
		return nil, err
	}
	items := make([]runtimePickerItem, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessionID := entry.Name()
		sessionDir := filepath.Join(CLIHistoryDir, sessionID)
		title := sessionID
		if meta, err := eventlog.LoadMetadata(sessionDir); err == nil && strings.TrimSpace(meta.Title) != "" {
			title = meta.Title
		}
		label := title
		historyFile := filepath.Join(sessionDir, eventlog.HistoryFileName)
		if info, err := os.Stat(historyFile); err == nil {
			label = fmt.Sprintf("%s (%s, %d B)", title, info.ModTime().Format("2006-01-02 15:04:05"), info.Size())
		}
		items = append(items, runtimePickerItem{Label: label, Value: sessionID})
	}
	return items, nil
}

func runtimeMemoryMarkdown() string {
	entries, err := runtimeMemoryEntries()
	if err != nil {
		return err.Error()
	}
	if len(entries) == 0 {
		return "暂无长期记忆条目"
	}
	var sb strings.Builder
	sb.WriteString("| 类型 | 摘要 | 详情 | 命中 |\n")
	sb.WriteString("|------|------|------|------|\n")
	for _, entry := range entries {
		fmt.Fprintf(&sb, "| %s | %s | %s | %d |\n",
			markdownEscapeTable(string(entry.Type)),
			markdownEscapeTable(entry.Summary),
			markdownEscapeTable(entry.Detail),
			entry.HitCount,
		)
	}
	fmt.Fprintf(&sb, "\n共 **%d** 条记忆，使用 `delete_memory` 删除条目，或 `clear_memory` 清空全部。", len(entries))
	return sb.String()
}

func runtimeMemoryEntries() ([]memory.MemoryEntry, error) {
	if g.MemoryManager == nil {
		return nil, fmt.Errorf("长期记忆未启用，请在 config.toml 中设置 [memory] enabled = true")
	}
	return g.MemoryManager.List(), nil
}

func runtimeScheduleMarkdown() string {
	tasks, err := runtimeScheduledTasks("")
	if err != nil {
		return err.Error()
	}
	if len(tasks) == 0 {
		return "暂无定时任务"
	}
	var sb strings.Builder
	sb.WriteString("| ID | 状态 | 任务 | 下次执行 |\n")
	sb.WriteString("|----|------|------|----------|\n")
	for _, task := range tasks {
		fmt.Fprintf(&sb, "| `%s` | %s | %s | %s |\n",
			markdownEscapeTable(task.ID),
			markdownEscapeTable(task.Status),
			markdownEscapeTable(truncateRuntimeText(task.Task, 80)),
			task.NextRunAt.Format("2006-01-02 15:04"),
		)
	}
	fmt.Fprintf(&sb, "\n共 **%d** 个定时任务。", len(tasks))
	return sb.String()
}

func runtimeScheduledTasks(status string) ([]scheduler.ScheduledTask, error) {
	s := scheduler.Global()
	if s == nil {
		return nil, fmt.Errorf("定时任务调度器未初始化")
	}
	return s.GetTasks(status)
}

func markdownEscapeTable(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.ReplaceAll(value, "|", "\\|")
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
	width := m.screenWidth()
	return max(20, width-runtimeHorizontalGutter*2)
}

func (m runtimeModel) screenWidth() int {
	width := m.width
	if width <= 0 {
		width = 100
	}
	return max(24, width)
}
