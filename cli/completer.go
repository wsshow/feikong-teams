package cli

import (
	"fkteams/agents"
	"os"
	"strings"
)

// GetWorkspaceDir 获取工作目录路径
func GetWorkspaceDir() string {
	dir := os.Getenv("FEIKONG_WORKSPACE_DIR")
	if dir == "" {
		dir = "./workspace"
	}
	return dir
}

// BuildSuggestions 构建自动补全候选项列表，供 huh Input 的 SuggestFunc 使用
// huh 内部按输入前缀自动过滤，因此返回全量候选即可
func BuildSuggestions() []string {
	var suggestions []string

	// 智能体引用
	for _, agent := range agents.GetRegistry() {
		suggestions = append(suggestions, "@"+agent.Name)
	}

	// 命令补全
	suggestions = append(suggestions,
		"quit",
		"help",
		"list_agents",
		"list_chat_history",
		"load_chat_history",
		"save_chat_history",
		"clear_chat_history",
		"clear_todo",
		"list_schedule",
		"cancel_schedule",
		"switch_work_mode",
		"save_chat_history_to_html",
		"save_chat_history_to_markdown",
	)

	// 工作目录顶层文件引用
	baseDir := GetWorkspaceDir()
	if entries, err := os.ReadDir(baseDir); err == nil {
		for _, entry := range entries {
			name := entry.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			if entry.IsDir() {
				suggestions = append(suggestions, "#"+name+"/")
			} else {
				suggestions = append(suggestions, "#"+name)
			}
		}
	}

	return suggestions
}
