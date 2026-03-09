package tui

import (
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

type textareaModel struct {
	textarea textarea.Model
	text     string
	aborted  bool
}

func newTextareaModel(initialValue string) textareaModel {
	ta := textarea.New()
	ta.Placeholder = "输入内容... (Ctrl+D 提交 | Esc 取消)"
	ta.SetWidth(80)
	ta.SetHeight(15)

	s := ta.Styles()
	s.Focused.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	ta.SetStyles(s)

	ta.Focus()
	if initialValue != "" {
		ta.SetValue(initialValue)
	}

	return textareaModel{textarea: ta}
}

func (m textareaModel) Init() tea.Cmd {
	return textarea.Blink
}

func (m textareaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			m.aborted = true
			return m, tea.Quit
		case "ctrl+d":
			m.text = m.textarea.Value()
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

func (m textareaModel) View() tea.View {
	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(
		"  Ctrl+D 提交 | Esc 取消",
	)
	v := tea.NewView(m.textarea.View() + "\n" + hint)
	v.AltScreen = true
	return v
}

// ReadMultiLine 打开全屏多行编辑器，返回用户输入的完整文本。
func ReadMultiLine(initialValue string) (string, error) {
	p := tea.NewProgram(newTextareaModel(initialValue))
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	m := final.(textareaModel)
	if m.aborted {
		return "", ErrInterrupted
	}
	return m.text, nil
}
