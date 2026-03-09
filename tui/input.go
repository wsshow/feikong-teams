package tui

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
)

// ErrInterrupted 用户中断输入
var ErrInterrupted = errors.New("user interrupted")

// triggerChars 在空输入时立即触发选择
var triggerChars = map[string]bool{"@": true, "/": true}

// pasteTagRe / pasteTagSuffixRe 匹配内联粘贴占位符
var pasteTagRe = regexp.MustCompile(`\[粘贴\d+行内容\]`)
var pasteTagSuffixRe = regexp.MustCompile(`\s?\[粘贴\d+行内容\]\s?$`)

type ReadLineOpts struct {
	History      []string
	InitialValue string
}

type inputModel struct {
	textInput    textinput.Model
	text         string
	trigger      string
	aborted      bool
	history      []string
	historyIndex int
	savedInput   string
	pastes       []string // 多行粘贴内容，顺序与文本中占位符一一对应
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
	case tea.PasteMsg:
		content := msg.Content
		if strings.ContainsAny(content, "\n\r") {
			return m.insertPaste(strings.TrimRight(content, "\n\r")), nil
		}
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			m.text = expandPastes(strings.TrimSpace(m.textInput.Value()), m.pastes)
			return m, tea.Quit
		case "ctrl+c", "esc":
			m.aborted = true
			return m, tea.Quit
		case "ctrl+v":
			if text, err := clipboard.ReadAll(); err == nil && strings.ContainsAny(text, "\n\r") {
				return m.insertPaste(strings.TrimRight(text, "\n\r")), nil
			}
		case "backspace":
			if newM, ok := m.backspaceTag(); ok {
				return newM, nil
			}
		case "up":
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
			if msg.Text == "#" {
				m.text = expandPastes(m.textInput.Value(), m.pastes)
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

// insertPaste 在当前光标位置插入多行粘贴内容的占位符，维护 pastes 顺序。
func (m inputModel) insertPaste(content string) inputModel {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	lines := strings.Split(content, "\n")
	placeholder := fmt.Sprintf("[粘贴%d行内容]", max(len(lines), 2))

	pos := m.textInput.Position()
	runes := []rune(m.textInput.Value())
	before := runes[:pos]
	after := runes[pos:]

	pastesBefore := len(pasteTagRe.FindAllString(string(before), -1))
	newPastes := make([]string, len(m.pastes)+1)
	copy(newPastes[:pastesBefore], m.pastes[:pastesBefore])
	newPastes[pastesBefore] = content
	copy(newPastes[pastesBefore+1:], m.pastes[pastesBefore:])
	m.pastes = newPastes

	padded := " " + placeholder + " "
	if pos == 0 {
		padded = placeholder + " "
	}
	pRunes := []rune(padded)
	newRunes := make([]rune, 0, len(runes)+len(pRunes))
	newRunes = append(newRunes, before...)
	newRunes = append(newRunes, pRunes...)
	newRunes = append(newRunes, after...)
	m.textInput.SetValue(string(newRunes))
	m.textInput.SetCursor(pos + len(pRunes))
	return m
}

// backspaceTag 若光标紧跟在占位符末尾，整体删除该占位符及对应 pastes 条目。
func (m inputModel) backspaceTag() (inputModel, bool) {
	pos := m.textInput.Position()
	val := m.textInput.Value()
	before := string([]rune(val)[:pos])
	loc := pasteTagSuffixRe.FindStringIndex(before)
	if loc == nil {
		return m, false
	}
	pasteIdx := len(pasteTagRe.FindAllString(before[:loc[0]], -1))
	after := string([]rune(val)[pos:])
	m.textInput.SetValue(before[:loc[0]] + after)
	m.textInput.SetCursor(len([]rune(before[:loc[0]])))
	if pasteIdx < len(m.pastes) {
		m.pastes = append(m.pastes[:pasteIdx], m.pastes[pasteIdx+1:]...)
	}
	return m, true
}

// expandPastes 将文本中的粘贴占位符按序替换为实际内容。
func expandPastes(text string, pastes []string) string {
	if len(pastes) == 0 {
		return text
	}
	idx := 0
	return pasteTagRe.ReplaceAllStringFunc(text, func(match string) string {
		if idx >= len(pastes) {
			return match
		}
		content := pastes[idx]
		idx++
		return content
	})
}

func (m inputModel) View() tea.View {
	// 提交或取消后返回空视图，清除屏幕上的输入行
	if m.text != "" || m.trigger != "" || m.aborted {
		return tea.NewView("")
	}

	viewStr := m.textInput.View()
	if len(m.pastes) == 0 {
		return tea.NewView(viewStr)
	}
	tagStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("178")).Bold(true)
	return tea.NewView(pasteTagRe.ReplaceAllStringFunc(viewStr, func(match string) string {
		return tagStyle.Render(match)
	}))
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
	if m.aborted {
		return "", "", ErrInterrupted
	}
	return m.text, m.trigger, nil
}
