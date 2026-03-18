package dispatch

import (
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// extractMessage 从 AgentEvent 提取消息
func extractMessage(event *adk.AgentEvent) *schema.Message {
	if event.Output != nil && event.Output.MessageOutput != nil {
		return event.Output.MessageOutput.Message
	}
	return nil
}

// extractOperations 从 AgentEvent 提取工具调用记录（含参数）
func extractOperations(event *adk.AgentEvent) []string {
	msg := extractMessage(event)
	if msg == nil || len(msg.ToolCalls) == 0 {
		return nil
	}
	ops := make([]string, 0, len(msg.ToolCalls))
	for _, tc := range msg.ToolCalls {
		args := tc.Function.Arguments
		if runes := []rune(args); len(runes) > 120 {
			args = string(runes[:120]) + "..."
		}
		ops = append(ops, fmt.Sprintf("%s(%s)", tc.Function.Name, args))
	}
	return ops
}
