package fkevent

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

var PrintEvent = printEvent()

func printEvent() func(Event) {
	agentName := ""
	lastToolName := ""
	return func(event Event) {
		switch event.Type {
		case "stream_chunk":
			if agentName != event.AgentName {
				agentName = event.AgentName
				fmt.Printf("\n\033[1;36m╭─ [%s] %s\033[0m\n", agentName, event.RunPath)
				fmt.Printf("\033[1;36m╰─▶\033[0m ")
			}
			fmt.Printf("%s", event.Content)

		case "message":
			if event.Content != "" {
				fmt.Printf("\n\033[1;32m✓ [%s] 消息:\033[0m %s\n", event.AgentName, event.Content)
			}

		case "tool_result":
			fmt.Printf("\n\033[1;33m⚙ [%s] 工具结果:\033[0m\n", event.AgentName)
			if event.Content != "" {
				var formatted string
				switch lastToolName {
				case "duckduckgo_search":
					formatted = formatSearchResults(event.Content)
				case "execute_command", "command_execute":
					formatted = formatCommandResult(event.Content)
				case "file_read", "file_edit", "file_list", "file_create", "file_delete", "dir_create", "dir_delete", "file_search":
					formatted = formatFileOpResult(event.Content)
				case "file_patch":
					formatted = formatFilePatchResult(event.Content)
				case "file_diff":
					formatted = formatFileDiffResult(event.Content)
				case "ssh_execute", "ssh_file_upload", "ssh_file_download", "ssh_list_dir":
					formatted = formatSSHResult(event.Content, lastToolName)
				case "todo_add", "todo_list", "todo_update", "todo_delete", "todo_batch_add", "todo_batch_delete", "todo_clear":
					formatted = formatTodoResult(event.Content, lastToolName)
				}

				if formatted != "" {
					fmt.Print(formatted)
				} else {
					printPlainResult(event.Content)
				}
			}
			fmt.Println()

		case "tool_result_chunk":
			fmt.Printf("%s", event.Content)

		case "tool_calls_preparing":
			for _, tool := range event.ToolCalls {
				if tool.Function.Name != "" {
					fmt.Printf("\n\033[1;35m[%s] 准备调用工具: \033[1m%s\033[0m \033[90m(参数准备中...)\033[0m\n", event.AgentName, tool.Function.Name)
					lastToolName = tool.Function.Name
				}
			}

		case "tool_calls":
			fmt.Printf("\n\033[1;35m[%s] 调用工具:\033[0m\n", event.AgentName)
			for i, tool := range event.ToolCalls {
				fmt.Printf("  %d. \033[1m%s\033[0m\n", i+1, tool.Function.Name)
				if i == len(event.ToolCalls)-1 {
					lastToolName = tool.Function.Name
				}
				if tool.Function.Arguments != "" {
					args := truncateString(tool.Function.Arguments, 200)
					fmt.Printf("     参数: %s\n", args)
				}
			}
			fmt.Println()

		case "action":
			fmt.Printf("\n\033[1;34m▸ [%s] 动作: %s\033[0m\n", event.AgentName, event.ActionType)
			if event.Content != "" {
				fmt.Printf("  详情: %s\n", event.Content)
			}

		case "error":
			fmt.Printf("\n\033[1;31m✗ [%s] 错误:\033[0m\n", event.AgentName)
			fmt.Printf("  \033[31m%s\033[0m\n", event.Error)
			if event.RunPath != "" {
				fmt.Printf("  路径: %s\n", event.RunPath)
			}
			fmt.Println()

		default:
			fmt.Printf("\n\033[1;90m? 未知事件: %s\033[0m\n", event.Type)
			if event.AgentName != "" {
				fmt.Printf("  代理: %s\n", event.AgentName)
			}
			if event.Content != "" {
				fmt.Printf("  内容: %s\n", event.Content)
			}
		}
	}
}

func printPlainResult(content string) {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if line != "" {
			fmt.Printf("  │ %s\n", line)
		}
	}
}

func formatSearchResults(content string) string {
	var result struct {
		Message string `json:"message"`
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Summary string `json:"summary"`
		} `json:"results"`
	}

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return ""
	}

	var output strings.Builder

	if result.Message != "" {
		output.WriteString(fmt.Sprintf("  \033[32m✓ %s\033[0m\n\n", result.Message))
	}

	for i, r := range result.Results {
		output.WriteString(fmt.Sprintf("  \033[1;36m%d. %s\033[0m\n", i+1, r.Title))

		if r.URL != "" {
			output.WriteString(fmt.Sprintf("     \033[90mURL: %s\033[0m\n", r.URL))
		}

		if r.Summary != "" {
			summary := strings.ReplaceAll(r.Summary, "\n", " ")
			summary = truncateString(summary, 120)
			output.WriteString(fmt.Sprintf("     %s\n", summary))
		}

		if i < len(result.Results)-1 {
			output.WriteString("\n")
		}
	}

	return output.String()
}

