package eventview

import (
	"encoding/json"

	"fmt"

	"strings"
	"time"
)

func printPlainResult(content string) {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if i >= 30 {
			fmt.Printf("  │ ... 还有 %d 行\n", len(lines)-30)
			break
		}
		if line != "" {
			fmt.Printf("  │ %s\n", truncateString(line, 200))
		}
	}
}

func formatToolResultForPrint(toolName, content string) string {
	switch toolName {
	case "search":
		return formatSearchResults(content)
	case "execute":
		return formatCommandResult(content)
	case "file_read", "file_write", "file_edit", "file_list", "grep":
		return formatFileOpResult(content)
	case "file_patch":
		return formatFilePatchResult(content)
	case "ssh_execute", "ssh_upload", "ssh_download", "ssh_list_dir":
		return formatSSHResult(content, toolName)
	case "todo_add", "todo_list", "todo_update", "todo_delete", "todo_batch_add", "todo_batch_delete", "todo_clear":
		return formatTodoResult(content, toolName)
	case "schedule_add", "schedule_list", "schedule_cancel", "schedule_delete":
		return formatSchedulerResult(content, toolName)
	case "dispatch_tasks":
		return formatDispatchResult(content)
	default:
		return ""
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
		fmt.Fprintf(&output, "  \033[32m✓ %s\033[0m\n\n", result.Message)
	}

	for i, r := range result.Results {
		if i >= 5 {
			fmt.Fprintf(&output, "  \033[90m... 还有 %d 条结果\033[0m\n", len(result.Results)-5)
			break
		}
		fmt.Fprintf(&output, "  \033[1;36m%d. %s\033[0m\n", i+1, r.Title)

		if r.URL != "" {
			fmt.Fprintf(&output, "     \033[90mURL: %s\033[0m\n", truncateString(r.URL, 80))
		}

		if r.Summary != "" {
			summary := strings.ReplaceAll(r.Summary, "\n", " ")
			summary = truncateString(summary, 120)
			fmt.Fprintf(&output, "     %s\n", summary)
		}

		if i < min(len(result.Results)-1, 4) {
			output.WriteString("\n")
		}
	}

	return output.String()
}

func formatCommandResult(content string) string {
	var result struct {
		Stdout         string `json:"stdout"`
		Stderr         string `json:"stderr"`
		ExitCode       int    `json:"exit_code"`
		ExecutionTime  string `json:"execution_time"`
		SecurityLevel  string `json:"security_level"`
		WarningMessage string `json:"warning_message"`
		ErrorMessage   string `json:"error_message"`
	}

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return ""
	}

	var output strings.Builder

	if result.ErrorMessage != "" {
		fmt.Fprintf(&output, "  \033[31m✗ %s\033[0m\n", result.ErrorMessage)
		return output.String()
	}

	if result.ExitCode == 0 {
		fmt.Fprintf(&output, "  \033[32m✓ 执行成功\033[0m (退出码: 0, 耗时: %s)\n", result.ExecutionTime)
	} else {
		fmt.Fprintf(&output, "  \033[31m✗ 执行失败\033[0m (退出码: %d, 耗时: %s)\n", result.ExitCode, result.ExecutionTime)
	}

	if result.WarningMessage != "" {
		fmt.Fprintf(&output, "  \033[33m⚠ %s\033[0m\n", result.WarningMessage)
	}

	if result.Stdout != "" {
		output.WriteString("\n")
		lines := strings.Split(result.Stdout, "\n")
		for i, line := range lines {
			if i >= 30 {
				fmt.Fprintf(&output, "  │ ... 还有 %d 行\n", len(lines)-30)
				break
			}
			if line != "" {
				fmt.Fprintf(&output, "  │ %s\n", truncateString(line, 200))
			}
		}
	}

	if result.Stderr != "" {
		output.WriteString("\n  \033[31m标准错误:\033[0m\n")
		lines := strings.Split(result.Stderr, "\n")
		for i, line := range lines {
			if i >= 20 {
				fmt.Fprintf(&output, "  │ ... 还有 %d 行\n", len(lines)-20)
				break
			}
			if line != "" {
				fmt.Fprintf(&output, "  │ %s\n", truncateString(line, 200))
			}
		}
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
		fmt.Fprintf(&output, "  %s\n", result.Message)
	}

	if result.FilePath != "" {
		fmt.Fprintf(&output, "  \033[90m路径: %s\033[0m\n", result.FilePath)
	}

	if result.Size > 0 {
		fmt.Fprintf(&output, "  大小: %s\n", formatFileSize(result.Size))
	}

	if len(result.Files) > 0 {
		output.WriteString("\n  \033[1m文件列表:\033[0m\n")
		for i, file := range result.Files {
			if i < 20 {
				fmt.Fprintf(&output, "  │ %s\n", file)
			} else if i == 20 {
				fmt.Fprintf(&output, "  │ ... 还有 %d 个文件\n", len(result.Files)-20)
				break
			}
		}
	}

	if result.Content != "" {
		output.WriteString("\n  \033[1m内容:\033[0m\n")
		lines := strings.Split(result.Content, "\n")
		for i, line := range lines {
			if i < 30 {
				fmt.Fprintf(&output, "  %3d │ %s\n", i+1, line)
			} else if i == 30 {
				fmt.Fprintf(&output, "  ... 还有 %d 行\n", len(lines)-30)
				break
			}
		}
	}

	if result.ErrorMessage != "" {
		fmt.Fprintf(&output, "  \033[31m错误: %s\033[0m\n", result.ErrorMessage)
	}

	return output.String()
}

