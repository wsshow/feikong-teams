package eventlog

import (
	"fkteams/internal/app/agent/catalog/toolmeta"
	"fkteams/internal/app/appdata"
	"fkteams/internal/runtime/atomicfile"

	"fmt"

	"os"
	"path/filepath"

	"strings"

	"time"
)

func (h *HistoryRecorder) SaveToMarkdownFile(filePath string) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return saveMessagesToMarkdown(h.snapshotMessagesLocked(), filePath)
}

func saveMessagesToMarkdown(messages []AgentMessage, filePath string) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create dir %s: %w", dir, err)
	}

	var md strings.Builder

	md.WriteString("# 对话历史\n\n")
	fmt.Fprintf(&md, "**生成时间**: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&md, "**对话轮次**: %d\n\n", len(messages))

	agentMap := make(map[string]int)
	for _, msg := range messages {
		agentMap[msg.AgentName]++
	}
	md.WriteString("**参与代理**: ")
	first := true
	for agent, count := range agentMap {
		if !first {
			md.WriteString(", ")
		}
		fmt.Fprintf(&md, "%s (%d次)", agent, count)
		first = false
	}
	md.WriteString("\n\n---\n\n")

	for i, msg := range messages {
		fmt.Fprintf(&md, "## %d. %s\n\n", i+1, msg.AgentName)

		duration := msg.EndTime.Sub(msg.StartTime)
		fmt.Fprintf(&md, "**时间**: %s - %s (%v)\n\n",
			msg.StartTime.Format("15:04:05"),
			msg.EndTime.Format("15:04:05"),
			duration.Round(time.Millisecond))

		if msg.RunPath != "" {
			fmt.Fprintf(&md, "**路径**: `%s`\n\n", msg.RunPath)
		}

		md.WriteString("**内容**:\n\n")
		for _, event := range msg.Events {
			switch event.Type {
			case MsgTypeText:
				md.WriteString(event.Content)
				md.WriteString("\n\n")

			case MsgTypeToolCall:
				if event.ToolCall != nil {
					displayName := event.ToolCall.DisplayName
					if displayName == "" {
						displayName = toolmeta.FallbackDisplay(event.ToolCall.Name).DisplayName
					}
					fmt.Fprintf(&md, "> **工具调用**: %s\n", displayName)
					if event.ToolCall.Arguments != "" {
						fmt.Fprintf(&md, "> - **参数**: `%s`\n", event.ToolCall.Arguments)
					}
					if event.ToolCall.Result != "" {
						fmt.Fprintf(&md, "> - **结果**: %s\n", event.ToolCall.Result)
					}
					md.WriteString("\n")
				}

			case MsgTypeNotice:
				if event.Content != "" {
					fmt.Fprintf(&md, "> **[Notice]**: %s\n\n", event.Content)
				}

			case MsgTypeError:
				fmt.Fprintf(&md, "> **[错误]**: %s\n\n", event.Content)
			}
		}

		if i < len(messages)-1 {
			md.WriteString("---\n\n")
		}
	}

	if err := atomicfile.WriteFile(filePath, []byte(md.String()), 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

func (h *HistoryRecorder) SaveToMarkdownWithTimestamp() (string, error) {
	timestamp := time.Now().Format("20060102_150405")
	filePath := filepath.Join(appdata.Dir(), "history", "output_history", fmt.Sprintf("chat_%s.md", timestamp))
	err := h.SaveToMarkdownFile(filePath)
	return filePath, err
}
