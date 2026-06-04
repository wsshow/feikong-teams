package tui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

const (
	defaultToolDisplayName = "tool"
)

type ToolStatus string

const (
	ToolStatusRunning ToolStatus = "running"
	ToolStatusDone    ToolStatus = "done"
	ToolStatusError   ToolStatus = "error"
)

type ToolChainItem struct {
	Name   string
	Args   string
	Status string
	Error  string
}

type WelcomeInfo struct {
	Version   string
	Mode      string
	SessionID string
	Workspace string
	Model     string
}

func PromptMarker() string {
	return "❯ "
}

func DoneMarker() string {
	return "✻ "
}

func RenderRuntimeInputBox(width int, content string, hint string) string {
	width = max(20, width)
	line := dividerStyle().Render(strings.Repeat("─", width))
	bottomLine := dividerStyle().Render(strings.Repeat("─", width))
	inputLine := strings.Join(WrapStyledLine(content, width), "\n")
	hintLine := dimStyle().Render(hint)
	return strings.Join([]string{line, inputLine, bottomLine, hintLine}, "\n")
}

func JumpToBottomButton() string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("236")).
		Render(" Jump to bottom (End) ↓ ")
}

func CenterLine(content string, width int) string {
	if width <= 0 {
		return content
	}
	contentWidth := CellWidth(StripANSI(content))
	if contentWidth >= width {
		return content
	}
	return strings.Repeat(" ", (width-contentWidth)/2) + content
}

func RightLine(content string, width int) string {
	if width <= 0 {
		return content
	}
	contentWidth := CellWidth(StripANSI(content))
	if contentWidth >= width {
		return content
	}
	return strings.Repeat(" ", width-contentWidth) + content
}

func RenderRuntimeScreen(content string, width int, height int, gutter int) string {
	if width <= 0 {
		width = 100
	}
	if height <= 0 {
		height = LineCount(content)
	}
	if gutter < 0 {
		gutter = 0
	}
	if gutter*2 >= width {
		gutter = 0
	}
	contentWidth := max(1, width-gutter*2)
	left := strings.Repeat(" ", gutter)
	right := strings.Repeat(" ", gutter)
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	for i, line := range lines {
		if CellWidth(StripANSI(line)) > contentWidth {
			line = SliceCells(StripANSI(line), 0, contentWidth)
		}
		lines[i] = left + line + strings.Repeat(" ", max(0, contentWidth-CellWidth(StripANSI(line)))) + right
	}
	return strings.Join(lines, "\n")
}

func RenderUserMessageBlock(content string, width int) string {
	if width <= 0 {
		width = 100
	}
	lineWidth := max(20, width-2)
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	for i, line := range lines {
		prefix := "  "
		if i == 0 {
			prefix = PromptMarker()
		}
		lines[i] = userLineStyle(lineWidth).Render(prefix + userTextStyle().Render(line))
	}
	return strings.Join(lines, "\n")
}

func RenderWelcomePanel(info WelcomeInfo, width int) string {
	if width <= 0 {
		width = 100
	}
	panelWidth := max(60, width)
	leftWidth := max(24, panelWidth/3)
	rightWidth := max(32, panelWidth-leftWidth-5)

	left := welcomeLeft(info, leftWidth)
	right := welcomeRight(rightWidth)
	bodyHeight := max(LineCount(left), LineCount(right))
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, welcomeDivider(bodyHeight), right)
	return welcomePanelStyle(panelWidth).Render(body)
}

func welcomeLeft(info WelcomeInfo, width int) string {
	var lines []string
	lines = append(lines,
		welcomeTitleStyle().Render("非空小队 "+info.Version),
		Dim("欢迎回来"),
		"",
		welcomeMarkStyle().Render("  协同思考 · 稳定推进"),
		welcomeMarkStyle().Render("  ─────────"),
		welcomeMarkStyle().Render("  多智能体协作终端"),
		"",
		Dim("模式: ")+info.Mode,
		Dim("模型: ")+emptyAs(info.Model, "未配置"),
		Dim("工作区: ")+emptyAs(info.Workspace, "."),
		Dim("会话: ")+emptyAs(info.SessionID, "新会话"),
	)
	return lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n"))
}

func welcomeRight(width int) string {
	lines := []string{
		welcomeSectionStyle().Render("开始使用"),
		"直接输入目标，我会拆解、执行并汇总结果。",
		"",
		welcomeSectionStyle().Render("常用入口"),
		"/ help          查看命令",
		"@ agent         切换智能体",
		"# file          引用文件",
		"Shift+Enter    输入换行",
		"",
		welcomeSectionStyle().Render("终端操作"),
		"滚轮翻阅历史，拖拽选择后自动复制。",
	}
	return lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n"))
}

