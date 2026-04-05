package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

// AskOption 提问选项
type AskOption struct {
	Label string
	Value string
}

// AskResult 提问结果
type AskResult struct {
	Selected []string
	FreeText string
}

// askModel bubbletea 模型：展示问题 + 选项列表 + 自由输入
type askModel struct {
	question    string
	options     []AskOption
	multiSelect bool

	cursor  int          // 当前光标位置
	checked map[int]bool // 多选时已选中的项

	// 输入模式：选项选择 or 自由输入
	inputMode bool
	textInput textinput.Model

	result  AskResult
	done    bool
	aborted bool
}

const customInputLabel = "[输入] 自行输入..."

func newAskModel(question string, options []AskOption, multiSelect bool) askModel {
	ti := textinput.New()
	ti.Prompt = "  > "
	s := ti.Styles()
	s.Focused.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	ti.SetStyles(s)
	ti.Placeholder = "输入你的回答..."
	ti.SetWidth(60)

	return askModel{
		question:    question,
		options:     options,
		multiSelect: multiSelect,
		checked:     make(map[int]bool),
		textInput:   ti,
	}
}

func (m askModel) Init() tea.Cmd { return nil }

// totalItems 包括选项 + "自行输入"
func (m askModel) totalItems() int {
	return len(m.options) + 1 // +1 for custom input
}

func (m askModel) isCustomIndex() bool {
	return m.cursor == len(m.options)
}

func (m askModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// 自由输入模式
	if m.inputMode {
		switch msg := msg.(type) {
		case tea.KeyPressMsg:
			switch msg.String() {
			case "enter":
				m.result.FreeText = strings.TrimSpace(m.textInput.Value())
				// 收集多选的已选项
				if m.multiSelect {
					for i, opt := range m.options {
						if m.checked[i] {
							m.result.Selected = append(m.result.Selected, opt.Value)
						}
					}
				}
				m.done = true
				return m, tea.Quit
			case "esc":
				// 返回选项模式
				m.inputMode = false
				m.textInput.Blur()
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}

	// 选项选择模式
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			} else {
				m.cursor = m.totalItems() - 1
			}
			return m, nil
		case "down", "j":
			if m.cursor < m.totalItems()-1 {
				m.cursor++
			} else {
				m.cursor = 0
			}
			return m, nil
		case " ":
			if m.multiSelect && !m.isCustomIndex() {
				m.checked[m.cursor] = !m.checked[m.cursor]
			}
			return m, nil
		case "enter":
			if m.isCustomIndex() {
				// 进入自由输入模式
				m.inputMode = true
				m.textInput.Focus()
				return m, textinput.Blink
			}
			if m.multiSelect {
				// 多选模式：切换当前项 或 提交所有已选
				if !m.checked[m.cursor] {
					m.checked[m.cursor] = true
					return m, nil
				}
				// 收集所有已选
				for i, opt := range m.options {
					if m.checked[i] {
						m.result.Selected = append(m.result.Selected, opt.Value)
					}
				}
				m.done = true
				return m, tea.Quit
			}
			// 单选：直接提交
			m.result.Selected = []string{m.options[m.cursor].Value}
			m.done = true
			return m, tea.Quit
		case "ctrl+d":
			// 多选模式：提交已选项
			if m.multiSelect {
				for i, opt := range m.options {
					if m.checked[i] {
						m.result.Selected = append(m.result.Selected, opt.Value)
					}
				}
				m.done = true
				return m, tea.Quit
			}
		case "esc", "ctrl+c":
			m.aborted = true
			return m, tea.Quit
		}
	}
	return m, nil
}

// 样式
var (
	askQuestionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("12")).
				Bold(true).
				PaddingLeft(1)
	askOptionStyle = lipgloss.NewStyle().
			PaddingLeft(4)
	askCursorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("6")).
			PaddingLeft(2)
	askCheckedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10"))
	askCustomStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("3")).
			Italic(true)
	askHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			PaddingLeft(2)
)

