package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

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
	innerWidth := max(20, width-2)
	line := dividerStyle().Render(strings.Repeat("─", innerWidth))
	inputLine := lipgloss.NewStyle().Width(innerWidth).Render(content)
	hintLine := dimStyle().Render(hint)
	return strings.Join([]string{line, inputLine, line, hintLine}, "\n")
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
	panelWidth := max(60, width-4)
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
