package dispatch

import (
	"fmt"
	"image/color"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/mattn/go-isatty"
)

// viewEvent 子任务 UI 更新事件
type viewEvent struct {
	TaskIndex int
	Type      string // "start", "op", "content", "done", "error", "timeout"
	Content   string
}

// allDoneMsg 所有任务完成信号
type allDoneMsg struct{}

// autoExitMsg 自动退出信号
type autoExitMsg struct{}

// taskCard 单个子任务的 UI 状态
type taskCard struct {
	description string
	status      string // "waiting", "running", "done", "error", "timeout"
	operations  []string
	content     strings.Builder
}

// dispatchModel Bubble Tea 模型
type dispatchModel struct {
	tasks     []taskCard
	cursor    int
	expanded  int // -1 = 无展开
	scrollY   int
	eventCh   <-chan viewEvent
	width     int
	allDone   bool
	cancelled bool
}

func newDispatchModel(tasks []taskItem, eventCh <-chan viewEvent) dispatchModel {
	cards := make([]taskCard, len(tasks))
	for i, t := range tasks {
		cards[i] = taskCard{description: t.Description, status: "waiting"}
	}
	return dispatchModel{
		tasks:    cards,
		expanded: -1,
		eventCh:  eventCh,
		width:    80,
	}
}

// waitForEvent 从 channel 接收下一个事件
func waitForEvent(ch <-chan viewEvent) tea.Cmd {
	return func() tea.Msg {
		e, ok := <-ch
		if !ok {
			return allDoneMsg{}
		}
		return e
	}
}

func (m dispatchModel) Init() tea.Cmd {
	return waitForEvent(m.eventCh)
}

func (m dispatchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case viewEvent:
		m.applyEvent(msg)
		return m, waitForEvent(m.eventCh)

	case allDoneMsg:
		m.allDone = true
		return m, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg { return autoExitMsg{} })

	case autoExitMsg:
		return m, tea.Quit

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m dispatchModel) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Ctrl+C 中断
	if key == "ctrl+c" {
		m.cancelled = true
		return m, tea.Quit
	}

	// 退出
	if key == "q" {
		if m.allDone {
			return m, tea.Quit
		}
		return m, nil
	}

	// 展开/收起
	if key == "enter" {
		if m.expanded >= 0 {
			m.expanded = -1
			m.scrollY = 0
		} else {
			m.expanded = m.cursor
			m.scrollY = 0
		}
		return m, nil
	}

	// Esc 收起展开
	if key == "esc" {
		if m.expanded >= 0 {
			m.expanded = -1
			m.scrollY = 0
		} else if m.allDone {
			return m, tea.Quit
		}
		return m, nil
	}

	// 导航
	switch key {
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
		} else if m.cursor < len(m.tasks)-1 {
			m.cursor++
		}
	}
	return m, nil
}

func (m *dispatchModel) applyEvent(e viewEvent) {
	if e.TaskIndex < 0 || e.TaskIndex >= len(m.tasks) {
		return
	}
	card := &m.tasks[e.TaskIndex]
	switch e.Type {
	case "start":
		card.status = "running"
	case "op":
		card.operations = append(card.operations, e.Content)
	case "content":
		card.content.WriteString(e.Content)
	case "done":
		card.status = "done"
	case "error":
		card.status = "error"
		card.content.WriteString("\n错误: " + e.Content)
	case "timeout":
		card.status = "timeout"
	}
}

// --- 渲染 ---

var (
	colorCyan    = lipgloss.Color("6")
	colorGreen   = lipgloss.Color("2")
	colorRed     = lipgloss.Color("1")
	colorYellow  = lipgloss.Color("3")
	colorDim     = lipgloss.Color("8")
	headerStyle  = lipgloss.NewStyle().Bold(true).Foreground(colorCyan)
	dimStyle     = lipgloss.NewStyle().Foreground(colorDim)
	statusStyles = map[string]lipgloss.Style{
		"done":  lipgloss.NewStyle().Foreground(colorGreen),
		"error": lipgloss.NewStyle().Foreground(colorRed),
	}
)

func statusIcon(s string) string {
	switch s {
	case "waiting":
		return "○"
	case "running":
		return "◐"
	case "done":
		return "✓"
	case "error":
		return "✗"
	case "timeout":
		return "⏱"
	default:
		return "?"
	}
}

