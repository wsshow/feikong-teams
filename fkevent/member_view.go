package fkevent

import (
	"fmt"
	"image/color"
	"os"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/mattn/go-isatty"
)

type memberViewEvent struct {
	Key     string
	NewKey  string
	Name    string
	Type    string
	Content string
}

type memberAllDoneMsg struct{}

type memberAutoExitMsg struct{}

var (
	memberColorCyan   = lipgloss.Color("6")
	memberColorGreen  = lipgloss.Color("2")
	memberColorRed    = lipgloss.Color("1")
	memberColorYellow = lipgloss.Color("3")
	memberColorDim    = lipgloss.Color("8")
	memberHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(memberColorCyan)
	memberDimStyle    = lipgloss.NewStyle().Foreground(memberColorDim)
)

type memberCard struct {
	key        string
	name       string
	status     string
	operations []string
	content    string
}

type memberModel struct {
	members  []memberCard
	indexes  map[string]int
	cursor   int
	expanded int
	scrollY  int
	width    int
	done     bool
}

func newMemberModel() memberModel {
	return memberModel{
		indexes:  make(map[string]int),
		expanded: -1,
		width:    80,
	}
}

func (m memberModel) Init() tea.Cmd {
	return nil
}

func (m memberModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case memberViewEvent:
		m.applyEvent(msg)
		return m, nil
	case memberAllDoneMsg:
		m.done = true
		return m, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg { return memberAutoExitMsg{} })
	case memberAutoExitMsg:
		return m, tea.Quit
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m memberModel) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "q":
		if m.done {
			return m, tea.Quit
		}
	case "enter":
		if m.expanded >= 0 {
			m.expanded = -1
			m.scrollY = 0
		} else if len(m.members) > 0 {
			m.expanded = m.cursor
			m.scrollY = 0
		}
	case "esc":
		if m.expanded >= 0 {
			m.expanded = -1
			m.scrollY = 0
		} else if m.done {
			return m, tea.Quit
		}
	case "up", "k":
		if m.expanded >= 0 {
			if m.scrollY > 0 {
				m.scrollY--
			}
		} else if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.expanded >= 0 {
			m.scrollY++
		} else if m.cursor < len(m.members)-1 {
			m.cursor++
		}
	}
	return m, nil
}

func (m *memberModel) applyEvent(e memberViewEvent) {
	if e.Key == "" {
		return
	}
	if e.Type == "rename" && e.NewKey != "" {
		if i, ok := m.indexes[e.Key]; ok {
			if existing, exists := m.indexes[e.NewKey]; exists && existing != i {
				dst := &m.members[existing]
				src := m.members[i]
				newExisting := existing
				if existing > i {
					newExisting = existing - 1
				}
				if e.Name != "" {
					dst.name = e.Name
				}
				if dst.status == "waiting" || src.status == "error" || (src.status == "done" && dst.status != "error") {
					dst.status = src.status
				}
				dst.operations = append(dst.operations, src.operations...)
				if src.content != "" {
					dst.content += src.content
				}
				m.members = append(m.members[:i], m.members[i+1:]...)
				delete(m.indexes, e.Key)
				for key, idx := range m.indexes {
					if idx > i {
						m.indexes[key] = idx - 1
					}
				}
				if m.cursor >= len(m.members) {
					m.cursor = len(m.members) - 1
				}
				if m.expanded == i {
					m.expanded = newExisting
				} else if m.expanded > i {
					m.expanded--
				}
				return
			}
			delete(m.indexes, e.Key)
			m.indexes[e.NewKey] = i
			m.members[i].key = e.NewKey
			if e.Name != "" {
				m.members[i].name = e.Name
			}
		}
		return
	}
	i, ok := m.indexes[e.Key]
	if !ok {
		i = len(m.members)
		m.indexes[e.Key] = i
		name := e.Name
		if name == "" {
			name = e.Key
		}
		m.members = append(m.members, memberCard{key: e.Key, name: name, status: "waiting"})
	}

	card := &m.members[i]
	if e.Name != "" {
		card.name = e.Name
	}
	switch e.Type {
	case "start":
		card.status = "running"
	case "op":
		if e.Content != "" {
			card.operations = append(card.operations, e.Content)
		}
	case "content":
		card.content += e.Content
	case "done":
		card.status = "done"
		if e.Content != "" {
			card.content += e.Content
		}
	case "error":
		card.status = "error"
		if e.Content != "" {
			if card.content != "" {
				card.content += "\n"
			}
			card.content += "错误: " + e.Content
		}
	}
}

func memberStatusIcon(status string) string {
	switch status {
	case "waiting":
		return "○"
	case "running":
		return "◐"
	case "done":
		return "✓"
	case "error":
		return "✗"
	default:
		return "?"
	}
}

func memberStatusColor(status string) lipgloss.Style {
	switch status {
	case "done":
		return lipgloss.NewStyle().Foreground(memberColorGreen)
	case "error":
		return lipgloss.NewStyle().Foreground(memberColorRed)
	case "running":
		return lipgloss.NewStyle().Foreground(memberColorYellow)
	default:
		return lipgloss.NewStyle().Foreground(memberColorDim)
	}
}

