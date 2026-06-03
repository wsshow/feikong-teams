package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-isatty"
)

type MemberEvent struct {
	Key      string
	NewKey   string
	Name     string
	Type     string
	Content  string
	ToolKey  string
	ToolName string
	Append   bool
}

var (
	memberColorGreen  = lipgloss.Color("2")
	memberColorRed    = lipgloss.Color("1")
	memberColorYellow = lipgloss.Color("3")
	memberColorDim    = lipgloss.Color("8")
	memberDimStyle    = lipgloss.NewStyle().Foreground(memberColorDim)
)

type memberCard struct {
	key        string
	name       string
	status     string
	operations []string
	content    string
	errorText  string
	task       string
	tools      []memberToolFlow
	toolIndex  map[string]int
}

type memberToolFlow struct {
	key    string
	name   string
	status string
	args   string
	result string
	error  string
}

type memberModel struct {
	members   []memberCard
	indexes   map[string]int
	width     int
	done      bool
	emptyText string
}

func newMemberModel(emptyText string) memberModel {
	return memberModel{
		indexes:   make(map[string]int),
		width:     80,
		emptyText: emptyText,
	}
}

func (m *memberModel) applyEvent(e MemberEvent) {
	if e.Key == "" {
		return
	}
	if e.Type == "rename" && e.NewKey != "" {
		if i, ok := m.indexes[e.Key]; ok {
			if existing, exists := m.indexes[e.NewKey]; exists && existing != i {
				dst := &m.members[existing]
				src := m.members[i]
				if e.Name != "" {
					dst.name = e.Name
				}
				if dst.status == "waiting" || src.status == "error" || (src.status == "done" && dst.status != "error") {
					dst.status = src.status
				}
				dst.operations = append(dst.operations, src.operations...)
				if dst.toolIndex == nil {
					dst.toolIndex = make(map[string]int)
				}
				for _, tool := range src.tools {
					dstTool := dst.ensureTool(tool.key, tool.name)
					dstTool.status = tool.status
					dstTool.args += tool.args
					dstTool.result += tool.result
					dstTool.error = tool.error
				}
				if src.content != "" {
					dst.content += src.content
				}
				if dst.errorText == "" {
					dst.errorText = src.errorText
				}
				m.members = append(m.members[:i], m.members[i+1:]...)
				delete(m.indexes, e.Key)
				for key, idx := range m.indexes {
					if idx > i {
						m.indexes[key] = idx - 1
					}
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
		m.members = append(m.members, memberCard{key: e.Key, name: name, status: "waiting", toolIndex: make(map[string]int)})
	}

	card := &m.members[i]
	if card.toolIndex == nil {
		card.toolIndex = make(map[string]int)
	}
	if e.Name != "" {
		card.name = e.Name
	}
	switch e.Type {
	case "start":
		card.status = "running"
	case "op":
		if e.Content != "" {
			card.operations = append(card.operations, e.Content)
			if task := memberTaskFromOperation(e.Content); task != "" {
				card.task = task
			}
		}
	case "tool_prepare":
		tool := card.ensureTool(e.ToolKey, e.ToolName)
		tool.status = "参数准备中"
	case "tool_args":
		tool := card.ensureTool(e.ToolKey, e.ToolName)
		tool.status = "已调用"
		if e.Append {
			tool.args += e.Content
		} else {
			tool.args = e.Content
		}
	case "tool_result":
		tool := card.ensureTool(e.ToolKey, e.ToolName)
		if e.Append {
			tool.result += e.Content
		} else {
			tool.result = e.Content
		}
		if isErrorSummary(e.Content) {
			tool.status = "失败"
			tool.error = errorSummary(e.Content)
		} else {
			tool.status = "已完成"
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
		card.errorText = errorSummary(e.Content)
		if card.errorText == "" {
			card.errorText = "未返回错误原因"
		}
		if e.ToolKey != "" || e.ToolName != "" {
			tool := card.ensureTool(e.ToolKey, e.ToolName)
			tool.status = "失败"
			tool.error = card.errorText
		}
		if card.errorText != "" {
			if card.content != "" {
				card.content += "\n"
			}
			card.content += "错误: " + card.errorText
		}
	}
}

func (c *memberCard) ensureTool(key, name string) *memberToolFlow {
	if key == "" {
		key = name
	}
	if key == "" {
		key = fmt.Sprintf("tool:%d", len(c.tools)+1)
	}
	if name == "" {
		name = key
	}
	if c.toolIndex == nil {
		c.toolIndex = make(map[string]int)
	}
	if i, ok := c.toolIndex[key]; ok {
		if name != "" {
			c.tools[i].name = name
		}
		return &c.tools[i]
	}
	c.tools = append(c.tools, memberToolFlow{key: key, name: name, status: "参数准备中"})
	c.toolIndex[key] = len(c.tools) - 1
	return &c.tools[len(c.tools)-1]
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

func (m memberModel) View() string {
	var b strings.Builder
	w := m.width
	if w < 40 {
		w = 80
	}

	if len(m.members) == 0 {
		emptyText := m.emptyText
		if emptyText == "" {
			emptyText = "等待事件..."
		}
		b.WriteString(memberDimStyle.Render("  " + emptyText))
		b.WriteString("\n")
		return b.String()
	}

	for i := range m.members {
		b.WriteString(m.renderCard(i, w))
		b.WriteString("\n")
	}

	return b.String()
}

func (m memberModel) renderCard(i, w int) string {
	card := m.members[i]
	lineWidth := max(20, w-4)

	icon := memberStatusIcon(card.status)
	name := truncateRunes(card.name, 16)
	status := memberStatusText(card)
	task := card.task
	if task == "" {
		task = latestMemberOperation(card.operations)
	}
	if task == "" {
		task = "任务准备中"
	}
	task = truncateRunes(memberCompactLine(task), max(12, lineWidth-8))

	var b strings.Builder
	fmt.Fprintf(&b, "%s %s  %s",
		memberStatusColor(card.status).Render(icon),
		name,
		memberDimStyle.Render(status),
	)
	fmt.Fprintf(&b, "\n%s", memberDimStyle.Render("  目标: "+task))
	if card.status == "error" {
		reason := card.errorText
		if reason == "" {
			reason = "未返回错误原因"
		}
		fmt.Fprintf(&b, "\n%s", memberStatusColor("error").Render("  原因: "+truncateRunes(memberCompactLine(reason), max(12, lineWidth-8))))
	}
	for _, line := range memberToolChainLines(card.tools, lineWidth) {
		fmt.Fprintf(&b, "\n%s", memberDimStyle.Render(line))
	}
	return b.String()
}

func memberCompactLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return strings.Join(strings.Fields(s), " ")
}

func latestMemberOperation(ops []string) string {
	for i := len(ops) - 1; i >= 0; i-- {
		if op := memberCompactLine(ops[i]); op != "" {
			return strings.TrimPrefix(op, "任务: ")
		}
	}
	return ""
}

func memberStatusText(card memberCard) string {
	switch card.status {
	case "done":
		if len(card.tools) > 0 {
			return fmt.Sprintf("完成 · 工具 %d", len(card.tools))
		}
		return "完成"
	case "error":
		return "失败"
	case "waiting":
		return "等待中"
	}
	if current := currentMemberTool(card.tools); current != "" {
		return "运行中 · " + current
	}
	return "运行中"
}

func currentMemberTool(tools []memberToolFlow) string {
	for i := len(tools) - 1; i >= 0; i-- {
		tool := tools[i]
		if tool.status == "已完成" || tool.status == "失败" {
			continue
		}
		name := tool.name
		if name == "" {
			name = tool.key
		}
		name = toolLabel(name, tool.args, 32)
		if tool.status == "" {
			return name
		}
		return fmt.Sprintf("%s %s", name, tool.status)
	}
	return ""
}

func memberToolChainLines(tools []memberToolFlow, lineWidth int) []string {
	if len(tools) == 0 {
		return []string{"  工具链: 等待工具"}
	}
	const maxTools = 6
	start := 0
	if len(tools) > maxTools {
		start = len(tools) - maxTools
	}
	lines := make([]string, 0, len(tools)-start+2)
	lines = append(lines, "  工具链:")
	if start > 0 {
		lines = append(lines, fmt.Sprintf("  │  ... 省略 %d 个较早工具", start))
	}
	for i, tool := range tools[start:] {
		name := tool.name
		if name == "" {
			name = tool.key
		}
		branch := "├─"
		if i == len(tools[start:])-1 {
			branch = "└─"
		}
		label := toolTreeLabel(name, tool.args, max(16, lineWidth-8))
		if tool.status == "失败" {
			reason := tool.error
			if reason == "" {
				reason = "未返回错误原因"
			}
			label = "✗ " + label + " · " + truncateRunes(memberCompactLine(reason), max(8, lineWidth-runewidthStringWidth(label)-14))
		}
		lines = append(lines, fmt.Sprintf("  %s %s", branch, label))
	}
	return lines
}

func toolTreeLabel(name, args string, maxWidth int) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "tool"
	}
	summary := toolArgsSummary(args)
	if summary == "" {
		return truncateRunes(name, maxWidth)
	}
	prefix := name + ": "
	return prefix + truncateRunes(summary, max(8, maxWidth-runewidthStringWidth(prefix)))
}

func memberTaskFromOperation(op string) string {
	op = strings.TrimSpace(strings.TrimPrefix(op, "任务:"))
	if op == "" || op == "任务准备中" || op == "任务已分配" || op == "任务参数接收中" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(op), &payload); err == nil {
		for _, key := range []string{"request", "task", "prompt", "query", "goal"} {
			if v, ok := payload[key].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	if strings.HasPrefix(op, "{") || strings.HasPrefix(op, "[") {
		return ""
	}
	return op
}

func isErrorSummary(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(content, "执行出错") ||
		strings.Contains(lower, "error") ||
		strings.Contains(lower, "failed") ||
		strings.Contains(content, "失败")
}

func errorSummary(content string) string {
	content = memberCompactLine(content)
	if content == "" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(content), &payload); err == nil {
		for _, key := range []string{"error", "message", "reason", "ErrorMessage"} {
			if v, ok := payload[key].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	return content
}

type MemberPanel struct {
	mu        sync.Mutex
	model     memberModel
	active    bool
	enabled   bool
	emptyText string
	lastLines int
	lastView  string
}

func NewMemberPanel() *MemberPanel {
	return &MemberPanel{enabled: isatty.IsTerminal(os.Stdout.Fd()), emptyText: "等待子智能体启动..."}
}

func (p *MemberPanel) Send(e MemberEvent) bool {
	if !p.enabled {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.active {
		p.model = newMemberModel(p.emptyText)
		p.active = true
	}
	p.model.applyEvent(e)
	p.renderLocked()
	return true
}

func (p *MemberPanel) Finish() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.active {
		return
	}
	p.model.done = true
	p.renderLocked()
	p.active = false
	p.lastLines = 0
	p.lastView = ""
}

func (p *MemberPanel) renderLocked() {
	p.model.width = terminalWidth()
	view := p.model.View()
	if view == p.lastView {
		return
	}
	if p.lastLines > 0 {
		fmt.Printf("\033[%dF\033[J", p.lastLines)
	}
	fmt.Print(view)
	p.lastLines = renderedLineCount(view, p.model.width)
	p.lastView = view
}