func (m askModel) View() tea.View {
	if m.done || m.aborted {
		return tea.NewView("")
	}

	var b strings.Builder

	// 问题
	b.WriteString("\n")
	b.WriteString(askQuestionStyle.Render("[提问] " + m.question))
	b.WriteString("\n\n")

	if m.inputMode {
		// 自由输入模式
		if len(m.options) > 0 && m.multiSelect {
			// 显示已选项摘要
			var sel []string
			for i, opt := range m.options {
				if m.checked[i] {
					sel = append(sel, opt.Label)
				}
			}
			if len(sel) > 0 {
				b.WriteString(askHintStyle.Render("已选: "+strings.Join(sel, ", ")) + "\n\n")
			}
		}
		b.WriteString(m.textInput.View())
		b.WriteString("\n\n")
		b.WriteString(askHintStyle.Render("Enter 提交 | Esc 返回选项"))
		b.WriteString("\n")
		return tea.NewView(b.String())
	}

	// 选项列表
	total := m.totalItems()
	for i := 0; i < total; i++ {
		isCursor := i == m.cursor
		isCustom := i == len(m.options)

		var prefix, label string

		if isCustom {
			// 自行输入选项
			if m.multiSelect {
				prefix = "[ ] "
			} else {
				prefix = "○ "
			}
			label = customInputLabel
		} else {
			opt := m.options[i]
			if m.multiSelect {
				if m.checked[i] {
					prefix = askCheckedStyle.Render("[✓] ")
				} else {
					prefix = "[ ] "
				}
			} else {
				if isCursor {
					prefix = "● "
				} else {
					prefix = "○ "
				}
			}
			label = opt.Label
		}

		line := prefix + label

		if isCursor {
			if isCustom {
				b.WriteString(askCursorStyle.Render("> " + askCustomStyle.Render(line)))
			} else {
				b.WriteString(askCursorStyle.Render("> " + line))
			}
		} else {
			if isCustom {
				b.WriteString(askOptionStyle.Render(askCustomStyle.Render(line)))
			} else {
				b.WriteString(askOptionStyle.Render(line))
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	if m.multiSelect {
		b.WriteString(askHintStyle.Render("↑↓ 移动 | Space 选择 | Enter 切换/确认 | Ctrl+D 提交 | Esc 取消"))
	} else {
		b.WriteString(askHintStyle.Render("↑↓ 移动 | Enter 选择 | Esc 取消"))
	}
	b.WriteString("\n")

	return tea.NewView(b.String())
}

// AskQuestions 展示问题和选项，收集用户回答。
// 无选项时直接进入自由输入模式。
func AskQuestions(question string, options []AskOption, multiSelect bool) (*AskResult, error) {
	if len(options) == 0 {
		// 无选项，直接自由输入
		return askFreeText(question)
	}

	p := tea.NewProgram(newAskModel(question, options, multiSelect))
	final, err := p.Run()
	if err != nil {
		return nil, err
	}
	m := final.(askModel)
	if m.aborted {
		return &AskResult{}, nil
	}
	return &m.result, nil
}

// askFreeText 仅自由输入模式
func askFreeText(question string) (*AskResult, error) {
	ti := textinput.New()
	ti.Prompt = "  > "
	s := ti.Styles()
	s.Focused.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	ti.SetStyles(s)
	ti.Placeholder = "输入你的回答..."
	ti.SetWidth(60)
	ti.Focus()

	m := freeTextModel{question: question, textInput: ti}
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return nil, err
	}
	result := final.(freeTextModel)
	if result.aborted {
		return &AskResult{}, nil
	}
	return &AskResult{FreeText: strings.TrimSpace(result.text)}, nil
}

// freeTextModel 纯自由输入模型
type freeTextModel struct {
	question  string
	textInput textinput.Model
	text      string
	aborted   bool
}

func (m freeTextModel) Init() tea.Cmd { return textinput.Blink }

func (m freeTextModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			m.text = m.textInput.Value()
			return m, tea.Quit
		case "esc", "ctrl+c":
			m.aborted = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m freeTextModel) View() tea.View {
	if m.text != "" || m.aborted {
		return tea.NewView("")
	}
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(askQuestionStyle.Render("[提问] " + m.question))
	b.WriteString("\n\n")
	b.WriteString(m.textInput.View())
	b.WriteString("\n\n")
	b.WriteString(askHintStyle.Render("Enter 提交 | Esc 取消"))
	b.WriteString("\n")
	return tea.NewView(b.String())
}
