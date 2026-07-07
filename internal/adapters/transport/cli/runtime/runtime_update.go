package runtime

import (
	"context"

	"errors"

	"fkteams/internal/adapters/transport/cli/tui"

	"fmt"

	"strings"

	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/atotto/clipboard"
)

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
		m.status = "执行中..."
		m.activeOutput = -1
		m.activeReason = -1
		return m, nil
	case runtimeAgentEventMsg:
		m.applyEvent(msg.event)
		return m, nil
	case runtimeAskPendingMsg:
		m.applyAskPending(msg.ask)
		return m, nil
	case runtimeAskAnsweredMsg:
		m.applyAskAnswered(msg.askID, msg.selected, msg.freeText)
		return m, nil
	case runtimeAskCancelledMsg:
		m.applyAskCancelled(msg.askID)
		return m, nil
	case runtimeApprovalPendingMsg:
		m.applyApprovalPending(msg.approval)
		return m, nil
	case runtimeApprovalAnsweredMsg:
		m.applyApprovalAnswered(msg.id, msg.decision)
		return m, nil
	case runtimeApprovalCancelledMsg:
		m.applyApprovalCancelled(msg.id)
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
		if m.memberView != "" && m.currentMemberPendingAsk() == nil {
			return m, nil
		}
		content := msg.Content
		if strings.ContainsAny(content, "\n\r") {
			m.exitUntil = time.Time{}
			return m.insertPaste(strings.TrimRight(content, "\n\r")), nil
		}
	case tea.KeyPressMsg:
		if m.approval != nil {
			return m.updateApproval(msg)
		}
		if m.memberView != "" {
			switch msg.String() {
			case "esc", "left":
				m.memberView = ""
				return m, nil
			case "backspace":
				if m.currentMemberPendingAsk() == nil || m.input.Value() == "" {
					m.memberView = ""
					return m, nil
				}
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
			case "enter":
				if m.currentMemberPendingAsk() != nil {
					return m.submitCurrentMemberAsk()
				}
				return m, nil
			}
			if msg.String() != "ctrl+c" && m.currentMemberPendingAsk() == nil {
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
				m.restoreSteeringQueueToInput()
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
				input := strings.TrimSpace(m.expandInput())
				m.input.SetValue("")
				m.pastes = nil
				m.savedInput = ""
				m.historyIndex = len(m.runtime.session.InputHistory)
				m.exitUntil = time.Time{}
				if input == "" {
					return m, nil
				}
				m.runtime.session.InputHistory = append(m.runtime.session.InputHistory, input)
				if m.runtime.queueSteering(input) {
					m.status = "已排队转向，等待下一次模型调用..."
				} else {
					m.appendBlock(runtimeBlockError, "转向失败", "当前没有正在运行的任务")
				}
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
					m.picker = newAgentPicker(m.runtime.ctx)
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

func (m *runtimeModel) restoreSteeringQueueToInput() {
	queued := m.runtime.drainSteeringText()
	if queued == "" {
		return
	}
	current := strings.TrimSpace(m.expandInput())
	next := queued
	if current != "" {
		next = queued + "\n\n" + current
	}
	m.input.SetValue(strings.ReplaceAll(next, "\n", tui.InlineLineBreakTag))
	m.pastes = nil
	m.savedInput = ""
	m.input.CursorEnd()
	m.appendBlock(runtimeBlockSystem, "转向", "未执行的转向消息已回到输入框")
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