// formatCommandResult 格式化命令执行结果
func formatCommandResult(content string) string {
	var result struct {
		Output       string `json:"output"`
		ExitCode     int    `json:"exit_code"`
		ErrorMessage string `json:"error_message"`
	}

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return ""
	}

	var output strings.Builder

	if result.ExitCode == 0 {
		output.WriteString("  \033[32m✓ 执行成功\033[0m (退出码: 0)\n\n")
	} else {
		output.WriteString(fmt.Sprintf("  \033[31m✗ 执行失败\033[0m (退出码: %d)\n\n", result.ExitCode))
	}

	if result.Output != "" {
		output.WriteString("  \033[1m输出:\033[0m\n")
		lines := strings.Split(result.Output, "\n")
		for _, line := range lines {
			if line != "" {
				output.WriteString(fmt.Sprintf("  │ %s\n", line))
			}
		}
		output.WriteString("\n")
	}

	if result.ErrorMessage != "" {
		output.WriteString(fmt.Sprintf("  \033[31m错误: %s\033[0m\n", result.ErrorMessage))
	}

	return output.String()
}

func formatFileOpResult(content string) string {
	var result struct {
		Success      bool     `json:"success"`
		Message      string   `json:"message"`
		FilePath     string   `json:"file_path"`
		Content      string   `json:"content"`
		Files        []string `json:"files"`
		Size         int64    `json:"size"`
		ErrorMessage string   `json:"error_message"`
	}

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return ""
	}

	result.Success = result.ErrorMessage == ""

	var output strings.Builder

	if result.Success {
		output.WriteString("  \033[32m✓ 操作成功\033[0m\n")
	} else {
		output.WriteString("  \033[31m✗ 操作失败\033[0m\n")
	}

	if result.Message != "" {
		output.WriteString(fmt.Sprintf("  %s\n", result.Message))
	}

	if result.FilePath != "" {
		output.WriteString(fmt.Sprintf("  \033[90m路径: %s\033[0m\n", result.FilePath))
	}

	if result.Size > 0 {
		output.WriteString(fmt.Sprintf("  大小: %s\n", formatFileSize(result.Size)))
	}

	if len(result.Files) > 0 {
		output.WriteString("\n  \033[1m文件列表:\033[0m\n")
		for i, file := range result.Files {
			if i < 20 { // 限制显示数量
				output.WriteString(fmt.Sprintf("  │ %s\n", file))
			} else if i == 20 {
				output.WriteString(fmt.Sprintf("  │ ... 还有 %d 个文件\n", len(result.Files)-20))
				break
			}
		}
	}

	if result.Content != "" {
		output.WriteString("\n  \033[1m内容:\033[0m\n")
		lines := strings.Split(result.Content, "\n")
		for i, line := range lines {
			if i < 30 {
				output.WriteString(fmt.Sprintf("  %3d │ %s\n", i+1, line))
			} else if i == 30 {
				output.WriteString(fmt.Sprintf("  ... 还有 %d 行\n", len(lines)-30))
				break
			}
		}
	}

	// 显示错误信息
	if result.ErrorMessage != "" {
		output.WriteString(fmt.Sprintf("  \033[31m错误: %s\033[0m\n", result.ErrorMessage))
	}

	return output.String()
}

func formatSSHResult(content string, toolName string) string {
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return ""
	}

	var output strings.Builder

	if execTime, ok := result["execution_time"].(string); ok && execTime != "" {
		output.WriteString(fmt.Sprintf("  执行时间: %s\n", execTime))
	}

	if errMsg, ok := result["error_message"].(string); ok && errMsg != "" {
		output.WriteString(fmt.Sprintf("  \033[31m✗ %s\033[0m\n", errMsg))
		return output.String()
	}

	output.WriteString("  \033[32m✓ 操作成功\033[0m\n\n")

	switch toolName {
	case "ssh_execute":
		if out, ok := result["output"].(string); ok && out != "" {
			output.WriteString("  \033[1m输出:\033[0m\n")
			lines := strings.Split(out, "\n")
			for _, line := range lines {
				if line != "" {
					output.WriteString(fmt.Sprintf("  │ %s\n", line))
				}
			}
		}

	case "ssh_file_upload", "ssh_file_download":
		if msg, ok := result["message"].(string); ok {
			output.WriteString(fmt.Sprintf("  %s\n", msg))
		}
		if size, ok := result["bytes_transferred"].(float64); ok {
			output.WriteString(fmt.Sprintf("  传输大小: %s\n", formatFileSize(int64(size))))
		}

	case "ssh_list_dir":
		if files, ok := result["files"].([]interface{}); ok && len(files) > 0 {
			output.WriteString("  \033[1m文件列表:\033[0m\n")
			for i, f := range files {
				if i < 20 {
					output.WriteString(fmt.Sprintf("  │ %v\n", f))
				} else if i == 20 {
					output.WriteString(fmt.Sprintf("  │ ... 还有 %d 个文件\n", len(files)-20))
					break
				}
			}
		}
	}

	return output.String()
}

func formatFileSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

func formatTodoResult(content string, toolName string) string {
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return ""
	}

	var output strings.Builder

	success, _ := result["success"].(bool)
	errorMsg, _ := result["error_message"].(string)

	if !success || errorMsg != "" {
		output.WriteString(fmt.Sprintf("  \033[31m✗ 操作失败: %s\033[0m\n", errorMsg))
		return output.String()
	}

	output.WriteString("  \033[32m✓ 操作成功\033[0m\n")

	if msg, ok := result["message"].(string); ok && msg != "" {
		output.WriteString(fmt.Sprintf("  %s\n", msg))
	}

	// 按工具类型格式化
	switch toolName {
	case "todo_add", "todo_update":
		if todoData, ok := result["todo"].(map[string]interface{}); ok {
			output.WriteString("\n")
			output.WriteString(formatSingleTodo(todoData))
		}

	case "todo_batch_add":
		if todosData, ok := result["added_todos"].([]interface{}); ok {
			addedCount, _ := result["added_count"].(float64)
			output.WriteString(fmt.Sprintf("\n  \033[1m已添加 %d 个待办事项:\033[0m\n\n", int(addedCount)))

			for i, todoItem := range todosData {
				if todoMap, ok := todoItem.(map[string]interface{}); ok {
					output.WriteString(formatSingleTodo(todoMap))
					if i < len(todosData)-1 {
						output.WriteString("  \033[90m────────────────────────────────────────\033[0m\n")
					}
				}
			}
		}

	case "todo_list":
		if todosData, ok := result["todos"].([]interface{}); ok {
			totalCount, _ := result["total_count"].(float64)
			output.WriteString(fmt.Sprintf("\n  \033[1m共 %d 个待办事项:\033[0m\n\n", int(totalCount)))

			if len(todosData) == 0 {
				output.WriteString("  \033[90m（暂无待办事项）\033[0m\n")
			} else {
				for i, todoItem := range todosData {
					if todoMap, ok := todoItem.(map[string]interface{}); ok {
						output.WriteString(formatSingleTodo(todoMap))
						// 分隔线
						if i < len(todosData)-1 {
							output.WriteString("  \033[90m────────────────────────────────────────\033[0m\n")
						}
					}
				}
			}
		}

	case "todo_delete":
		// no additional output

	case "todo_batch_delete":
		if deletedCount, ok := result["deleted_count"].(float64); ok {
			output.WriteString(fmt.Sprintf("\n  已删除 %d 个待办事项\n", int(deletedCount)))
		}
		if notFoundIDs, ok := result["not_found_ids"].([]interface{}); ok && len(notFoundIDs) > 0 {
			output.WriteString(fmt.Sprintf("  \033[33m注意: %d 个 ID 未找到\033[0m\n", len(notFoundIDs)))
			if len(notFoundIDs) <= 5 {
				for _, id := range notFoundIDs {
					if idStr, ok := id.(string); ok {
						output.WriteString(fmt.Sprintf("    - %s\n", idStr))
					}
				}
			}
		}

	case "todo_clear":
		if clearedCount, ok := result["cleared_count"].(float64); ok {
			output.WriteString(fmt.Sprintf("\n  已清空 %d 个待办事项\n", int(clearedCount)))
		}
	}

	return output.String()
}

