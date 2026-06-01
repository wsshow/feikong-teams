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

func isMemberRunPath(runPath string) bool {
	return strings.Contains(runPath, "ask_") || strings.Contains(runPath, "ask-")
}

func agentDisplayName(name string) string {
	normalized := strings.TrimPrefix(strings.TrimSpace(name), agentToolPrefix)
	normalized = strings.ReplaceAll(normalized, "-", "_")
	if label := agentToolLabels[normalized]; label != "" {
		return label
	}
	return titleIdentifier(normalized)
}

func agentKey(name string) string {
	normalized := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(name)), agentToolPrefix)
	normalized = strings.ReplaceAll(normalized, "-", "_")
	if normalized == "" {
		return "member"
	}
	return normalized
}

func agentToolKey(name string) (string, string, bool) {
	target, ok := strings.CutPrefix(name, agentToolPrefix)
	if !ok {
		return "", "", false
	}
	return agentKey(target), agentDisplayName(target), true
}

func isErrorContent(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(content, "执行出错") ||
		strings.Contains(lower, "error") ||
		strings.Contains(lower, "failed") ||
		strings.Contains(content, "失败")
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
	toolNamesByID := map[string]string{}
	memberPanel := newMemberPanel()
	memberNamesByToolID := map[string]string{}
	memberKeysByToolID := map[string]string{}
	memberPending := map[string]bool{}
	inReasoning := false
	var sb streamBuf
	var rw *reasoningWriter

	tryFlush := func() {
		if sb.buf.Len() > 0 {
			sb.flush()
		}
	}

	ensureMember := func(key, name string) {
		if key == "" {
			return
		}
		if name == "" {
			name = agentDisplayName(key)
		}
		memberPending[key] = true
		memberPanel.send(memberViewEvent{Key: key, Name: name, Type: "start"})
	}

	memberFromEvent := func(event Event) (string, string) {
		key := agentKey(event.AgentName)
		name := agentDisplayName(event.AgentName)
		ensureMember(key, name)
		return key, name
	}

	finishMembersIfIdle := func() {
		if len(memberPending) == 0 {
			return
		}
		for _, pending := range memberPending {
			if pending {
				return
			}
		}
		memberPanel.finish()
		memberPending = map[string]bool{}
	}

	finishMembersBeforeParentOutput := func(event Event) {
		if len(memberPending) == 0 || isMemberRunPath(event.RunPath) {
			return
		}
		for _, pending := range memberPending {
			if pending {
				return
			}
		}
		memberPanel.finish()
		memberPending = map[string]bool{}
	}

	printFn := func(event Event) {
		switch event.Type {
		case EventReasoningChunk:
			if isMemberRunPath(event.RunPath) {
				key, name := memberFromEvent(event)
				memberPanel.send(memberViewEvent{Key: key, Name: name, Type: "content", Content: event.Content})
				return
			}
			finishMembersBeforeParentOutput(event)
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
			if isMemberRunPath(event.RunPath) {
				key, name := memberFromEvent(event)
				memberPanel.send(memberViewEvent{Key: key, Name: name, Type: "content", Content: event.Content})
				return
			}
			finishMembersBeforeParentOutput(event)
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
			if isMemberRunPath(event.RunPath) {
				key, name := memberFromEvent(event)
				if event.ReasoningContent != "" {
					memberPanel.send(memberViewEvent{Key: key, Name: name, Type: "content", Content: event.ReasoningContent})
				}
				if event.Content != "" {
					memberPanel.send(memberViewEvent{Key: key, Name: name, Type: "content", Content: event.Content})
				}
				return
			}
			finishMembersBeforeParentOutput(event)
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
			if isMemberRunPath(event.RunPath) {
				key, name := memberFromEvent(event)
				if event.Content != "" {
					memberPanel.send(memberViewEvent{Key: key, Name: name, Type: "content", Content: "\n[工具结果]\n" + event.Content})
				}
				return
			}
			tryFlush()
			toolName := lastToolName
			if event.ToolCallID != "" {
				if name, ok := toolNamesByID[event.ToolCallID]; ok {
					toolName = name
				}
			}
			display := FormatToolDisplay(toolName)
			if display.Kind == "agent" {
				key, memberName, ok := agentToolKey(toolName)
				if !ok {
					key = agentKey(display.Target)
					memberName = display.Target
				}
				if event.ToolCallID != "" && memberNamesByToolID[event.ToolCallID] != "" {
					memberName = memberNamesByToolID[event.ToolCallID]
				}
				if event.ToolCallID != "" && memberKeysByToolID[event.ToolCallID] != "" {
					key = memberKeysByToolID[event.ToolCallID]
				}
				if isErrorContent(event.Content) {
					memberPanel.send(memberViewEvent{Key: key, Name: memberName, Type: "error", Content: event.Content})
				} else {
					memberPanel.send(memberViewEvent{Key: key, Name: memberName, Type: "done", Content: event.Content})
				}
				memberPending[key] = false
				finishMembersIfIdle()
				return
			}
			resultTitle := "工具结果"
			fmt.Printf("\n\033[1;33m⚙ [%s] %s:\033[0m\n", event.AgentName, resultTitle)
			if event.Content != "" {
				var formatted string
				switch toolName {
				case "search":
					formatted = formatSearchResults(event.Content)
				case "execute":
					formatted = formatCommandResult(event.Content)
				case "file_read", "file_write", "file_edit", "file_list", "grep":
					formatted = formatFileOpResult(event.Content)
				case "file_patch":
					formatted = formatFilePatchResult(event.Content)
				case "ssh_execute", "ssh_upload", "ssh_download", "ssh_list_dir":
					formatted = formatSSHResult(event.Content, toolName)
				case "todo_add", "todo_list", "todo_update", "todo_delete", "todo_batch_add", "todo_batch_delete", "todo_clear":
					formatted = formatTodoResult(event.Content, toolName)
				case "schedule_add", "schedule_list", "schedule_cancel", "schedule_delete":
					formatted = formatSchedulerResult(event.Content, toolName)
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
			if isMemberRunPath(event.RunPath) {
				key, name := memberFromEvent(event)
				memberPanel.send(memberViewEvent{Key: key, Name: name, Type: "content", Content: event.Content})
				return
			}
			fmt.Printf("%s", event.Content)

		case EventToolCallsPreparing:
			if isMemberRunPath(event.RunPath) {
				key, name := memberFromEvent(event)
				for _, tool := range event.ToolCalls {
					if tool.Function.Name != "" {
						display := FormatToolDisplay(tool.Function.Name)
						memberPanel.send(memberViewEvent{Key: key, Name: name, Type: "op", Content: "准备调用工具: " + display.DisplayName})
					}
				}
				return
			}
			tryFlush()
			if inReasoning {
				inReasoning = false
				fmt.Printf("\033[0m\n")
			}
			for _, tool := range event.ToolCalls {
				if tool.Function.Name != "" {
					if tool.ID != "" {
						toolNamesByID[tool.ID] = tool.Function.Name
					}
					display := FormatToolDisplay(tool.Function.Name)
					if display.Kind == "agent" {
						key, memberName, _ := agentToolKey(tool.Function.Name)
						if tool.ID != "" {
							memberNamesByToolID[tool.ID] = memberName
							memberKeysByToolID[tool.ID] = key
						}
						ensureMember(key, memberName)
						memberPanel.send(memberViewEvent{Key: key, Name: memberName, Type: "op", Content: "任务准备中"})
					} else {
						fmt.Printf("\n\033[1;35m[%s] 准备调用工具: \033[1m%s\033[0m \033[90m(参数准备中...)\033[0m\n", event.AgentName, display.DisplayName)
					}
					lastToolName = tool.Function.Name
				}
			}

		case EventToolCalls:
			if isMemberRunPath(event.RunPath) {
				key, name := memberFromEvent(event)
				for _, tool := range event.ToolCalls {
					if tool.Function.Name == "" {
						continue
					}
					display := FormatToolDisplay(tool.Function.Name)
					op := "调用工具: " + display.DisplayName
					if tool.Function.Arguments != "" {
						op += " 参数: " + truncateString(tool.Function.Arguments, 160)
					}
					memberPanel.send(memberViewEvent{Key: key, Name: name, Type: "op", Content: op})
				}
				return
			}
			tryFlush()
			printedHeader := false
			for i, tool := range event.ToolCalls {
				if tool.ID != "" {
					toolNamesByID[tool.ID] = tool.Function.Name
				}
				display := FormatToolDisplay(tool.Function.Name)
				if display.Kind == "agent" {
					key, memberName, _ := agentToolKey(tool.Function.Name)
					if tool.ID != "" {
						memberNamesByToolID[tool.ID] = memberName
						memberKeysByToolID[tool.ID] = key
					}
					ensureMember(key, memberName)
					op := "任务已分配"
					if tool.Function.Arguments != "" {
						op = "任务: " + truncateString(tool.Function.Arguments, 200)
					}
					memberPanel.send(memberViewEvent{Key: key, Name: memberName, Type: "op", Content: op})
				} else {
					if !printedHeader {
						fmt.Printf("\n\033[1;35m[%s] 调用:\033[0m\n", event.AgentName)
						printedHeader = true
					}
					fmt.Printf("  %d. \033[1m%s\033[0m\n", i+1, display.DisplayName)
					if tool.Function.Arguments != "" {
						args := truncateString(tool.Function.Arguments, 200)
						fmt.Printf("     参数: %s\n", args)
					}
				}
				if i == len(event.ToolCalls)-1 {
					lastToolName = tool.Function.Name
				}
			}
			if printedHeader {
				fmt.Println()
			}

		case EventAction:
			finishMembersBeforeParentOutput(event)
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
			if isMemberRunPath(event.RunPath) {
				key, name := memberFromEvent(event)
				memberPanel.send(memberViewEvent{Key: key, Name: name, Type: "error", Content: event.Error})
				memberPending[key] = false
				finishMembersIfIdle()
				return
			}
			finishMembersBeforeParentOutput(event)
			tryFlush()
			fmt.Printf("\n\033[1;31m✗ [%s] 错误:\033[0m\n", event.AgentName)
			fmt.Printf("  \033[31m%s\033[0m\n", event.Error)
			if event.RunPath != "" {
				fmt.Printf("  路径: %s\n", event.RunPath)
			}
			fmt.Println()

		case EventToolCallsArgsDelta:
			if isMemberRunPath(event.RunPath) {
				return
			}
		default:
			finishMembersBeforeParentOutput(event)
			fmt.Printf("\n\033[1;90m? 未知事件: %s\033[0m\n", event.Type)
			if event.AgentName != "" {
				fmt.Printf("  代理: %s\n", event.AgentName)
			}
			if event.Content != "" {
				fmt.Printf("  内容: %s\n", event.Content)
			}
		}
	}

	return printFn, func() {
		tryFlush()
		memberPanel.finish()
	}
}

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
			if i < 20 { // 限制显示数量
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

// NewMarkdownCollector 创建事件 Markdown 收集器，供后台任务使用
func NewMarkdownCollector() (callback func(Event) error, getResult func() string) {
	var buf strings.Builder
	lastAgent := ""
	lastToolName := ""
	toolNamesByID := map[string]string{}
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
				fmt.Fprintf(&buf, "\n\n**[%s]**\n\n", event.AgentName)
			}
			buf.WriteString(event.Content)
			inStream = true

		case EventMessage:
			if event.Content != "" {
				flushStream()
				if lastAgent != event.AgentName {
					lastAgent = event.AgentName
					fmt.Fprintf(&buf, "\n\n**[%s]**\n\n", event.AgentName)
				}
				buf.WriteString(event.Content)
			}

		case EventToolCallsPreparing:
			for _, tc := range event.ToolCalls {
				if tc.Function.Name != "" {
					lastToolName = tc.Function.Name
					if tc.ID != "" {
						toolNamesByID[tc.ID] = tc.Function.Name
					}
				}
			}

		case EventToolCalls:
			flushStream()
			for i, tc := range event.ToolCalls {
				if tc.ID != "" {
					toolNamesByID[tc.ID] = tc.Function.Name
				}
				if i == len(event.ToolCalls)-1 {
					lastToolName = tc.Function.Name
				}
				args := truncateString(tc.Function.Arguments, 100)
				display := FormatToolDisplay(tc.Function.Name)
				if args != "" {
					fmt.Fprintf(&buf, "\n\n> **[%s]** 调用: `%s`\n> 参数: `%s`", event.AgentName, display.DisplayName, args)
				} else {
					fmt.Fprintf(&buf, "\n\n> **[%s]** 调用: `%s`", event.AgentName, display.DisplayName)
				}
			}
			lastAgent = ""

		case EventToolResult:
			if event.Content != "" {
				toolName := lastToolName
				if event.ToolCallID != "" {
					if name, ok := toolNamesByID[event.ToolCallID]; ok {
						toolName = name
					}
				}
				if formatted := formatToolResultMarkdown(event.Content, toolName); formatted != "" {
					buf.WriteString(formatted)
				}
			}
			lastAgent = ""

		case EventAction:
			switch event.ActionType {
			case ActionTransfer:
				flushStream()
				fmt.Fprintf(&buf, "\n\n> **[%s]** → %s", event.AgentName, event.Content)
				lastAgent = ""
			case ActionContextCompressStart, ActionContextCompress:
				// 跳过上下文压缩提示
			}

		case EventError:
			flushStream()
			fmt.Fprintf(&buf, "\n\n**错误 [%s]**: %s", event.AgentName, event.Error)
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
	case "search":
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
		fmt.Fprintf(&output, "\n\n> ✓ %s", result.Message)
	}
	for i, r := range result.Results {
		if i >= 5 {
			fmt.Fprintf(&output, "\n>\n> _...还有 %d 条结果_", len(result.Results)-5)
			break
		}
		fmt.Fprintf(&output, "\n>\n> %d. **%s**", i+1, r.Title)
		if r.URL != "" {
			fmt.Fprintf(&output, " <%s>", r.URL)
		}
		if r.Summary != "" {
			summary := truncateString(strings.ReplaceAll(r.Summary, "\n", " "), 100)
			fmt.Fprintf(&output, "\n>    %s", summary)
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
			fmt.Fprintf(&output, "  \033[31m✗ %s\033[0m\n", result.ErrorMessage)
		} else if result.Task != nil {
			fmt.Fprintf(&output, "  \033[32m✓ %s\033[0m\n", result.Message)
			fmt.Fprintf(&output, "  \033[90mID: %s\033[0m\n", result.Task.ID)
			fmt.Fprintf(&output, "  任务: %s\n", result.Task.Task)
			if result.Task.CronExpr != "" {
				fmt.Fprintf(&output, "  Cron: %s\n", result.Task.CronExpr)
			}
			fmt.Fprintf(&output, "  下次执行: %s\n", formatTime(result.Task.NextRunAt))
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
			fmt.Fprintf(&output, "  \033[31m✗ %s\033[0m\n", result.ErrorMessage)
		} else {
			fmt.Fprintf(&output, "  \033[32m✓ 共 %d 个定时任务\033[0m\n", result.TotalCount)
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
				fmt.Fprintf(&output, "\n  %s \033[1m%d. %s\033[0m\n", statusIcon, i+1, t.Task)
				fmt.Fprintf(&output, "     \033[90mID: %s | 状态: %s\033[0m\n", t.ID, t.Status)
				if t.CronExpr != "" {
					fmt.Fprintf(&output, "     Cron: %s\n", t.CronExpr)
				}
				fmt.Fprintf(&output, "     下次执行: %s\n", formatTime(t.NextRunAt))
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
			fmt.Fprintf(&output, "  \033[31m✗ %s\033[0m\n", result.ErrorMessage)
		} else {
			fmt.Fprintf(&output, "  \033[32m✓ %s\033[0m\n", result.Message)
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
	fmt.Fprintf(&b, "  \033[1m分发完成: %d 成功, %d 失败\033[0m\n", success, failed)

	for _, r := range data.Results {
		icon := "\033[32m✓\033[0m"
		if r.Status != "success" {
			icon = "\033[31m✗\033[0m"
		}
		fmt.Fprintf(&b, "  %s [%d] %s", icon, r.TaskIndex, truncateString(r.Description, 60))
		if r.Error != "" {
			fmt.Fprintf(&b, " \033[31m— %s\033[0m", truncateString(r.Error, 40))
		}
		if len(r.Operations) > 0 {
			fmt.Fprintf(&b, " \033[90m(%d 项操作)\033[0m", len(r.Operations))
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
	fmt.Fprintf(&b, "\n\n> **分发完成**: %d 成功, %d 失败\n", success, failed)
	for _, r := range data.Results {
		icon := "✓"
		if r.Status != "success" {
			icon = "✗"
		}
		fmt.Fprintf(&b, "> %s [%d] %s", icon, r.TaskIndex, r.Description)
		if r.Error != "" {
			fmt.Fprintf(&b, " — %s", r.Error)
		}
		if len(r.Operations) > 0 {
			fmt.Fprintf(&b, " (%d 项操作)", len(r.Operations))
		}
		b.WriteString("\n")
	}
	return b.String()
}
