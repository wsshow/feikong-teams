package fkevent

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"
)

var (
	PrintEvent      func(Event)
	FlushPrintEvent func() // 刷新流式缓冲
)

func init() {
	PrintEvent, FlushPrintEvent = newPrintEvent()
}

// streamBuf 累积流式文本并增量差分渲染
type streamBuf struct {
	buf          strings.Builder
	agent        string
	path         string
	lastRendered string
	lastRender   time.Time
	hasRendered  bool
}

const renderInterval = 100 * time.Millisecond

func (s *streamBuf) reset() {
	s.buf.Reset()
	s.agent = ""
	s.path = ""
	s.lastRendered = ""
	s.hasRendered = false
	s.lastRender = time.Time{}
}

func (s *streamBuf) addChunk(content string) {
	s.buf.WriteString(content)
	if !s.hasRendered || time.Since(s.lastRender) >= renderInterval {
		s.render()
	}
}

// render 对比新旧输出，只清除并重绘变化的尾部
func (s *streamBuf) render() {
	content := s.buf.String()
	if content == "" {
		return
	}
	rendered := RenderMarkdown(content)
	if rendered == "" || rendered == s.lastRendered {
		return
	}

	if !s.hasRendered {
		lipgloss.Print(rendered)
		fmt.Print("\n")
	} else {
		diffIdx := commonPrefixLen(s.lastRendered, rendered)
		// 回退到换行边界
		snapIdx := strings.LastIndex(s.lastRendered[:diffIdx], "\n")
		if snapIdx < 0 {
			snapIdx = 0
		} else {
			snapIdx++
		}
		oldTail := s.lastRendered[snapIdx:]
		clearLines := strings.Count(oldTail, "\n") + 1
		if clearLines > 0 {
			fmt.Printf("\033[%dF\033[J", clearLines)
		}
		lipgloss.Print(rendered[snapIdx:])
		fmt.Print("\n")
	}

	s.lastRendered = rendered
	s.lastRender = time.Now()
	s.hasRendered = true
}

func (s *streamBuf) flush() {
	if s.buf.Len() == 0 {
		s.reset()
		return
	}
	s.render()
	s.reset()
}

