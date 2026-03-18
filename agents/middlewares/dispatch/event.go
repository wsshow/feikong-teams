package dispatch

import (
	"context"
	"fkteams/fkevent"
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// emit 发送事件
func emit(ctx context.Context, agentName, eventType, content string) {
	evt := fkevent.Event{Type: eventType, AgentName: agentName}
	if eventType == "error" {
		evt.Error = content
	} else {
		evt.Content = content
	}
	_ = fkevent.DispatchEvent(ctx, evt)
}

// extractMessage 从 AgentEvent 提取消息
func extractMessage(event *adk.AgentEvent) *schema.Message {
	if event.Output != nil && event.Output.MessageOutput != nil {
		return event.Output.MessageOutput.Message
	}
	return nil
}

// extractOperations 从 AgentEvent 提取工具调用记录
func extractOperations(event *adk.AgentEvent) []string {
	msg := extractMessage(event)
	if msg == nil || len(msg.ToolCalls) == 0 {
		return nil
	}
	ops := make([]string, 0, len(msg.ToolCalls))
	for _, tc := range msg.ToolCalls {
		ops = append(ops, fmt.Sprintf("调用工具: %s", tc.Function.Name))
	}
	return ops
}
