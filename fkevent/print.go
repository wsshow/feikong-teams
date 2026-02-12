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
	lastToolName := "" // 记录最后调用的工具名称
	return func(event Event) {
		switch event.Type {
		case "stream_chunk":
			// 流式输出内容块，显示代理名称和路径
			if agentName != event.AgentName {
				agentName = event.AgentName
				fmt.Printf("\n\033[1;36m╭─ [%s] %s\033[0m\n", agentName, event.RunPath)
				fmt.Printf("\033[1;36m╰─▶\033[0m ")
			}
			fmt.Printf("%s", event.Content)

		case "message":
			// 完整消息输出
			if event.Content != "" {
				fmt.Printf("\n\033[1;32m✓ [%s] 消息:\033[0m %s\n", event.AgentName, event.Content)
			}

		case "tool_result":
			// 工具执行结果
			fmt.Printf("\n\033[1;33m⚙ [%s] 工具结果:\033[0m\n", event.AgentName)
			if event.Content != "" {
				// 根据工具名称选择展示格式
				var formatted string
				switch lastToolName {
				case "duckduckgo_search":
					formatted = formatSearchResults(event.Content)
				case "execute_command", "command_execute":
					formatted = formatCommandResult(event.Content)
				case "file_read", "file_edit", "file_list", "file_create", "file_delete", "dir_create", "dir_delete", "file_search":
					formatted = formatFileOpResult(event.Content)
				case "ssh_execute", "ssh_file_upload", "ssh_file_download", "ssh_list_dir":
					formatted = formatSSHResult(event.Content, lastToolName)
				case "todo_add", "todo_list", "todo_update", "todo_delete", "todo_batch_add", "todo_batch_delete", "todo_clear":
					formatted = formatTodoResult(event.Content, lastToolName)
				}

				if formatted != "" {
					fmt.Print(formatted)
				} else {
					// 使用普通格式
					printPlainResult(event.Content)
				}
			}
			fmt.Println()

		case "tool_result_chunk":
			// 工具结果流式输出块
			fmt.Printf("%s", event.Content)

		case "tool_calls_preparing":
			// 工具调用准备中（参数收集阶段）
			for _, tool := range event.ToolCalls {
				if tool.Function.Name != "" {
					fmt.Printf("\n\033[1;35m[%s] 准备调用工具: \033[1m%s\033[0m \033[90m(参数准备中...)\033[0m\n", event.AgentName, tool.Function.Name)
					// 记录工具名称
					lastToolName = tool.Function.Name
				}
			}

		case "tool_calls":
			// 工具调用信息（参数已准备完成）
			fmt.Printf("\n\033[1;35m[%s] 调用工具:\033[0m\n", event.AgentName)
			for i, tool := range event.ToolCalls {
				fmt.Printf("  %d. \033[1m%s\033[0m\n", i+1, tool.Function.Name)
				// 记录最后一个工具名称
				if i == len(event.ToolCalls)-1 {
					lastToolName = tool.Function.Name
				}
				if tool.Function.Arguments != "" {
					// 显示参数（截断过长的参数）
					args := tool.Function.Arguments
					args = truncateString(args, 200)
					fmt.Printf("     参数: %s\n", args)
				}
			}
			fmt.Println()

		case "action":
			// 动作类型事件
			fmt.Printf("\n\033[1;34m▸ [%s] 动作: %s\033[0m\n", event.AgentName, event.ActionType)
			if event.Content != "" {
				fmt.Printf("  详情: %s\n", event.Content)
			}

		case "error":
			// 错误信息，红色高亮显示
			fmt.Printf("\n\033[1;31m✗ [%s] 错误:\033[0m\n", event.AgentName)
			fmt.Printf("  \033[31m%s\033[0m\n", event.Error)
			if event.RunPath != "" {
				fmt.Printf("  路径: %s\n", event.RunPath)
			}
			fmt.Println()

		default:
			// 未知事件类型
			fmt.Printf("\n\033[1;90m? 未知事件类型: %s\033[0m\n", event.Type)
			if event.AgentName != "" {
				fmt.Printf("  代理: %s\n", event.AgentName)
			}
			if event.Content != "" {
				fmt.Printf("  内容: %s\n", event.Content)
			}
		}
	}
}

// printPlainResult 打印普通文本结果
func printPlainResult(content string) {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if line != "" {
			fmt.Printf("  │ %s\n", line)
		}
	}
}

