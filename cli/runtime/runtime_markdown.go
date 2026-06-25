package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fkteams/internal/app/agent/catalog"
	"fkteams/internal/app/appstate"
	"fkteams/internal/app/config"
	"fkteams/internal/app/version"
	"fkteams/tui"
)

func runtimeHelpMarkdown() string {
	var sb strings.Builder
	sb.WriteString("常用命令按场景分组如下。\n\n")
	for _, category := range runtimeCommandCategories() {
		sb.WriteString("## " + category + "\n\n")
		for _, command := range allCommands {
			if command.Category != category {
				continue
			}
			fmt.Fprintf(&sb, "- `%s` %s\n", runtimeCommandSyntax(command), command.Desc)
		}
		sb.WriteString("\n")
	}
	sb.WriteString("## 输入\n\n")
	sb.WriteString("- `@agent` 指定智能体\n")
	sb.WriteString("- `#file` 引用文件\n")
	sb.WriteString("- `Shift+Enter` 输入换行\n")
	sb.WriteString("- 任务运行中输入内容并按 `Enter` 发送转向消息\n")
	sb.WriteString("- 任务运行中按 `Esc` 暂停当前任务，并将未执行的转向消息回填到输入框\n")
	sb.WriteString("\n直接输入问题即可与智能体团队对话。")
	return sb.String()
}

func runtimeCommandCategories() []string {
	categories := make([]string, 0)
	seen := map[string]bool{}
	for _, command := range allCommands {
		if command.Category == "" || seen[command.Category] {
			continue
		}
		seen[command.Category] = true
		categories = append(categories, command.Category)
	}
	return categories
}

func runtimeCommandSyntax(command CommandInfo) string {
	syntax := "/" + command.Name
	if command.Usage != "" {
		syntax += " " + command.Usage
	}
	return syntax
}

func runtimeAgentsMarkdown() string {
	var sb strings.Builder
	sb.WriteString("| 智能体 | 说明 |\n")
	sb.WriteString("|--------|------|\n")
	for _, info := range agents.GetRegistry() {
		fmt.Fprintf(&sb, "| %s | %s |\n", info.Name, strings.ReplaceAll(info.Description, "|", "\\|"))
	}
	return sb.String()
}

func runtimeChatHistoryMarkdown(interactive bool) string {
	items, err := runtimeSessionPickerItems()
	if err != nil {
		return "读取历史目录失败: " + err.Error()
	}
	if len(items) == 0 {
		return "暂无聊天历史文件"
	}

	var sb strings.Builder
	sb.WriteString("| 会话 ID | 标题 |\n")
	sb.WriteString("|---------|------|\n")
	for _, item := range items {
		fmt.Fprintf(&sb, "| `%s` | %s |\n", markdownEscapeTable(item.Value), markdownEscapeTable(item.Label))
	}
	if interactive {
		fmt.Fprintf(&sb, "\n共 **%d** 个会话，使用 `load_chat_history` 加载。", len(items))
	} else {
		fmt.Fprintf(&sb, "\n共 **%d** 个会话，使用 `fkteams --resume <session_id>` 恢复。", len(items))
	}
	return sb.String()
}

func runtimeMemoryMarkdown(manager appstate.MemoryManager) string {
	entries, err := runtimeMemoryEntriesFrom(manager)
	if err != nil {
		return err.Error()
	}
	if len(entries) == 0 {
		return "暂无长期记忆条目"
	}
	var sb strings.Builder
	sb.WriteString("| 类型 | 摘要 | 详情 | 命中 |\n")
	sb.WriteString("|------|------|------|------|\n")
	for _, entry := range entries {
		fmt.Fprintf(&sb, "| %s | %s | %s | %d |\n",
			markdownEscapeTable(string(entry.Type)),
			markdownEscapeTable(entry.Summary),
			markdownEscapeTable(entry.Detail),
			entry.HitCount,
		)
	}
	fmt.Fprintf(&sb, "\n共 **%d** 条记忆，使用 `delete_memory` 删除条目，或 `clear_memory` 清空全部。", len(entries))
	return sb.String()
}

func runtimeScheduleMarkdown() string {
	tasks, err := runtimeScheduledTasks("")
	if err != nil {
		return err.Error()
	}
	if len(tasks) == 0 {
		return "暂无定时任务"
	}
	var sb strings.Builder
	sb.WriteString("| ID | 状态 | 任务 | 下次执行 |\n")
	sb.WriteString("|----|------|------|----------|\n")
	for _, task := range tasks {
		fmt.Fprintf(&sb, "| `%s` | %s | %s | %s |\n",
			markdownEscapeTable(task.ID),
			markdownEscapeTable(string(task.Status)),
			markdownEscapeTable(truncateRuntimeText(task.Task, 80)),
			task.NextRunAt.Format("2006-01-02 15:04"),
		)
	}
	fmt.Fprintf(&sb, "\n共 **%d** 个定时任务。", len(tasks))
	return sb.String()
}

func markdownEscapeTable(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.ReplaceAll(value, "|", "\\|")
}

func runtimeWelcomeInfo(session *Session) tui.WelcomeInfo {
	modelName := ""
	if mc := config.Get().ResolveModel("default"); mc != nil {
		modelName = mc.Model
		if mc.Provider != "" {
			modelName = mc.Provider + "/" + modelName
		}
	}
	return tui.WelcomeInfo{
		Version:   fmt.Sprint(version.Get()),
		Mode:      runtimeModeName(session.CurrentMode),
		SessionID: activeSessionID,
		Workspace: runtimeShortPath(GetWorkspaceDir()),
		Model:     modelName,
	}
}

func runtimeModeName(mode WorkMode) string {
	switch mode {
	case ModeDeep:
		return "深度模式"
	case ModeGroup:
		return "多智能体讨论模式"
	case ModeCustom:
		return "自定义会议模式"
	default:
		return "团队模式"
	}
}

func runtimeShortPath(path string) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		if rel, relErr := filepath.Rel(home, path); relErr == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(filepath.Join("~", rel))
		}
	}
	return filepath.ToSlash(path)
}

func (m runtimeModel) runtimeRenderMarkdown(content string) string {
	return tui.TrimRenderedIndent(tui.RenderMarkdownWithWidth(content, m.contentWidth()))
}

func (m runtimeModel) contentWidth() int {
	width := m.screenWidth()
	return max(20, width-runtimeHorizontalGutter*2)
}

func (m runtimeModel) screenWidth() int {
	width := m.width
	if width <= 0 {
		width = 100
	}
	return max(24, width)
}