func statusColor(s string) lipgloss.Style {
	switch s {
	case "done":
		return lipgloss.NewStyle().Foreground(colorGreen)
	case "error", "timeout":
		return lipgloss.NewStyle().Foreground(colorRed)
	case "running":
		return lipgloss.NewStyle().Foreground(colorYellow)
	default:
		return lipgloss.NewStyle().Foreground(colorDim)
	}
}

func (m dispatchModel) View() tea.View {
	var b strings.Builder
	w := m.width
	if w < 40 {
		w = 80
	}

	b.WriteString(headerStyle.Render(fmt.Sprintf("⚡ 并行分发 %d 个子任务", len(m.tasks))))
	b.WriteString("\n\n")

	for i := range m.tasks {
		if i == m.expanded {
			b.WriteString(m.renderExpanded(i, w))
		} else {
			b.WriteString(m.renderCollapsed(i, w))
		}
		b.WriteString("\n")
	}

	// 底部提示
	if !m.allDone && !m.cancelled {
		b.WriteString("\n")
		if m.expanded >= 0 {
			b.WriteString(dimStyle.Render("  ↑↓ 滚动  Enter/Esc 收起"))
		} else {
			b.WriteString(dimStyle.Render("  ↑↓ 选择  Enter 展开  等待任务完成..."))
		}
		b.WriteString("\n")
	}

	return tea.NewView(b.String())
}

func cardBorderColor(status string, selected bool) color.Color {
	if selected {
		return colorCyan
	}
	switch status {
	case "done":
		return colorGreen
	case "error", "timeout":
		return colorRed
	case "running":
		return colorYellow
	default:
		return colorDim
	}
}

func (m dispatchModel) renderCollapsed(i, w int) string {
	card := m.tasks[i]
	selected := i == m.cursor

	prefix := "  "
	if selected {
		prefix = "▶ "
	}

	icon := statusIcon(card.status)
	sColor := statusColor(card.status)
	title := fmt.Sprintf("%s%s 子任务-%d: %s", prefix, sColor.Render(icon), i, card.description)

	var ops string
	if len(card.operations) > 0 {
		names := make([]string, 0, min(3, len(card.operations)))
		for j := len(card.operations) - 1; j >= 0 && len(names) < 3; j-- {
			names = append(names, card.operations[j])
		}
		ops = dimStyle.Render("  " + strings.Join(names, " | "))
		if len(card.operations) > 3 {
			ops += dimStyle.Render(fmt.Sprintf(" (+%d)", len(card.operations)-3))
		}
	}

	content := title
	if ops != "" {
		content += "\n" + ops
	}

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cardBorderColor(card.status, selected)).
		Width(w - 6).
		PaddingLeft(1).PaddingRight(1)

	return style.Render(content)
}

func (m dispatchModel) renderExpanded(i, w int) string {
	card := m.tasks[i]

	icon := statusIcon(card.status)
	sColor := statusColor(card.status)
	title := fmt.Sprintf("▼ %s 子任务-%d: %s", sColor.Render(icon), i, card.description)

	// 构建详情内容
	var detail strings.Builder
	detail.WriteString(title)
	detail.WriteString("\n")

	// 输出内容
	output := strings.TrimSpace(card.content.String())
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
			detail.WriteString(dimStyle.Render(fmt.Sprintf("  ... 还有 %d 行", len(lines)-end)) + "\n")
		}
	}

	// 操作记录
	if len(card.operations) > 0 {
		detail.WriteString("\n")
		detail.WriteString(dimStyle.Render("操作记录:") + "\n")
		for _, op := range card.operations {
			detail.WriteString(dimStyle.Render("  ▸ "+op) + "\n")
		}
	}

	var borderColor color.Color = colorCyan
	switch card.status {
	case "done":
		borderColor = colorGreen
	case "error", "timeout":
		borderColor = colorRed
	}

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(w - 6).
		PaddingLeft(1).PaddingRight(1)

	return style.Render(detail.String())
}

// runDispatchView 启动 Bubble Tea 分发视图，返回是否被用户取消
func runDispatchView(tasks []taskItem, eventCh <-chan viewEvent) bool {
	if !isatty.IsTerminal(os.Stdout.Fd()) {
		for range eventCh {
		}
		return false
	}
	p := tea.NewProgram(newDispatchModel(tasks, eventCh))
	final, _ := p.Run()
	if m, ok := final.(dispatchModel); ok {
		return m.cancelled
	}
	return false
}