// formatSearchResults 格式化搜索结果以便美观显示
func formatSearchResults(content string) string {
	// 尝试解析为搜索结果JSON
	var result struct {
		Message string `json:"message"`
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Summary string `json:"summary"`
		} `json:"results"`
	}

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return "" // 不是搜索结果格式，返回空字符串
	}

	var output strings.Builder

	// 显示消息
	if result.Message != "" {
		output.WriteString(fmt.Sprintf("  \033[32m✓ %s\033[0m\n\n", result.Message))
	}

	// 显示搜索结果
	for i, r := range result.Results {
		output.WriteString(fmt.Sprintf("  \033[1;36m%d. %s\033[0m\n", i+1, r.Title))

		// URL（灰色显示）
		if r.URL != "" {
			output.WriteString(fmt.Sprintf("     \033[90mURL: %s\033[0m\n", r.URL))
		}

		// 摘要（截断过长的内容）
		if r.Summary != "" {
			summary := r.Summary
			// 替换换行符为空格
			summary = strings.ReplaceAll(summary, "\n", " ")
			// 限制长度
			summary = truncateString(summary, 120)
			output.WriteString(fmt.Sprintf("     %s\n", summary))
		}

		// 结果之间添加空行
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

	// 显示退出码
	if result.ExitCode == 0 {
		output.WriteString("  \033[32m✓ 执行成功\033[0m (退出码: 0)\n\n")
	} else {
		output.WriteString(fmt.Sprintf("  \033[31m✗ 执行失败\033[0m (退出码: %d)\n\n", result.ExitCode))
	}

	// 显示输出
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

	// 显示错误信息
	if result.ErrorMessage != "" {
		output.WriteString(fmt.Sprintf("  \033[31m错误: %s\033[0m\n", result.ErrorMessage))
	}

	return output.String()
}

