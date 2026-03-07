package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
)

// SelectItem 选择列表项
type SelectItem struct {
	Label string
	Value string
}

// selectModel 可过滤的选择列表
type selectModel struct {
	title   string
	items   []SelectItem
	matches []int  // 匹配过滤器的项索引
	cursor  int    // 当前选中项在 matches 中的位置
	offset  int    // 滚动偏移
	height  int    // 可见行数
	filter  string // 过滤文本

	selected string
	aborted  bool
}

func newSelectModel(title string, items []SelectItem, height int) selectModel {
	if height <= 0 {
		height = 10
	}
	m := selectModel{
		title:  title,
		items:  items,
		height: height,
	}
	m.updateMatches()
	return m
}

func (m *selectModel) updateMatches() {
	m.matches = nil
	lower := strings.ToLower(m.filter)
	for i, item := range m.items {
		if m.filter == "" || strings.Contains(strings.ToLower(item.Label), lower) {
			m.matches = append(m.matches, i)
		}
	}
	if m.cursor >= len(m.matches) {
		m.cursor = max(0, len(m.matches)-1)
	}
	m.adjustOffset()
}

func (m *selectModel) adjustOffset() {
	if len(m.matches) <= m.height {
		m.offset = 0
		return
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+m.height {
		m.offset = m.cursor - m.height + 1
	}
}

func (m selectModel) Init() tea.Cmd { return nil }

func (m selectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			if len(m.matches) > 0 {
				m.selected = m.items[m.matches[m.cursor]].Value
			}
			return m, tea.Quit
		case "esc", "ctrl+c":
			m.aborted = true
			return m, tea.Quit
		case "up":
			if m.cursor > 0 {
				m.cursor--
			} else if len(m.matches) > 0 {
				m.cursor = len(m.matches) - 1
			}
			m.adjustOffset()
			return m, nil
		case "down":
			if m.cursor < len(m.matches)-1 {
				m.cursor++
			} else {
				m.cursor = 0
			}
			m.adjustOffset()
			return m, nil
		case "backspace":
			if len(m.filter) > 0 {
				runes := []rune(m.filter)
				m.filter = string(runes[:len(runes)-1])
				m.updateMatches()
			}
			return m, nil
		default:
			if msg.Text != "" {
				m.filter += msg.Text
				m.updateMatches()
				return m, nil
			}
		}
	}
	return m, nil
}

// 样式定义
var (
	selectTitleStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	selectCursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	selectFilterStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	selectDimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

func (m selectModel) View() tea.View {
	var b strings.Builder

	b.WriteString(selectTitleStyle.Render("? "+m.title) + "\n")

	if m.filter != "" {
		b.WriteString(selectFilterStyle.Render("  / "+m.filter) + "\n")
	}

	if len(m.matches) == 0 {
		b.WriteString(selectDimStyle.Render("  (无匹配项)") + "\n")
	} else {
		end := min(m.offset+m.height, len(m.matches))
		if m.offset > 0 {
			b.WriteString(selectDimStyle.Render("  ↑ 更多...") + "\n")
		}
		for i := m.offset; i < end; i++ {
			item := m.items[m.matches[i]]
			if i == m.cursor {
				b.WriteString(selectCursorStyle.Render("  > "+item.Label) + "\n")
			} else {
				b.WriteString("    " + item.Label + "\n")
			}
		}
		if end < len(m.matches) {
			b.WriteString(selectDimStyle.Render("  ↓ 更多...") + "\n")
		}
	}

	b.WriteString(selectDimStyle.Render("  ↑↓ 移动 | Enter 选择 | Esc 返回 | 输入过滤") + "\n")
	return tea.NewView(b.String())
}

// SelectFromList 显示可过滤的选择列表，返回选中项的 Value
// height 可选，默认 10 行
func SelectFromList(title string, items []SelectItem, height ...int) (string, error) {
	h := 10
	if len(height) > 0 && height[0] > 0 {
		h = height[0]
	}
	p := tea.NewProgram(newSelectModel(title, items, h))
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	result := final.(selectModel)
	if result.aborted || result.selected == "" {
		return "", ErrInterrupted
	}
	return result.selected, nil
}
