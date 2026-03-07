package tui

import (
	"errors"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ErrInterrupted 用户中断输入（Ctrl+C / Esc）
var ErrInterrupted = errors.New("user interrupted")

// triggerChars 触发立即选择的字符
var triggerChars = map[string]bool{"@": true, "#": true, "/": true}

// inputModel bubbletea 模型：行输入 + 触发字符检测 + 历史导航
type inputModel struct {
	textInput    textinput.Model
	text         string
	trigger      string
	aborted      bool
	history      []string // 输入历史
	historyIndex int      // 当前浏览的历史索引，len(history) 表示新输入
	savedInput   string   // 按上键前暂存的当前输入
}

func newInputModel(prompt string, history []string) inputModel {
	ti := textinput.New()
	ti.Prompt = prompt
	ti.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	ti.Placeholder = "@ agent | # file | / cmd"
	ti.Width = 80
	ti.Focus()
	return inputModel{
		textInput:    ti,
		history:      history,
		historyIndex: len(history),
	}
}

func (m inputModel) Init() tea.Cmd { return textinput.Blink }

func (m inputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			m.text = strings.TrimSpace(m.textInput.Value())
			return m, tea.Quit
		case tea.KeyCtrlC:
			m.aborted = true
			return m, tea.Quit
		case tea.KeyUp:
			if len(m.history) > 0 && m.historyIndex > 0 {
				// 首次按上键时保存当前输入
				if m.historyIndex == len(m.history) {
					m.savedInput = m.textInput.Value()
				}
				m.historyIndex--
				m.textInput.SetValue(m.history[m.historyIndex])
				m.textInput.CursorEnd()
			}
			return m, nil
		case tea.KeyDown:
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
		case tea.KeyRunes:
			// 空输入时检测触发字符，立即响应
			if m.textInput.Value() == "" && len(msg.Runes) == 1 {
				ch := string(msg.Runes[0])
				if triggerChars[ch] {
					m.trigger = ch
					return m, tea.Quit
				}
			}
		}
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m inputModel) View() string { return m.textInput.View() }

// ReadLine 读取一行输入，同时检测触发字符（@/#/）
// trigger 为触发字符，空串表示普通输入
// 可选传入历史记录列表，支持上下键导航
func ReadLine(prompt string, history ...[]string) (text string, trigger string, err error) {
	var hist []string
	if len(history) > 0 {
		hist = history[0]
	}
	p := tea.NewProgram(newInputModel(prompt, hist))
	final, runErr := p.Run()
	if runErr != nil {
		return "", "", runErr
	}
	m := final.(inputModel)
	if m.aborted {
		return "", "", ErrInterrupted
	}
	return m.text, m.trigger, nil
}
