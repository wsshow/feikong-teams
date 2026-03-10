package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fkteams/agents"
	"fkteams/tui"
)

// SelectAgent 以可过滤的列表形式选择智能体
func SelectAgent() (string, error) {
	registry := agents.GetRegistry()
	if len(registry) == 0 {
		return "", fmt.Errorf("无可用的智能体")
	}

	var items []tui.SelectItem
	for _, a := range registry {
		items = append(items, tui.SelectItem{
			Label: fmt.Sprintf("%s - %s", a.Name, a.Description),
			Value: a.Name,
		})
	}
	return tui.SelectFromList("选择智能体", items)
}

// SelectFileOrDir 以可过滤的列表形式选择工作目录下的文件或目录
// 进入目录后可选择 "✓ 选择当前目录" 来选中该目录
func SelectFileOrDir(baseDir string) (string, error) {
	currentDir := baseDir

	for {
		entries, err := os.ReadDir(currentDir)
		if err != nil {
			return "", err
		}

		var items []tui.SelectItem

		if rel, _ := filepath.Rel(baseDir, currentDir); rel != "." {
			items = append(items, tui.SelectItem{Label: "← 返回上级目录", Value: ".."})
			items = append(items, tui.SelectItem{Label: "✓ 选择当前目录", Value: "."})
		}

		for _, entry := range entries {
			name := entry.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			label := name
			if entry.IsDir() {
				label += "/"
			}
			items = append(items, tui.SelectItem{Label: label, Value: name})
		}

		if len(items) == 0 {
			return "", fmt.Errorf("目录为空: %s", currentDir)
		}

		displayDir, _ := filepath.Rel(baseDir, currentDir)
		if displayDir == "." {
			displayDir = "工作目录"
		}

		selected, selectErr := tui.SelectFromList("选择文件/目录 ["+displayDir+"]", items, 15)
		if selectErr != nil {
			return "", selectErr
		}

		if selected == ".." {
			currentDir = filepath.Dir(currentDir)
			continue
		}

		if selected == "." {
			result, _ := filepath.Rel(baseDir, currentDir)
			return filepath.ToSlash(result), nil
		}

		fullPath := filepath.Join(currentDir, selected)
		info, statErr := os.Stat(fullPath)
		if statErr != nil {
			return "", statErr
		}

		if info.IsDir() {
			currentDir = fullPath
			continue
		}

		result, _ := filepath.Rel(baseDir, fullPath)
		return filepath.ToSlash(result), nil
	}
}

// CommandInfo 命令信息
type CommandInfo struct {
	Name string
	Desc string
}

// allCommands 所有可用的交互式命令
var allCommands = []CommandInfo{
	{"help", "帮助信息"},
	{"list_agents", "列出所有可用的智能体"},
	{"list_chat_history", "列出所有聊天历史会话"},
	{"load_chat_history", "选择并加载聊天历史会话"},
	{"save_chat_history", "保存聊天历史到当前会话文件"},
	{"clear_chat_history", "清空当前聊天历史"},
	{"switch_work_mode", "切换工作模式(团队/深度/讨论/自定义)"},
	{"save_chat_history_to_html", "导出聊天历史为 HTML 文件"},
	{"save_chat_history_to_markdown", "导出聊天历史为 Markdown 文件"},
	{"clear_todo", "清空所有待办事项"},
	{"list_schedule", "列出所有定时任务"},
	{"cancel_schedule", "选择并取消定时任务"},
	{"list_memory", "列出所有长期记忆条目"},
	{"delete_memory", "选择并删除记忆条目"},
	{"clear_memory", "清空所有长期记忆"},
	{"quit", "退出程序"},
}

// SelectCommand 以可过滤的列表形式选择命令
func SelectCommand() (string, error) {
	var items []tui.SelectItem
	for _, c := range allCommands {
		items = append(items, tui.SelectItem{
			Label: fmt.Sprintf("%s - %s", c.Name, c.Desc),
			Value: c.Name,
		})
	}
	return tui.SelectFromList("选择命令", items)
}
