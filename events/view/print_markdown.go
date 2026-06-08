package eventview

import (
	"encoding/json"

	"fkteams/events"

	"fmt"

	"strings"
)

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
		case EventMessageDelta:
			if event.DeltaKind != "" && event.DeltaKind != events.DeltaOutput {
				return nil
			}
			if lastAgent != event.AgentName {
				flushStream()
				lastAgent = event.AgentName
				fmt.Fprintf(&buf, "\n\n**[%s]**\n\n", event.AgentName)
			}
			buf.WriteString(event.Content)
			inStream = true

		case EventToolStart:
			flushStream()
			toolCalls := events.ToolCallsFromEvent(event)
			for i, tc := range toolCalls {
				if isInternalToolName(tc.Function.Name) {
					continue
				}
				if tc.ID != "" {
					toolNamesByID[tc.ID] = tc.Function.Name
				}
				if i == len(toolCalls)-1 {
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

		case EventToolEnd:
			if event.Content != "" && !isInternalContinueContent(event.Content) {
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