func welcomeDivider(height int) string {
	if height < 1 {
		height = 1
	}
	lines := make([]string, height)
	for i := range lines {
		lines[i] = "  │  "
	}
	return Dim(strings.Join(lines, "\n"))
}

func welcomePanelStyle(width int) lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("6")).
		Padding(1, 2).
		Width(width).
		BorderTop(true).
		BorderBottom(true).
		BorderLeft(true).
		BorderRight(true).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("6")).
		Align(lipgloss.Left).
		Inline(false)
}

func welcomeBrandStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
}

func welcomeMarkStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
}

func welcomeSectionStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
}

func welcomeTitleStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
}

func emptyAs(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func Dim(text string) string {
	return dimStyle().Render(text)
}

func Key(text string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Render(text)
}

func Status(text string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render(text)
}

func ToolCall(name string, args string, status ToolStatus) string {
	name = emptyAs(name, defaultToolDisplayName)
	args = toolArgsSummary(args)
	label := name
	if args != "" {
		label = name + "(" + truncateRunes(args, 88) + ")"
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(toolStatusColor(status))).Render("● ") +
		lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true).Render(label)
}

func ToolResult(name string, args string, content string, status ToolStatus) string {
	if strings.TrimSpace(content) == "" {
		return ToolCall(name, args, status)
	}
	line, hidden := toolResultPreviewLine(content)
	rendered := make([]string, 0, 3)
	rendered = append(rendered, ToolCall(name, args, status))
	if line != "" {
		if hidden > 0 {
			line += "  ... 隐藏 " + formatInt(hidden) + " 行"
		}
		rendered = append(rendered, dimStyle().Render("  └ "+line))
	}
	return strings.Join(rendered, "\n")
}

func RenderToolChainLines(items []ToolChainItem, lineWidth int) []string {
	if len(items) == 0 {
		return []string{"  工具链: 等待工具"}
	}
	const maxTools = 6
	start := 0
	if len(items) > maxTools {
		start = len(items) - maxTools
	}
	lines := make([]string, 0, len(items)-start+2)
	lines = append(lines, "  工具链:")
	if start > 0 {
		lines = append(lines, "  │  ... 省略 "+formatInt(start)+" 个较早工具")
	}
	visible := items[start:]
	for i, item := range visible {
		name := item.Name
		if name == "" {
			name = defaultToolDisplayName
		}
		branch := "├─"
		if i == len(visible)-1 {
			branch = "└─"
		}
		label := toolTreeLabel(name, item.Args, max(16, lineWidth-8))
		if isToolChainErrorStatus(item.Status) {
			reason := item.Error
			if strings.TrimSpace(reason) == "" {
				reason = "未返回错误原因"
			}
			label = "✗ " + label + " · " + truncateRunes(strings.Join(strings.Fields(reason), " "), max(8, lineWidth-CellWidth(StripANSI(label))-14))
		}
		lines = append(lines, "  "+branch+" "+label)
	}
	return lines
}

func isToolChainErrorStatus(status string) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	return status == "error" || status == "failed" || status == "失败"
}

func toolTreeLabel(name, args string, maxWidth int) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = defaultToolDisplayName
	}
	summary := toolArgsSummary(args)
	if summary == "" {
		return truncateRunes(name, maxWidth)
	}
	prefix := name + ": "
	return prefix + truncateRunes(summary, max(8, maxWidth-CellWidth(StripANSI(prefix))))
}

func toolStatusColor(status ToolStatus) string {
	switch status {
	case ToolStatusRunning:
		return "3"
	case ToolStatusError:
		return "1"
	case ToolStatusDone:
		return "10"
	default:
		return "8"
	}
}

func Interrupted(text string) string {
	return dimStyle().Render("  └─ " + text)
}

func System(text string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(text)
}

func Error(text string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true).Render(text)
}

func Tool(text string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(text)
}

func Reasoning(text string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true).Render(text)
}

func Banner(text string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true).Render(text)
}

func PickerBox(width int, content string) string {
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("8")).
		Padding(0, 1).
		Width(width).
		Render(content)
}

func PickerTitle(text string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true).Render(text)
}

func PickerSelected(text string) string {
	return Key(text)
}

func dividerStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
}

func dimStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
}

func userLineStyle(width int) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("236")).
		Padding(0, 0).
		Width(max(20, width))
}

func userTextStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
}

func toolResultPreviewLine(content string) (string, int) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	content = strings.TrimSpace(content)
	rawLines := strings.Split(content, "\n")
	var parts []string
	hidden := 0
	for _, line := range rawLines {
		line = strings.TrimSpace(line)
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(parts) < 2 {
			parts = append(parts, strings.Join(strings.Fields(line), " "))
		} else {
			hidden++
		}
	}
	return truncateRunes(strings.Join(parts, "  "), 160), hidden
}

func formatInt(n int) string {
	return strconv.Itoa(n)
}
