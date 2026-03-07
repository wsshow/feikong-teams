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

// triggerChars 触发立即选择的字符（仅在空输入时触发）
var triggerChars = map[string]bool{"@": true, "/": true}

// ReadLineOpts ReadLine 选项
type ReadLineOpts struct {
	History      []string // 输入历史
	InitialValue string   // 初始值
}

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

func newInputModel(prompt string, opts *ReadLineOpts) inputModel {
	ti := textinput.New()
	ti.Prompt = prompt
	ti.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	ti.Placeholder = "@ agent | # file | / cmd"
	ti.Width = 80
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
			if len(msg.Runes) == 1 {
				ch := string(msg.Runes[0])
				// # 在任意位置触发文件选择
				if ch == "#" {
					m.text = m.textInput.Value()
					m.trigger = "#"
					return m, tea.Quit
				}
				// @、/ 仅在空输入时触发
				if m.textInput.Value() == "" && triggerChars[ch] {
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
// # 在任意输入位置都可触发文件选择，触发时 text 返回已输入的内容
// @ 和 / 仅在空输入时触发
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
	if m.aborted {
		return "", "", ErrInterrupted
	}
	return m.text, m.trigger, nil
}