func commonPrefixLen(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// reasoningWriter 按终端宽度换行，每行补 │ 前缀
type reasoningWriter struct {
	col      int
	maxWidth int
}

const reasoningPrefix = "\033[1;36m│\033[0m  \033[3;90m"

func newReasoningWriter() *reasoningWriter {
	w := termWidth() - 4
	if w < 20 {
		w = 20
	}
	return &reasoningWriter{maxWidth: w}
}

func (rw *reasoningWriter) writeChunk(content string) {
	for _, r := range content {
		if r == '\n' {
			fmt.Printf("\033[0m\n%s", reasoningPrefix)
			rw.col = 0
			continue
		}
		cw := runewidth.RuneWidth(r)
		if rw.col+cw > rw.maxWidth {
			fmt.Printf("\033[0m\n%s", reasoningPrefix)
			rw.col = 0
		}
		fmt.Printf("%c", r)
		rw.col += cw
	}
}

func newPrintEvent() (func(Event), func()) {
	agentName := ""
	lastToolName := ""
	inReasoning := false
	var sb streamBuf
	var rw *reasoningWriter

	tryFlush := func() {
		if sb.buf.Len() > 0 {
			sb.flush()
		}
	}

	printFn := func(event Event) {
		switch event.Type {
		case EventReasoningChunk:
			tryFlush()
			if agentName != event.AgentName {
				agentName = event.AgentName
				fmt.Printf("\n\033[1;36m╭─ [%s] %s\033[0m\n", agentName, event.RunPath)
			}
			if !inReasoning {
				inReasoning = true
				rw = newReasoningWriter()
				fmt.Printf("%s\033[90m[思考] \033[0m%s", reasoningPrefix, "\033[3;90m")
				rw.col = 6 // "[思考] " 占 6 列
			}
			rw.writeChunk(event.Content)

		case EventStreamChunk:
			wasReasoning := inReasoning
			if inReasoning {
				inReasoning = false
				fmt.Printf("\033[0m\n")
			}
			if agentName != event.AgentName {
				tryFlush()
				agentName = event.AgentName
				fmt.Printf("\n\033[1;36m╭─ [%s] %s\033[0m\n", agentName, event.RunPath)
				fmt.Printf("\033[1;36m╰─▶\033[0m\n")
				sb.agent = agentName
				sb.path = event.RunPath
			} else if wasReasoning {
				fmt.Printf("\033[1;36m╰─▶\033[0m\n")
			}
			sb.addChunk(event.Content)

		case EventMessage:
			tryFlush()
			if inReasoning {
				inReasoning = false
				fmt.Printf("\033[0m\n")
			}
			if event.ReasoningContent != "" {
				fmt.Printf("\n\033[90m[%s] 思考:\033[0m \033[3;90m%s\033[0m\n", event.AgentName, event.ReasoningContent)
			}
			if event.Content != "" {
				fmt.Printf("\n\033[1;32m✓ [%s]\033[0m\n", event.AgentName)
				lipgloss.Println(RenderMarkdown(event.Content))
			}

		case EventToolResult:
			tryFlush()
			fmt.Printf("\n\033[1;33m⚙ [%s] 工具结果:\033[0m\n", event.AgentName)
			if event.Content != "" {
				var formatted string
				switch lastToolName {
				case "duckduckgo_search":
					formatted = formatSearchResults(event.Content)
				case "execute_command", "command_execute", "smart_execute", "execute":
					formatted = formatCommandResult(event.Content)
				case "file_read", "file_write", "file_edit", "file_list", "grep":
					formatted = formatFileOpResult(event.Content)
				case "file_patch":
					formatted = formatFilePatchResult(event.Content)
				case "ssh_execute", "ssh_file_upload", "ssh_file_download", "ssh_list_dir":
					formatted = formatSSHResult(event.Content, lastToolName)
				case "todo_add", "todo_list", "todo_update", "todo_delete", "todo_batch_add", "todo_batch_delete", "todo_clear":
					formatted = formatTodoResult(event.Content, lastToolName)
				case "schedule_add", "schedule_list", "schedule_cancel", "schedule_delete":
					formatted = formatSchedulerResult(event.Content, lastToolName)
				case "dispatch_tasks":
					formatted = formatDispatchResult(event.Content)
				}
				if formatted != "" {
					fmt.Print(formatted)
				} else {
					printPlainResult(event.Content)
				}
			}
			fmt.Println()

		case EventToolResultChunk:
			fmt.Printf("%s", event.Content)

		case EventToolCallsPreparing:
			tryFlush()
			if inReasoning {
				inReasoning = false
				fmt.Printf("\033[0m\n")
			}
			for _, tool := range event.ToolCalls {
				if tool.Function.Name != "" {
					fmt.Printf("\n\033[1;35m[%s] 准备调用工具: \033[1m%s\033[0m \033[90m(参数准备中...)\033[0m\n", event.AgentName, tool.Function.Name)
					lastToolName = tool.Function.Name
				}
			}

		case EventToolCalls:
			tryFlush()
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

		case EventAction:
			tryFlush()
			switch event.ActionType {
			case ActionContextCompressStart:
				fmt.Printf("\n\033[1;33m~ [%s] %s\033[0m", event.AgentName, event.Content)
			case ActionContextCompress:
				fmt.Printf("\n\033[1;33m✓ [%s] %s\033[0m\n", event.AgentName, event.Content)
			default:
				fmt.Printf("\n\033[1;34m▸ [%s] 动作: %s\033[0m\n", event.AgentName, event.ActionType)
				if event.Content != "" {
					fmt.Printf("  详情: %s\n", event.Content)
				}
			}

		case EventError:
			tryFlush()
			fmt.Printf("\n\033[1;31m✗ [%s] 错误:\033[0m\n", event.AgentName)
			fmt.Printf("  \033[31m%s\033[0m\n", event.Error)
			if event.RunPath != "" {
				fmt.Printf("  路径: %s\n", event.RunPath)
			}
			fmt.Println()

		case EventToolCallsArgsDelta:
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

	return printFn, func() { tryFlush() }
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
		output.WriteString(fmt.Sprintf("  \033[31m✗ %s\033[0m\n", result.ErrorMessage))
		return output.String()
	}

	if result.ExitCode == 0 {
		output.WriteString(fmt.Sprintf("  \033[32m✓ 执行成功\033[0m (退出码: 0, 耗时: %s)\n", result.ExecutionTime))
	} else {
		output.WriteString(fmt.Sprintf("  \033[31m✗ 执行失败\033[0m (退出码: %d, 耗时: %s)\n", result.ExitCode, result.ExecutionTime))
	}

	if result.WarningMessage != "" {
		output.WriteString(fmt.Sprintf("  \033[33m⚠ %s\033[0m\n", result.WarningMessage))
	}

	if result.Stdout != "" {
		output.WriteString("\n")
		lines := strings.Split(result.Stdout, "\n")
		for _, line := range lines {
			if line != "" {
				output.WriteString(fmt.Sprintf("  │ %s\n", line))
			}
		}
	}

	if result.Stderr != "" {
		output.WriteString("\n  \033[31m标准错误:\033[0m\n")
		lines := strings.Split(result.Stderr, "\n")
		for _, line := range lines {
			if line != "" {
				output.WriteString(fmt.Sprintf("  │ %s\n", line))
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
						if i < len(todosData)-1 {
							output.WriteString("  \033[90m────────────────────────────────────────\033[0m\n")
						}
					}
				}
			}
		}

	case "todo_delete":

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

// NewMarkdownCollector 创建事件 Markdown 收集器，供后台任务使用
func NewMarkdownCollector() (callback func(Event) error, getResult func() string) {
	var buf strings.Builder
	lastAgent := ""
	lastToolName := ""
	inStream := false

	flushStream := func() {
		if inStream {
			buf.WriteString("\n")
			inStream = false
		}
	}

	callback = func(event Event) error {
		switch event.Type {
		case EventStreamChunk:
			if lastAgent != event.AgentName {
				flushStream()
				lastAgent = event.AgentName
				buf.WriteString(fmt.Sprintf("\n\n**[%s]**\n\n", event.AgentName))
			}
			buf.WriteString(event.Content)
			inStream = true

		case EventMessage:
			if event.Content != "" {
				flushStream()
				if lastAgent != event.AgentName {
					lastAgent = event.AgentName
					buf.WriteString(fmt.Sprintf("\n\n**[%s]**\n\n", event.AgentName))
				}
				buf.WriteString(event.Content)
			}

		case EventToolCallsPreparing:
			for _, tc := range event.ToolCalls {
				if tc.Function.Name != "" {
					lastToolName = tc.Function.Name
				}
			}

		case EventToolCalls:
			flushStream()
			for i, tc := range event.ToolCalls {
				if i == len(event.ToolCalls)-1 {
					lastToolName = tc.Function.Name
				}
				args := truncateString(tc.Function.Arguments, 100)
				if args != "" {
					buf.WriteString(fmt.Sprintf("\n\n> **[%s]** 调用工具: `%s`\n> 参数: `%s`", event.AgentName, tc.Function.Name, args))
				} else {
					buf.WriteString(fmt.Sprintf("\n\n> **[%s]** 调用工具: `%s`", event.AgentName, tc.Function.Name))
				}
			}
			lastAgent = ""

		case EventToolResult:
			if event.Content != "" {
				if formatted := formatToolResultMarkdown(event.Content, lastToolName); formatted != "" {
					buf.WriteString(formatted)
				}
			}
			lastAgent = ""

		case EventAction:
			switch event.ActionType {
			case ActionTransfer:
				flushStream()
				buf.WriteString(fmt.Sprintf("\n\n> **[%s]** → %s", event.AgentName, event.Content))
				lastAgent = ""
			case ActionContextCompressStart, ActionContextCompress:
				// 跳过上下文压缩提示
			}

		case EventError:
			flushStream()
			buf.WriteString(fmt.Sprintf("\n\n**错误 [%s]**: %s", event.AgentName, event.Error))
		}
		return nil
	}

	getResult = func() string {
		if inStream {
			buf.WriteString("\n")
		}
		return strings.TrimSpace(buf.String())
	}

	return
}

func formatToolResultMarkdown(content string, toolName string) string {
	switch toolName {
	case "duckduckgo_search":
		return formatSearchResultMarkdown(content)
	case "dispatch_tasks":
		return formatDispatchResultMarkdown(content)
	default:
		var raw struct {
			Message      string `json:"message"`
			ErrorMessage string `json:"error_message"`
		}
		if err := json.Unmarshal([]byte(content), &raw); err == nil {
			if raw.ErrorMessage != "" {
				return fmt.Sprintf("\n\n> ✗ %s", raw.ErrorMessage)
			}
			if raw.Message != "" {
				return fmt.Sprintf("\n\n> ✓ %s", raw.Message)
			}
		}
		return ""
	}
}

func formatSearchResultMarkdown(content string) string {
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
		output.WriteString(fmt.Sprintf("\n\n> ✓ %s", result.Message))
	}
	for i, r := range result.Results {
		if i >= 5 {
			output.WriteString(fmt.Sprintf("\n>\n> _...还有 %d 条结果_", len(result.Results)-5))
			break
		}
		output.WriteString(fmt.Sprintf("\n>\n> %d. **%s**", i+1, r.Title))
		if r.URL != "" {
			output.WriteString(fmt.Sprintf(" <%s>", r.URL))
		}
		if r.Summary != "" {
			summary := truncateString(strings.ReplaceAll(r.Summary, "\n", " "), 100)
			output.WriteString(fmt.Sprintf("\n>    %s", summary))
		}
	}
	return output.String()
}

func formatSchedulerResult(content string, toolName string) string {
	var output strings.Builder

	switch toolName {
	case "schedule_add":
		var result struct {
			Success      bool   `json:"success"`
			Message      string `json:"message"`
			ErrorMessage string `json:"error_message"`
			Task         *struct {
				ID        string `json:"id"`
				Task      string `json:"task"`
				CronExpr  string `json:"cron_expr"`
				OneTime   bool   `json:"one_time"`
				NextRunAt string `json:"next_run_at"`
				Status    string `json:"status"`
			} `json:"task"`
		}
		if err := json.Unmarshal([]byte(content), &result); err != nil {
			return ""
		}
		if result.ErrorMessage != "" {
			output.WriteString(fmt.Sprintf("  \033[31m✗ %s\033[0m\n", result.ErrorMessage))
		} else if result.Task != nil {
			output.WriteString(fmt.Sprintf("  \033[32m✓ %s\033[0m\n", result.Message))
			output.WriteString(fmt.Sprintf("  \033[90mID: %s\033[0m\n", result.Task.ID))
			output.WriteString(fmt.Sprintf("  任务: %s\n", result.Task.Task))
			if result.Task.CronExpr != "" {
				output.WriteString(fmt.Sprintf("  Cron: %s\n", result.Task.CronExpr))
			}
			output.WriteString(fmt.Sprintf("  下次执行: %s\n", formatTime(result.Task.NextRunAt)))
		}

	case "schedule_list":
		var result struct {
			Success      bool   `json:"success"`
			TotalCount   int    `json:"total_count"`
			ErrorMessage string `json:"error_message"`
			Tasks        []struct {
				ID        string `json:"id"`
				Task      string `json:"task"`
				CronExpr  string `json:"cron_expr"`
				OneTime   bool   `json:"one_time"`
				NextRunAt string `json:"next_run_at"`
				Status    string `json:"status"`
				LastRunAt string `json:"last_run_at"`
				Result    string `json:"result"`
			} `json:"tasks"`
		}
		if err := json.Unmarshal([]byte(content), &result); err != nil {
			return ""
		}
		if result.ErrorMessage != "" {
			output.WriteString(fmt.Sprintf("  \033[31m✗ %s\033[0m\n", result.ErrorMessage))
		} else {
			output.WriteString(fmt.Sprintf("  \033[32m✓ 共 %d 个定时任务\033[0m\n", result.TotalCount))
			for i, t := range result.Tasks {
				var statusIcon string
				switch t.Status {
				case "completed":
					statusIcon = "[完成]"
				case "running":
					statusIcon = "[运行]"
				case "failed":
					statusIcon = "[失败]"
				case "cancelled":
					statusIcon = "[取消]"
				default:
					statusIcon = "[等待]"
				}
				output.WriteString(fmt.Sprintf("\n  %s \033[1m%d. %s\033[0m\n", statusIcon, i+1, t.Task))
				output.WriteString(fmt.Sprintf("     \033[90mID: %s | 状态: %s\033[0m\n", t.ID, t.Status))
				if t.CronExpr != "" {
					output.WriteString(fmt.Sprintf("     Cron: %s\n", t.CronExpr))
				}
				output.WriteString(fmt.Sprintf("     下次执行: %s\n", formatTime(t.NextRunAt)))
			}
		}

	case "schedule_cancel", "schedule_delete":
		var result struct {
			Success      bool   `json:"success"`
			Message      string `json:"message"`
			ErrorMessage string `json:"error_message"`
		}
		if err := json.Unmarshal([]byte(content), &result); err != nil {
			return ""
		}
		if result.ErrorMessage != "" {
			output.WriteString(fmt.Sprintf("  \033[31m✗ %s\033[0m\n", result.ErrorMessage))
		} else {
			output.WriteString(fmt.Sprintf("  \033[32m✓ %s\033[0m\n", result.Message))
		}
	}

	return output.String()
}

func formatDispatchResult(content string) string {
	var data struct {
		Results []struct {
			TaskIndex   int      `json:"task_index"`
			Description string   `json:"description"`
			Status      string   `json:"status"`
			Error       string   `json:"error,omitempty"`
			Operations  []string `json:"operations,omitempty"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(content), &data); err != nil || len(data.Results) == 0 {
		return ""
	}

	var b strings.Builder
	success, failed := 0, 0
	for _, r := range data.Results {
		if r.Status == "success" {
			success++
		} else {
			failed++
		}
	}
	b.WriteString(fmt.Sprintf("  \033[1m分发完成: %d 成功, %d 失败\033[0m\n", success, failed))

	for _, r := range data.Results {
		icon := "\033[32m✓\033[0m"
		if r.Status != "success" {
			icon = "\033[31m✗\033[0m"
		}
		b.WriteString(fmt.Sprintf("  %s [%d] %s", icon, r.TaskIndex, truncateString(r.Description, 60)))
		if r.Error != "" {
			b.WriteString(fmt.Sprintf(" \033[31m— %s\033[0m", truncateString(r.Error, 40)))
		}
		if len(r.Operations) > 0 {
			b.WriteString(fmt.Sprintf(" \033[90m(%d 项操作)\033[0m", len(r.Operations)))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func formatDispatchResultMarkdown(content string) string {
	var data struct {
		Results []struct {
			TaskIndex   int      `json:"task_index"`
			Description string   `json:"description"`
			Status      string   `json:"status"`
			Error       string   `json:"error,omitempty"`
			Operations  []string `json:"operations,omitempty"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(content), &data); err != nil || len(data.Results) == 0 {
		return ""
	}

	var b strings.Builder
	success, failed := 0, 0
	for _, r := range data.Results {
		if r.Status == "success" {
			success++
		} else {
			failed++
		}
	}
	b.WriteString(fmt.Sprintf("\n\n> **分发完成**: %d 成功, %d 失败\n", success, failed))
	for _, r := range data.Results {
		icon := "✓"
		if r.Status != "success" {
			icon = "✗"
		}
		b.WriteString(fmt.Sprintf("> %s [%d] %s", icon, r.TaskIndex, r.Description))
		if r.Error != "" {
			b.WriteString(fmt.Sprintf(" — %s", r.Error))
		}
		if len(r.Operations) > 0 {
			b.WriteString(fmt.Sprintf(" (%d 项操作)", len(r.Operations)))
		}
		b.WriteString("\n")
	}
	return b.String()
}
