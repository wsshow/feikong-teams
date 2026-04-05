package tui

import (
	"fmt"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

// promptModel 通用单行输入模型（支持密码掩码、默认值）
type promptModel struct {
	textInput textinput.Model
	prompt    string
	value     string
	aborted   bool
}

func newPromptModel(prompt string, defaultValue string, echoMode textinput.EchoMode) promptModel {
	ti := textinput.New()
	ti.Prompt = "  > "
	s := ti.Styles()
	s.Focused.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	ti.SetStyles(s)
	ti.EchoMode = echoMode
	if echoMode == textinput.EchoPassword {
		ti.EchoCharacter = '*'
		ti.Placeholder = "输入后按 Enter 确认"
	}
	ti.SetWidth(60)
	if defaultValue != "" {
		ti.SetValue(defaultValue)
		ti.CursorEnd()
	}
	ti.Focus()

	return promptModel{
		textInput: ti,
		prompt:    prompt,
	}
}

func (m promptModel) Init() tea.Cmd { return textinput.Blink }

func (m promptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		w := msg.Width - 6
		if w < 20 {
			w = 20
		}
		m.textInput.SetWidth(w)
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			m.value = m.textInput.Value()
			return m, tea.Quit
		case "ctrl+c", "esc":
			m.aborted = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m promptModel) View() tea.View {
	if m.value != "" || m.aborted {
		return tea.NewView("")
	}
	label := selectTitleStyle.Render("? " + m.prompt)
	return tea.NewView(fmt.Sprintf("%s\n%s\n", label, m.textInput.View()))
}

// ReadSecret 交互式密码输入（掩码显示）
func ReadSecret(prompt string) (string, error) {
	p := tea.NewProgram(newPromptModel(prompt, "", textinput.EchoPassword))
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	m := final.(promptModel)
	if m.aborted {
		return "", ErrInterrupted
	}
	return m.value, nil
}

// ReadInput 交互式文本输入（可带默认值）
func ReadInput(prompt string, defaultValue string) (string, error) {
	p := tea.NewProgram(newPromptModel(prompt, defaultValue, textinput.EchoNormal))
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	m := final.(promptModel)
	if m.aborted {
		return "", ErrInterrupted
	}
	return m.value, nil
}
