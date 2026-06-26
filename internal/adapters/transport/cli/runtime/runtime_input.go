package runtime

import (
	"context"

	"fkteams/internal/adapters/storage/file/history"

	"fkteams/internal/adapters/transport/cli/tui"
	appchat "fkteams/internal/app/chat"

	"fmt"
	"os"
	"path/filepath"

	"strings"

	tea "charm.land/bubbletea/v2"
)

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
			m.appendBlock(runtimeBlockSystem, "定时任务", runtimeScheduleMarkdown(m.runtime.session.scheduler))
			return m, nil
		case "cancel_schedule":
			if args != "" {
				return m.cancelRuntimeSchedule(args), nil
			}
			picker, err := newScheduleCancelPicker(m.runtime.session.scheduler)
			return m.openRuntimePicker(picker, err, "取消定时任务")
		case "delete_schedule":
			if args != "" {
				return m.deleteRuntimeSchedule(args), nil
			}
			picker, err := newScheduleDeletePicker(m.runtime.session.scheduler)
			return m.openRuntimePicker(picker, err, "删除定时任务")
		case "list_memory":
			m.appendBlock(runtimeBlockSystem, "长期记忆", runtimeMemoryMarkdown(m.runtime.session.memory))
			return m, nil
		case "delete_memory":
			if args != "" {
				return m.deleteRuntimeMemory(args), nil
			}
			picker, err := newMemoryDeletePicker(m.runtime.session.memory)
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
	recorder := m.runtime.session.recorder()
	historyFile := filepath.Join(m.runtime.session.historyDir, m.runtime.session.sessionID(), eventlog.HistoryFileName)
	store := eventlog.NewChatSessionStore(m.runtime.session.historyDir)
	if err := appchat.NewSessionLifecycle(store, store).SaveActive(context.Background(), m.runtime.session.sessionID(), m.runtime.session.sessionTitle, recorder); err != nil {
		m.appendBlock(runtimeBlockError, "保存聊天历史失败", err.Error())
		return m
	}
	m.appendBlock(runtimeBlockSystem, "聊天历史", "已保存到: "+historyFile)
	return m
}

func (m runtimeModel) saveRuntimeChatHistoryMarkdown() runtimeModel {
	recorder := m.runtime.session.recorder()
	filePath, err := recorder.SaveToMarkdownWithTimestamp()
	if err != nil {
		m.appendBlock(runtimeBlockError, "导出 Markdown 失败", err.Error())
		return m
	}
	m.appendBlock(runtimeBlockSystem, "聊天历史", "已导出 Markdown: "+filePath)
	return m
}

func (m runtimeModel) saveRuntimeChatHistoryHTML() runtimeModel {
	htmlFilePath, err := m.runtime.session.SaveChatHistoryToHTML()
	if err != nil {
		m.appendBlock(runtimeBlockError, "导出 HTML 失败", err.Error())
		return m
	}
	m.appendBlock(runtimeBlockSystem, "聊天历史", "已导出 HTML: "+htmlFilePath)
	return m
}

func (m runtimeModel) clearRuntimeChatHistory() runtimeModel {
	m.runtime.session.recorder().Clear()
	m.appendBlock(runtimeBlockSystem, "聊天历史", "已清空当前聊天历史")
	return m
}

func (m runtimeModel) loadRuntimeSession(sessionID string) runtimeModel {
	historyFile := filepath.Join(m.runtime.session.historyDir, sessionID, eventlog.HistoryFileName)
	if _, err := os.Stat(historyFile); os.IsNotExist(err) {
		m.appendBlock(runtimeBlockError, "加载聊天历史失败", "历史文件不存在: "+historyFile)
		return m
	}

	m.runtime.session.activeSessionID = sessionID
	recorder := m.runtime.session.recorder()
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
	manager := m.runtime.session.memory
	if manager == nil {
		m.appendBlock(runtimeBlockError, "删除长期记忆失败", "长期记忆未启用，请在 config.toml 中设置 [memory] enabled = true")
		return m
	}
	deleted := manager.Delete(summary)
	if deleted == 0 {
		m.appendBlock(runtimeBlockSystem, "长期记忆", "未找到匹配的记忆条目: "+summary)
		return m
	}
	m.appendBlock(runtimeBlockSystem, "长期记忆", fmt.Sprintf("已删除 %d 条记忆: %s", deleted, summary))
	return m
}

func (m runtimeModel) clearRuntimeMemory() runtimeModel {
	manager := m.runtime.session.memory
	if manager == nil {
		m.appendBlock(runtimeBlockError, "清空长期记忆失败", "长期记忆未启用，请在 config.toml 中设置 [memory] enabled = true")
		return m
	}
	count := manager.Count()
	if count == 0 {
		m.appendBlock(runtimeBlockSystem, "长期记忆", "当前没有记忆条目")
		return m
	}
	manager.Clear()
	m.appendBlock(runtimeBlockSystem, "长期记忆", fmt.Sprintf("已清空 %d 条长期记忆", count))
	return m
}

func (m runtimeModel) cancelRuntimeSchedule(taskID string) runtimeModel {
	service := m.runtime.session.scheduler
	if service == nil {
		m.appendBlock(runtimeBlockError, "取消定时任务失败", "定时任务调度器未初始化")
		return m
	}
	if err := service.CancelTask(context.Background(), taskID); err != nil {
		m.appendBlock(runtimeBlockError, "取消定时任务失败", err.Error())
		return m
	}
	m.appendBlock(runtimeBlockSystem, "定时任务", "已取消: "+taskID)
	return m
}

func (m runtimeModel) deleteRuntimeSchedule(taskID string) runtimeModel {
	service := m.runtime.session.scheduler
	if service == nil {
		m.appendBlock(runtimeBlockError, "删除定时任务失败", "定时任务调度器未初始化")
		return m
	}
	if err := service.DeleteTask(context.Background(), taskID); err != nil {
		m.appendBlock(runtimeBlockError, "删除定时任务失败", err.Error())
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
