package cli

import (
	"fkteams/agents"
	"os"
	"path/filepath"
	"strings"

	"github.com/c-bata/go-prompt"
)

// GetWorkspaceDir 获取工作目录路径
func GetWorkspaceDir() string {
	dir := os.Getenv("FEIKONG_WORKSPACE_DIR")
	if dir == "" {
		dir = "./workspace"
	}
	return dir
}

// listFileSuggestions 根据输入路径列出文件/文件夹补全建议
func listFileSuggestions(partialPath string) []prompt.Suggest {
	baseDir := GetWorkspaceDir()

	// 确定要列出的目录和过滤前缀
	listDir := baseDir
	filterPrefix := partialPath

	if partialPath != "" {
		// 如果以 / 结尾，直接列出该子目录
		if strings.HasSuffix(partialPath, "/") {
			listDir = filepath.Join(baseDir, partialPath)
			filterPrefix = ""
		} else {
			// 提取目录部分和文件名前缀
			dir := filepath.Dir(partialPath)
			if dir != "." {
				listDir = filepath.Join(baseDir, dir)
			}
			filterPrefix = filepath.Base(partialPath)
		}
	}

	entries, err := os.ReadDir(listDir)
	if err != nil {
		return []prompt.Suggest{}
	}

	// 计算相对路径前缀（用于构建完整路径）
	relPrefix := ""
	if partialPath != "" {
		if strings.HasSuffix(partialPath, "/") {
			relPrefix = partialPath
		} else {
			dir := filepath.Dir(partialPath)
			if dir != "." {
				relPrefix = dir + "/"
			}
		}
	}

	var suggests []prompt.Suggest
	for _, entry := range entries {
		name := entry.Name()
		// 跳过隐藏文件
		if strings.HasPrefix(name, ".") {
			continue
		}

		// 前缀过滤
		if filterPrefix != "" && !strings.HasPrefix(strings.ToLower(name), strings.ToLower(filterPrefix)) {
			continue
		}

		displayName := relPrefix + name
		desc := "文件"
		if entry.IsDir() {
			displayName += "/"
			desc = "目录"
		}

		suggests = append(suggests, prompt.Suggest{
			Text:        "#" + displayName,
			Description: desc,
		})
	}

	return suggests
}

// Completer go-prompt 补全函数，支持 @ 智能体引用和 # 文件引用
func Completer(d prompt.Document) []prompt.Suggest {
	textBefore := d.TextBeforeCursor()

	// 检查是否输入了 # 符号（文件引用）
	if strings.HasPrefix(textBefore, "#") || strings.Contains(textBefore, " #") {
		lastHash := strings.LastIndex(textBefore, "#")
		afterHash := textBefore[lastHash+1:]

		// 如果 # 后面有空格，则不提示
		if strings.Contains(afterHash, " ") {
			return []prompt.Suggest{}
		}

		return listFileSuggestions(afterHash)
	}

	// 检查是否输入了 @ 符号
	if strings.HasPrefix(textBefore, "@") || strings.Contains(textBefore, " @") {
		// 提取 @ 之后的内容
		lastAt := strings.LastIndex(textBefore, "@")
		afterAt := textBefore[lastAt+1:]

		// 如果 @ 后面有空格，则不提示
		if strings.Contains(afterAt, " ") {
			return []prompt.Suggest{}
		}

		// 构建智能体建议列表
		var agentSuggests []prompt.Suggest
		for _, agent := range agents.GetRegistry() {
			agentSuggests = append(agentSuggests, prompt.Suggest{
				Text:        "@" + agent.Name,
				Description: agent.Description,
			})
		}

		// 过滤建议
		if afterAt == "" {
			return agentSuggests
		}
		return prompt.FilterHasPrefix(agentSuggests, "@"+afterAt, true)
	}

	if d.GetWordBeforeCursor() == "" {
		return []prompt.Suggest{}
	}
	s := []prompt.Suggest{
		{Text: "quit", Description: "退出"},
		{Text: "list_agents", Description: "列出所有可用的智能体"},
		{Text: "load_chat_history", Description: "加载聊天历史"},
		{Text: "save_chat_history", Description: "保存聊天历史"},
		{Text: "clear_chat_history", Description: "清空聊天历史"},
		{Text: "clear_todo", Description: "清空待办事项"},
		{Text: "switch_work_mode", Description: "切换工作模式(团队模式/多智能体讨论模式)"},
		{Text: "save_chat_history_to_html", Description: "保存完整聊天历史到 HTML 文件"},
		{Text: "save_chat_history_to_markdown", Description: "保存完整聊天历史到 Markdown 文件"},
		{Text: "help", Description: "帮助信息"},
	}
	if d.TextBeforeCursor() == "/" {
		return s
	}
	return prompt.FilterHasPrefix(s, d.GetWordBeforeCursor(), true)
}