func formatSingleTodo(todo map[string]interface{}) string {
	var output strings.Builder

	id, _ := todo["id"].(string)
	title, _ := todo["title"].(string)
	description, _ := todo["description"].(string)
	status, _ := todo["status"].(string)
	priority, _ := todo["priority"].(string)

	statusIcon := "○"
	statusColor := "\033[90m"
	statusText := status

	switch status {
	case "pending":
		statusIcon = "○"
		statusColor = "\033[90m"
		statusText = "待处理"
	case "in_progress":
		statusIcon = "◐"
		statusColor = "\033[36m"
		statusText = "进行中"
	case "completed":
		statusIcon = "●"
		statusColor = "\033[32m"
		statusText = "已完成"
	case "cancelled":
		statusIcon = "✕"
		statusColor = "\033[31m"
		statusText = "已取消"
	}

	priorityColor := "\033[0m"
	priorityText := priority

	switch priority {
	case "low":
		priorityColor = "\033[90m"
		priorityText = "低"
	case "medium":
		priorityColor = "\033[33m"
		priorityText = "中"
	case "high":
		priorityColor = "\033[35m"
		priorityText = "高"
	case "urgent":
		priorityColor = "\033[31m"
		priorityText = "紧急"
	}

	// 标题行
	output.WriteString(fmt.Sprintf("  %s%s\033[0m \033[1m%s\033[0m", statusColor, statusIcon, title))
	if priority != "" {
		output.WriteString(fmt.Sprintf(" %s[%s]\033[0m", priorityColor, priorityText))
	}
	output.WriteString("\n")

	output.WriteString(fmt.Sprintf("  │ 状态: %s%s\033[0m\n", statusColor, statusText))

	if id != "" {
		output.WriteString(fmt.Sprintf("  │ \033[90mID: %s\033[0m\n", truncateString(id, 30)))
	}

	if description != "" {
		output.WriteString(fmt.Sprintf("  │ 描述: %s\n", description))
	}

	if createdAt, ok := todo["created_at"].(string); ok && createdAt != "" {
		output.WriteString(fmt.Sprintf("  │ \033[90m创建时间: %s\033[0m\n", formatTime(createdAt)))
	}
	if completedAt, ok := todo["completed_at"].(string); ok && completedAt != "" {
		output.WriteString(fmt.Sprintf("  │ \033[90m完成时间: %s\033[0m\n", formatTime(completedAt)))
	}

	output.WriteString("\n")

	return output.String()
}

func formatTime(timeStr string) string {
	t, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		return timeStr
	}
	return t.Format("2006-01-02 15:04:05")
}

func formatFilePatchResult(content string) string {
	var result struct {
		Message string `json:"message"`
		Results []struct {
			Path    string `json:"path"`
			Success bool   `json:"success"`
			Error   string `json:"error"`
		} `json:"results"`
		TotalFiles   int    `json:"total_files"`
		Succeeded    int    `json:"succeeded"`
		Failed       int    `json:"failed"`
		ErrorMessage string `json:"error_message"`
	}

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return ""
	}

	var output strings.Builder

	if result.ErrorMessage != "" {
		output.WriteString(fmt.Sprintf("  \033[31m✗ %s\033[0m\n", result.ErrorMessage))
		return output.String()
	}

	if result.Failed == 0 {
		output.WriteString(fmt.Sprintf("  \033[32m✓ %s\033[0m\n", result.Message))
	} else {
		output.WriteString(fmt.Sprintf("  \033[33m⚠ %s\033[0m\n", result.Message))
	}

	for _, r := range result.Results {
		if r.Success {
			output.WriteString(fmt.Sprintf("  │ \033[32m✓\033[0m %s\n", r.Path))
		} else {
			output.WriteString(fmt.Sprintf("  │ \033[31m✗\033[0m %s: %s\n", r.Path, r.Error))
		}
	}

	return output.String()
}

func formatFileDiffResult(content string) string {
	var result struct {
		Diff         string `json:"diff"`
		Insertions   int    `json:"insertions"`
		Deletions    int    `json:"deletions"`
		ErrorMessage string `json:"error_message"`
	}

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return ""
	}

	var output strings.Builder

	if result.ErrorMessage != "" {
		output.WriteString(fmt.Sprintf("  \033[31m✗ %s\033[0m\n", result.ErrorMessage))
		return output.String()
	}

	if result.Diff == "" {
		output.WriteString("  \033[32m✓ 文件无变更\033[0m\n")
		return output.String()
	}

	var statParts []string
	if result.Insertions > 0 {
		statParts = append(statParts, fmt.Sprintf("\033[32m+%d\033[0m", result.Insertions))
	}
	if result.Deletions > 0 {
		statParts = append(statParts, fmt.Sprintf("\033[31m-%d\033[0m", result.Deletions))
	}
	output.WriteString(fmt.Sprintf("  变更统计: %s\n\n", strings.Join(statParts, " ")))

	lines := strings.Split(result.Diff, "\n")
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++"):
			output.WriteString(fmt.Sprintf("  \033[1m%s\033[0m\n", line))
		case strings.HasPrefix(line, "@@"):
			output.WriteString(fmt.Sprintf("  \033[36m%s\033[0m\n", line))
		case strings.HasPrefix(line, "+"):
			output.WriteString(fmt.Sprintf("  \033[32m%s\033[0m\n", line))
		case strings.HasPrefix(line, "-"):
			output.WriteString(fmt.Sprintf("  \033[31m%s\033[0m\n", line))
		case line != "":
			output.WriteString(fmt.Sprintf("  %s\n", line))
		}
	}

	return output.String()
}