func memberCardBorderColor(status string, selected bool) color.Color {
	if selected {
		return memberColorCyan
	}
	switch status {
	case "done":
		return memberColorGreen
	case "error":
		return memberColorRed
	case "running":
		return memberColorYellow
	default:
		return memberColorDim
	}
}

func (m memberModel) View() tea.View {
	var b strings.Builder
	w := m.width
	if w < 40 {
		w = 80
	}

	b.WriteString(memberHeaderStyle.Render(fmt.Sprintf("成员并行任务 %d 个智能体", len(m.members))))
	b.WriteString("\n\n")

	if len(m.members) == 0 {
		b.WriteString(memberDimStyle.Render("  等待子智能体启动..."))
		b.WriteString("\n")
		return tea.NewView(b.String())
	}

	for i := range m.members {
		if i == m.expanded {
			b.WriteString(m.renderExpanded(i, w))
		} else {
			b.WriteString(m.renderCollapsed(i, w))
		}
		b.WriteString("\n")
	}

	if !m.done {
		b.WriteString("\n")
		if m.expanded >= 0 {
			b.WriteString(memberDimStyle.Render("  ↑↓ 滚动  Enter/Esc 收起"))
		} else {
			b.WriteString(memberDimStyle.Render("  ↑↓ 选择  Enter 展开  等待成员完成..."))
		}
		b.WriteString("\n")
	}

	return tea.NewView(b.String())
}

func (m memberModel) renderCollapsed(i, w int) string {
	card := m.members[i]
	selected := !m.done && i == m.cursor

	prefix := "  "
	if selected {
		prefix = "▶ "
	}

	icon := memberStatusIcon(card.status)
	title := fmt.Sprintf("%s%s %s", prefix, memberStatusColor(card.status).Render(icon), card.name)

	if card.content != "" {
		preview := strings.TrimSpace(card.content)
		lines := strings.Split(preview, "\n")
		if len(lines) > 0 && lines[0] != "" {
			title += "\n" + memberDimStyle.Render("  "+truncateString(lines[0], 120))
		}
	}

	if len(card.operations) > 0 {
		names := make([]string, 0, min(3, len(card.operations)))
		for j := len(card.operations) - 1; j >= 0 && len(names) < 3; j-- {
			names = append(names, card.operations[j])
		}
		title += "\n" + memberDimStyle.Render("  "+strings.Join(names, " | "))
		if len(card.operations) > 3 {
			title += memberDimStyle.Render(fmt.Sprintf(" (+%d)", len(card.operations)-3))
		}
	}

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(memberCardBorderColor(card.status, selected)).
		Width(w - 6).
		PaddingLeft(1).PaddingRight(1)

	return style.Render(title)
}

func (m memberModel) renderExpanded(i, w int) string {
	card := m.members[i]

	var detail strings.Builder
	detail.WriteString(fmt.Sprintf("▼ %s %s\n", memberStatusColor(card.status).Render(memberStatusIcon(card.status)), card.name))

	output := strings.TrimSpace(card.content)
	if output != "" {
		lines := strings.Split(output, "\n")
		maxVisible := 15
		start := m.scrollY
		if start >= len(lines) {
			start = max(0, len(lines)-1)
		}
		end := min(start+maxVisible, len(lines))
		for _, line := range lines[start:end] {
			detail.WriteString(line + "\n")
		}
		if end < len(lines) {
			detail.WriteString(memberDimStyle.Render(fmt.Sprintf("  ... 还有 %d 行", len(lines)-end)) + "\n")
		}
	}

	if len(card.operations) > 0 {
		detail.WriteString("\n")
		detail.WriteString(memberDimStyle.Render("操作记录:") + "\n")
		for _, op := range card.operations {
			detail.WriteString(memberDimStyle.Render("  ▸ "+op) + "\n")
		}
	}

	var borderColor color.Color = memberColorCyan
	switch card.status {
	case "done":
		borderColor = memberColorGreen
	case "error":
		borderColor = memberColorRed
	}

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(w - 6).
		PaddingLeft(1).PaddingRight(1)

	return style.Render(detail.String())
}

type memberPanel struct {
	mu      sync.Mutex
	program *tea.Program
	done    chan struct{}
	active  bool
	enabled bool
}

func newMemberPanel() *memberPanel {
	return &memberPanel{enabled: isatty.IsTerminal(os.Stdout.Fd())}
}

func (p *memberPanel) send(e memberViewEvent) bool {
	if !p.enabled {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.active {
		p.program = tea.NewProgram(newMemberModel())
		p.done = make(chan struct{})
		p.active = true
		go func(program *tea.Program, done chan struct{}) {
			_, _ = program.Run()
			close(done)
		}(p.program, p.done)
	}
	p.program.Send(e)
	return true
}

func (p *memberPanel) finish() {
	p.mu.Lock()
	if !p.active || p.program == nil {
		p.mu.Unlock()
		return
	}
	program := p.program
	done := p.done
	p.active = false
	p.program = nil
	p.done = nil
	p.mu.Unlock()

	program.Send(memberAllDoneMsg{})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		program.Quit()
		<-done
	}
}