// formatFileOpResult 格式化文件操作结果
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

	// 显示操作状态
	if result.Success {
		output.WriteString("  \033[32m✓ 操作成功\033[0m\n")
	} else {
		output.WriteString("  \033[31m✗ 操作失败\033[0m\n")
	}

	// 显示消息
	if result.Message != "" {
		output.WriteString(fmt.Sprintf("  %s\n", result.Message))
	}

	// 显示文件路径
	if result.FilePath != "" {
		output.WriteString(fmt.Sprintf("  \033[90m路径: %s\033[0m\n", result.FilePath))
	}

	// 显示文件大小
	if result.Size > 0 {
		output.WriteString(fmt.Sprintf("  大小: %s\n", formatFileSize(result.Size)))
	}

	// 显示文件列表
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

	// 显示文件内容（截断）
	if result.Content != "" {
		output.WriteString("\n  \033[1m内容:\033[0m\n")
		lines := strings.Split(result.Content, "\n")
		for i, line := range lines {
			if i < 30 { // 限制显示行数
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

// formatSSHResult 格式化SSH操作结果
func formatSSHResult(content string, toolName string) string {
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return ""
	}

	var output strings.Builder

	// 显示执行时间（如果有）
	if execTime, ok := result["execution_time"].(string); ok && execTime != "" {
		output.WriteString(fmt.Sprintf("  执行时间: %s\n", execTime))
	}

	// 显示错误信息
	if errMsg, ok := result["error_message"].(string); ok && errMsg != "" {
		output.WriteString(fmt.Sprintf("  \033[31m✗ %s\033[0m\n", errMsg))
		return output.String()
	}

	output.WriteString("  \033[32m✓ 操作成功\033[0m\n\n")

	// 根据不同工具类型显示不同内容
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

// formatFileSize 格式化文件大小
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

// truncateString 安全地截断字符串，正确处理 Unicode 字符（如中文）
func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// formatTodoResult 格式化待办事项操作结果
func formatTodoResult(content string, toolName string) string {
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return ""
	}

	var output strings.Builder

	// 检查操作是否成功
	success, _ := result["success"].(bool)
	errorMsg, _ := result["error_message"].(string)

	if !success || errorMsg != "" {
		output.WriteString(fmt.Sprintf("  \033[31m✗ 操作失败: %s\033[0m\n", errorMsg))
		return output.String()
	}

	output.WriteString("  \033[32m✓ 操作成功\033[0m\n")

	// 显示消息
	if msg, ok := result["message"].(string); ok && msg != "" {
		output.WriteString(fmt.Sprintf("  %s\n", msg))
	}

	// 根据不同工具类型显示不同内容
	switch toolName {
	case "todo_add", "todo_update":
		// 显示单个待办事项
		if todoData, ok := result["todo"].(map[string]interface{}); ok {
			output.WriteString("\n")
			output.WriteString(formatSingleTodo(todoData))
		}

	case "todo_batch_add":
		// 显示批量添加的待办事项
		if todosData, ok := result["added_todos"].([]interface{}); ok {
			addedCount, _ := result["added_count"].(float64)
			output.WriteString(fmt.Sprintf("\n  \033[1m已添加 %d 个待办事项:\033[0m\n\n", int(addedCount)))

			for i, todoItem := range todosData {
				if todoMap, ok := todoItem.(map[string]interface{}); ok {
					output.WriteString(formatSingleTodo(todoMap))
					// 在每个待办事项之间添加分隔线
					if i < len(todosData)-1 {
						output.WriteString("  \033[90m────────────────────────────────────────\033[0m\n")
					}
				}
			}
		}

	case "todo_list":
		// 显示待办事项列表
		if todosData, ok := result["todos"].([]interface{}); ok {
			totalCount, _ := result["total_count"].(float64)
			output.WriteString(fmt.Sprintf("\n  \033[1m共 %d 个待办事项:\033[0m\n\n", int(totalCount)))

			if len(todosData) == 0 {
				output.WriteString("  \033[90m（暂无待办事项）\033[0m\n")
			} else {
				for i, todoItem := range todosData {
					if todoMap, ok := todoItem.(map[string]interface{}); ok {
						output.WriteString(formatSingleTodo(todoMap))
						// 在每个待办事项之间添加分隔线
						if i < len(todosData)-1 {
							output.WriteString("  \033[90m────────────────────────────────────────\033[0m\n")
						}
					}
				}
			}
		}

	case "todo_delete":
		// 删除操作不需要额外显示

	case "todo_batch_delete":
		// 显示批量删除的结果
		if deletedCount, ok := result["deleted_count"].(float64); ok {
			output.WriteString(fmt.Sprintf("\n  已删除 %d 个待办事项\n", int(deletedCount)))
		}
		// 如果有未找到的 ID，显示出来
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
		// 显示清空的结果
		if clearedCount, ok := result["cleared_count"].(float64); ok {
			output.WriteString(fmt.Sprintf("\n  已清空 %d 个待办事项\n", int(clearedCount)))
		}
	}

	return output.String()
}

// formatSingleTodo 格式化单个待办事项
func formatSingleTodo(todo map[string]interface{}) string {
	var output strings.Builder

	// 获取字段
	id, _ := todo["id"].(string)
	title, _ := todo["title"].(string)
	description, _ := todo["description"].(string)
	status, _ := todo["status"].(string)
	priority, _ := todo["priority"].(string)

	// 状态图标和颜色
	statusIcon := "○"
	statusColor := "\033[90m" // 灰色
	statusText := status

	switch status {
	case "pending":
		statusIcon = "○"
		statusColor = "\033[90m" // 灰色
		statusText = "待处理"
	case "in_progress":
		statusIcon = "◐"
		statusColor = "\033[36m" // 青色
		statusText = "进行中"
	case "completed":
		statusIcon = "●"
		statusColor = "\033[32m" // 绿色
		statusText = "已完成"
	case "cancelled":
		statusIcon = "✕"
		statusColor = "\033[31m" // 红色
		statusText = "已取消"
	}

	// 优先级颜色
	priorityColor := "\033[0m" // 默认
	priorityText := priority

	switch priority {
	case "low":
		priorityColor = "\033[90m" // 灰色
		priorityText = "低"
	case "medium":
		priorityColor = "\033[33m" // 黄色
		priorityText = "中"
	case "high":
		priorityColor = "\033[35m" // 紫色
		priorityText = "高"
	case "urgent":
		priorityColor = "\033[31m" // 红色
		priorityText = "紧急"
	}

	// 显示标题行（状态 + 标题 + 优先级）
	output.WriteString(fmt.Sprintf("  %s%s\033[0m \033[1m%s\033[0m", statusColor, statusIcon, title))
	if priority != "" {
		output.WriteString(fmt.Sprintf(" %s[%s]\033[0m", priorityColor, priorityText))
	}
	output.WriteString("\n")

	// 显示状态
	output.WriteString(fmt.Sprintf("  │ 状态: %s%s\033[0m\n", statusColor, statusText))

	// 显示 ID（灰色小字）
	if id != "" {
		output.WriteString(fmt.Sprintf("  │ \033[90mID: %s\033[0m\n", truncateString(id, 30)))
	}

	// 显示描述
	if description != "" {
		output.WriteString(fmt.Sprintf("  │ 描述: %s\n", description))
	}

	// 显示时间信息
	if createdAt, ok := todo["created_at"].(string); ok && createdAt != "" {
		output.WriteString(fmt.Sprintf("  │ \033[90m创建时间: %s\033[0m\n", formatTime(createdAt)))
	}
	if completedAt, ok := todo["completed_at"].(string); ok && completedAt != "" {
		output.WriteString(fmt.Sprintf("  │ \033[90m完成时间: %s\033[0m\n", formatTime(completedAt)))
	}

	output.WriteString("\n")

	return output.String()
}

// formatTime 格式化时间显示
func formatTime(timeStr string) string {
	// 尝试解析 ISO 8601 格式
	t, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		return timeStr
	}
	return t.Format("2006-01-02 15:04:05")
}
