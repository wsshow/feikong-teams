package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fkteams/agents"
	"fkteams/internal/app/appstate"

	"fkteams/internal/adapters/storage/file/history"
	appschedule "fkteams/internal/app/schedule"
	domainschedule "fkteams/internal/domain/schedule"
	"fkteams/memory"

	tea "charm.land/bubbletea/v2"
)

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

func newMemoryDeletePicker(manager appstate.MemoryManager) (*runtimePicker, error) {
	entries, err := runtimeMemoryEntriesFrom(manager)
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

func runtimeMemoryEntries() ([]memory.MemoryEntry, error) {
	return runtimeMemoryEntriesFrom(nil)
}

func runtimeMemoryEntriesFrom(manager appstate.MemoryManager) ([]memory.MemoryEntry, error) {
	if manager == nil {
		return nil, fmt.Errorf("长期记忆未启用，请在 config.toml 中设置 [memory] enabled = true")
	}
	return manager.List(), nil
}

func runtimeScheduledTasks(status string) ([]domainschedule.Task, error) {
	service := appschedule.Default()
	if service == nil {
		return nil, fmt.Errorf("定时任务调度器未初始化")
	}
	return service.ListTasks(context.Background(), domainschedule.Status(status))
}
