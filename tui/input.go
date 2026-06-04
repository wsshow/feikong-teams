package tui

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
)

// ErrInterrupted 用户中断输入
var ErrInterrupted = errors.New("user interrupted")

// ErrCtrlC 用户按下 Ctrl+C
var ErrCtrlC = errors.New("user pressed ctrl+c")

// triggerChars 在空输入时立即触发选择
var triggerChars = map[string]bool{"@": true, "/": true}

const (
	inputExitConfirmWindow = 2 * time.Second
	inputExitConfirmTick   = time.Second
)

type inputExitTickMsg time.Time

type ReadLineOpts struct {
	History      []string
	InitialValue string
}

type inputModel struct {
	textInput    textinput.Model
	text         string
	trigger      string
	aborted      bool
	ctrlC        bool
	history      []string
	historyIndex int
	savedInput   string
	pastes       []string // 多行粘贴内容，顺序与文本中占位符一一对应
	exitUntil    time.Time
}

func newInputModel(prompt string, opts *ReadLineOpts) inputModel {
	ti := textinput.New()
	ti.Prompt = prompt
	s := ti.Styles()
	s.Focused.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	ti.SetStyles(s)
	ti.Placeholder = "@ agent | # file | / cmd"
	ti.SetWidth(80)
	ti.Focus()

	var history []string
	if opts != nil {
		history = opts.History
		if opts.InitialValue != "" {
			ti.SetValue(opts.InitialValue)
			ti.CursorEnd()
		}
	}

	return inputModel{
		textInput:    ti,
		history:      history,
		historyIndex: len(history),
	}
}

func (m inputModel) Init() tea.Cmd { return textinput.Blink }

func (m inputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.textInput.SetWidth(msg.Width - 4)
		return m, nil
	case inputExitTickMsg:
		if m.exitUntil.IsZero() {
			return m, nil
		}
		if time.Now().After(m.exitUntil) {
			m.exitUntil = time.Time{}
			return m, nil
		}
		return m, inputExitTickCmd()
	case tea.PasteMsg:
		content := msg.Content
		if strings.ContainsAny(content, "\n\r") {
			m.exitUntil = time.Time{}
			return m.insertPaste(strings.TrimRight(content, "\n\r")), nil
		}
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			m.text = ExpandInlineInput(strings.TrimSpace(m.textInput.Value()), m.pastes)
			return m, tea.Quit
		case "ctrl+c":
			if m.isExitConfirming() {
				m.ctrlC = true
				return m, tea.Quit
			}
			m.exitUntil = time.Now().Add(inputExitConfirmWindow)
			return m, inputExitTickCmd()
		case "esc":
			m.aborted = true
			return m, tea.Quit
		case "ctrl+v":
			if text, err := clipboard.ReadAll(); err == nil && strings.ContainsAny(text, "\n\r") {
				m.exitUntil = time.Time{}
				return m.insertPaste(strings.TrimRight(text, "\n\r")), nil
			}
		case "backspace":
			m.exitUntil = time.Time{}
			if newM, ok := m.backspaceTag(); ok {
				return newM, nil
			}
		case "up":
			m.exitUntil = time.Time{}
			if len(m.history) > 0 && m.historyIndex > 0 {
				if m.historyIndex == len(m.history) {
					m.savedInput = m.textInput.Value()
				}
				m.historyIndex--
				m.textInput.SetValue(m.history[m.historyIndex])
				m.textInput.CursorEnd()
			}
			return m, nil
		case "down":
			m.exitUntil = time.Time{}
			if len(m.history) > 0 && m.historyIndex < len(m.history) {
				m.historyIndex++
				if m.historyIndex == len(m.history) {
					m.textInput.SetValue(m.savedInput)
				} else {
					m.textInput.SetValue(m.history[m.historyIndex])
				}
				m.textInput.CursorEnd()
			}
			return m, nil
		}
		if msg.Text != "" {
			m.exitUntil = time.Time{}
			if msg.Text == "#" {
				m.text = ExpandInlineInput(m.textInput.Value(), m.pastes)
				m.trigger = "#"
				return m, tea.Quit
			}
			if m.textInput.Value() == "" && triggerChars[msg.Text] {
				m.trigger = msg.Text
				return m, tea.Quit
			}
		}
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func inputExitTickCmd() tea.Cmd {
	return tea.Tick(inputExitConfirmTick, func(t time.Time) tea.Msg {
		return inputExitTickMsg(t)
	})
}

func (m inputModel) isExitConfirming() bool {
	return !m.exitUntil.IsZero() && time.Now().Before(m.exitUntil)
}

// insertPaste 在当前光标位置插入多行粘贴内容的占位符，维护 pastes 顺序。
func (m inputModel) insertPaste(content string) inputModel {
	value, cursor, pastes := InsertInlinePaste(m.textInput.Value(), m.textInput.Position(), m.pastes, content)
	m.pastes = pastes
	m.textInput.SetValue(value)
	m.textInput.SetCursor(cursor)
	return m
}

// backspaceTag 若光标紧跟在占位符末尾，整体删除该占位符及对应 pastes 条目。
func (m inputModel) backspaceTag() (inputModel, bool) {
	value, cursor, pastes, ok := DeleteInlinePasteBeforeCursor(m.textInput.Value(), m.textInput.Position(), m.pastes)
	if !ok {
		return m, false
	}
	m.pastes = pastes
	m.textInput.SetValue(value)
	m.textInput.SetCursor(cursor)
	return m, true
}

func (m inputModel) View() tea.View {
	// 提交或取消后返回空视图，清除屏幕上的输入行
	if m.text != "" || m.trigger != "" || m.aborted || m.ctrlC {
		return tea.NewView("")
	}

	viewStr := m.textInput.View()
	if len(m.pastes) == 0 {
		return tea.NewView(m.renderExitConfirm(viewStr))
	}
	viewStr = RenderInlineInputValue(viewStr)
	return tea.NewView(m.renderExitConfirm(viewStr))
}

func (m inputModel) renderExitConfirm(viewStr string) string {
	if !m.isExitConfirming() {
		return viewStr
	}
	seconds := int(math.Ceil(time.Until(m.exitUntil).Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	countdownStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	return fmt.Sprintf(
		"%s\n%s",
		viewStr,
		dimStyle.Render("再按 ")+keyStyle.Render("Ctrl+C")+dimStyle.Render(" 退出 · ")+countdownStyle.Render(fmt.Sprintf("%ds", seconds)),
	)
}

// ReadLine 读取一行输入，检测触发字符（@/#/）。
// # 在任意位置触发文件选择（text 返回已输入内容）；@ / 仅在空输入时触发。
func ReadLine(prompt string, opts ...*ReadLineOpts) (text string, trigger string, err error) {
	var o *ReadLineOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	p := tea.NewProgram(newInputModel(prompt, o))
	final, runErr := p.Run()
	if runErr != nil {
		return "", "", runErr
	}
	m := final.(inputModel)
	if m.ctrlC {
		return "", "", ErrCtrlC
	}
	if m.aborted {
		return "", "", ErrInterrupted
	}
	return m.text, m.trigger, nil
}