func formatSSHResult(content string, toolName string) string {
	var result map[string]any
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return ""
	}

	var output strings.Builder

	if execTime, ok := result["execution_time"].(string); ok && execTime != "" {
		fmt.Fprintf(&output, "  执行时间: %s\n", execTime)
	}

	if errMsg, ok := result["error_message"].(string); ok && errMsg != "" {
		fmt.Fprintf(&output, "  \033[31m✗ %s\033[0m\n", errMsg)
		return output.String()
	}

	output.WriteString("  \033[32m✓ 操作成功\033[0m\n\n")

	switch toolName {
	case "ssh_execute":
		if out, ok := result["output"].(string); ok && out != "" {
			output.WriteString("  \033[1m输出:\033[0m\n")
			lines := strings.Split(out, "\n")
			for i, line := range lines {
				if i >= 30 {
					fmt.Fprintf(&output, "  │ ... 还有 %d 行\n", len(lines)-30)
					break
				}
				if line != "" {
					fmt.Fprintf(&output, "  │ %s\n", truncateString(line, 200))
				}
			}
		}

	case "ssh_upload", "ssh_download":
		if msg, ok := result["message"].(string); ok {
			fmt.Fprintf(&output, "  %s\n", msg)
		}
		if size, ok := result["bytes_transferred"].(float64); ok {
			fmt.Fprintf(&output, "  传输大小: %s\n", formatFileSize(int64(size)))
		}

	case "ssh_list_dir":
		if files, ok := result["files"].([]any); ok && len(files) > 0 {
			output.WriteString("  \033[1m文件列表:\033[0m\n")
			for i, f := range files {
				if i < 20 {
					fmt.Fprintf(&output, "  │ %v\n", f)
				} else if i == 20 {
					fmt.Fprintf(&output, "  │ ... 还有 %d 个文件\n", len(files)-20)
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
	var result map[string]any
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return ""
	}

	var output strings.Builder

	success, _ := result["success"].(bool)
	errorMsg, _ := result["error_message"].(string)

	if !success || errorMsg != "" {
		fmt.Fprintf(&output, "  \033[31m✗ 操作失败: %s\033[0m\n", errorMsg)
		return output.String()
	}

	output.WriteString("  \033[32m✓ 操作成功\033[0m\n")

	if msg, ok := result["message"].(string); ok && msg != "" {
		fmt.Fprintf(&output, "  %s\n", msg)
	}

	switch toolName {
	case "todo_add", "todo_update":
		if todoData, ok := result["todo"].(map[string]any); ok {
			output.WriteString("\n")
			output.WriteString(formatSingleTodo(todoData))
		}

	case "todo_batch_add":
		if todosData, ok := result["added_todos"].([]any); ok {
			addedCount, _ := result["added_count"].(float64)
			fmt.Fprintf(&output, "\n  \033[1m已添加 %d 个待办事项:\033[0m\n\n", int(addedCount))

			limit := min(len(todosData), 10)
			for i, todoItem := range todosData[:limit] {
				if todoMap, ok := todoItem.(map[string]any); ok {
					output.WriteString(formatSingleTodo(todoMap))
					if i < limit-1 {
						output.WriteString("  \033[90m────────────────────────────────────────\033[0m\n")
					}
				}
			}
			if len(todosData) > 10 {
				fmt.Fprintf(&output, "  \033[90m... 还有 %d 个待办事项\033[0m\n", len(todosData)-10)
			}
		}

	case "todo_list":
		if todosData, ok := result["todos"].([]any); ok {
			totalCount, _ := result["total_count"].(float64)
			fmt.Fprintf(&output, "\n  \033[1m共 %d 个待办事项:\033[0m\n\n", int(totalCount))

			if len(todosData) == 0 {
				output.WriteString("  \033[90m（暂无待办事项）\033[0m\n")
			} else {
				limit := min(len(todosData), 10)
				for i, todoItem := range todosData[:limit] {
					if todoMap, ok := todoItem.(map[string]any); ok {
						output.WriteString(formatSingleTodo(todoMap))
						if i < limit-1 {
							output.WriteString("  \033[90m────────────────────────────────────────\033[0m\n")
						}
					}
				}
				if len(todosData) > 10 {
					fmt.Fprintf(&output, "  \033[90m... 还有 %d 个待办事项\033[0m\n", len(todosData)-10)
				}
			}
		}

	case "todo_delete":

	case "todo_batch_delete":
		if deletedCount, ok := result["deleted_count"].(float64); ok {
			fmt.Fprintf(&output, "\n  已删除 %d 个待办事项\n", int(deletedCount))
		}
		if notFoundIDs, ok := result["not_found_ids"].([]any); ok && len(notFoundIDs) > 0 {
			fmt.Fprintf(&output, "  \033[33m注意: %d 个 ID 未找到\033[0m\n", len(notFoundIDs))
			if len(notFoundIDs) <= 5 {
				for _, id := range notFoundIDs {
					if idStr, ok := id.(string); ok {
						fmt.Fprintf(&output, "    - %s\n", idStr)
					}
				}
			}
		}

	case "todo_clear":
		if clearedCount, ok := result["cleared_count"].(float64); ok {
			fmt.Fprintf(&output, "\n  已清空 %d 个待办事项\n", int(clearedCount))
		}
	}

	return output.String()
}

func formatSingleTodo(todo map[string]any) string {
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

	fmt.Fprintf(&output, "  %s%s\033[0m \033[1m%s\033[0m", statusColor, statusIcon, title)
	if priority != "" {
		fmt.Fprintf(&output, " %s[%s]\033[0m", priorityColor, priorityText)
	}
	output.WriteString("\n")

	fmt.Fprintf(&output, "  │ 状态: %s%s\033[0m\n", statusColor, statusText)

	if id != "" {
		fmt.Fprintf(&output, "  │ \033[90mID: %s\033[0m\n", truncateString(id, 30))
	}

	if description != "" {
		fmt.Fprintf(&output, "  │ 描述: %s\n", truncateString(description, 120))
	}

	if createdAt, ok := todo["created_at"].(string); ok && createdAt != "" {
		fmt.Fprintf(&output, "  │ \033[90m创建时间: %s\033[0m\n", formatTime(createdAt))
	}
	if completedAt, ok := todo["completed_at"].(string); ok && completedAt != "" {
		fmt.Fprintf(&output, "  │ \033[90m完成时间: %s\033[0m\n", formatTime(completedAt))
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
		fmt.Fprintf(&output, "  \033[31m✗ %s\033[0m\n", result.ErrorMessage)
		return output.String()
	}

	if result.Failed == 0 {
		fmt.Fprintf(&output, "  \033[32m✓ %s\033[0m\n", result.Message)
	} else {
		fmt.Fprintf(&output, "  \033[33m⚠ %s\033[0m\n", result.Message)
	}

	for i, r := range result.Results {
		if i >= 20 {
			fmt.Fprintf(&output, "  │ ... 还有 %d 个文件\n", len(result.Results)-20)
			break
		}
		if r.Success {
			fmt.Fprintf(&output, "  │ \033[32m✓\033[0m %s\n", r.Path)
		} else {
			fmt.Fprintf(&output, "  │ \033[31m✗\033[0m %s: %s\n", r.Path, r.Error)
		}
	}

	return output.String()
}
