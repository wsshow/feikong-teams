package schedule

import (
	"encoding/json"
	"fmt"
	"strings"

	"fkteams/internal/domain/event"
	"fkteams/internal/domain/message"
)

// newMarkdownCollector 收集后台调度运行事件并生成稳定的 Markdown 结果。
func newMarkdownCollector() (func(event.Event) error, func() string) {
	collector := &markdownCollector{
		toolNamesByID: map[string]string{},
	}
	return collector.handle, collector.result
}

type markdownCollector struct {
	buf           strings.Builder
	lastAgent     string
	lastToolName  string
	toolNamesByID map[string]string
	inStream      bool
}

func (c *markdownCollector) handle(e event.Event) error {
	switch e.Type {
	case event.TypeAssistantText:
		c.writeMessageDelta(e)
	case event.TypeToolCallStarted:
		c.writeToolStart(e)
	case event.TypeToolCallCompleted:
		c.writeToolEnd(e)
	case event.TypeSystemNotice:
		c.writeNotice(e)
	case event.TypeError:
		c.flushStream()
		fmt.Fprintf(&c.buf, "\n\n**Error [%s]**: %s", displayAgent(e.AgentName), e.Error)
	}
	return nil
}

func (c *markdownCollector) result() string {
	c.flushStream()
	return strings.TrimSpace(c.buf.String())
}

func (c *markdownCollector) flushStream() {
	if c.inStream {
		c.buf.WriteString("\n")
		c.inStream = false
	}
}

func (c *markdownCollector) writeMessageDelta(e event.Event) {
	if e.DeltaKind != "" && e.DeltaKind != event.DeltaOutput {
		return
	}
	if e.Content == "" {
		return
	}
	if e.AgentName != "" && c.lastAgent != e.AgentName {
		c.flushStream()
		c.lastAgent = e.AgentName
		fmt.Fprintf(&c.buf, "\n\n**[%s]**\n\n", e.AgentName)
	}
	c.buf.WriteString(e.Content)
	c.inStream = true
}

func (c *markdownCollector) writeToolStart(e event.Event) {
	c.flushStream()
	toolCalls := toolCallsFromEvent(e)
	if len(toolCalls) == 0 && e.ToolName != "" {
		toolCalls = append(toolCalls, message.ToolCall{
			ID: e.ToolCallID,
			Function: message.FunctionCall{
				Name:      e.ToolName,
				Arguments: e.ToolArgs,
			},
		})
	}
	for _, tc := range toolCalls {
		name := tc.Function.Name
		if name == "" {
			continue
		}
		c.lastToolName = name
		if tc.ID != "" {
			c.toolNamesByID[tc.ID] = name
		}
		fmt.Fprintf(&c.buf, "\n\n> **[%s]** tool: `%s`", displayAgent(e.AgentName), name)
		if tc.Function.Arguments != "" {
			fmt.Fprintf(&c.buf, "\n> args: `%s`", truncateMarkdownLine(tc.Function.Arguments, 120))
		}
	}
	c.lastAgent = ""
}

func (c *markdownCollector) writeToolEnd(e event.Event) {
	content := strings.TrimSpace(e.Content)
	if content == "" {
		content = strings.TrimSpace(e.ToolResult)
	}
	if content == "" {
		return
	}
	name := c.lastToolName
	if e.ToolCallID != "" {
		if mapped, ok := c.toolNamesByID[e.ToolCallID]; ok {
			name = mapped
		}
	}
	if summary := summarizeToolResult(content); summary != "" {
		if name != "" {
			fmt.Fprintf(&c.buf, "\n\n> `%s`: %s", name, summary)
		} else {
			fmt.Fprintf(&c.buf, "\n\n> %s", summary)
		}
	}
	c.lastAgent = ""
}

func (c *markdownCollector) writeNotice(e event.Event) {
	if e.Notice != nil && e.Notice.Code == "transfer" {
		c.flushStream()
		fmt.Fprintf(&c.buf, "\n\n> **[%s]** -> %s", displayAgent(e.AgentName), e.Content)
		c.lastAgent = ""
	}
}

func toolCallsFromEvent(e event.Event) []message.ToolCall {
	if e.ToolCall == nil {
		return e.ToolCalls
	}
	toolCalls := make([]message.ToolCall, 0, len(e.ToolCalls)+1)
	toolCalls = append(toolCalls, *e.ToolCall)
	toolCalls = append(toolCalls, e.ToolCalls...)
	return toolCalls
}

func summarizeToolResult(content string) string {
	var result struct {
		Message      string `json:"message"`
		ErrorMessage string `json:"error_message"`
	}
	if err := json.Unmarshal([]byte(content), &result); err == nil {
		switch {
		case result.ErrorMessage != "":
			return result.ErrorMessage
		case result.Message != "":
			return result.Message
		}
	}
	return truncateMarkdownLine(content, 160)
}

func displayAgent(agentName string) string {
	if agentName == "" {
		return "agent"
	}
	return agentName
}

func truncateMarkdownLine(s string, limit int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= limit {
		return s
	}
	if limit <= 3 {
		return s[:limit]
	}
	return s[:limit-3] + "..."
}
