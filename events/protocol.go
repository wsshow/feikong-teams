package events

import (
	"fkteams/agentcore"
	"fmt"
)

func ToolCallsFromEvent(event Event) []agentcore.ToolCall {
	if event.ToolCall == nil {
		return event.ToolCalls
	}
	toolCalls := make([]agentcore.ToolCall, 0, len(event.ToolCalls)+1)
	toolCalls = append(toolCalls, *event.ToolCall)
	toolCalls = append(toolCalls, event.ToolCalls...)
	return toolCalls
}

func ToolCallRefAt(event Event, tool agentcore.ToolCall, position int) string {
	if tool.Index != nil && event.ToolCallRefs != nil {
		if ref := event.ToolCallRefs[*tool.Index]; ref != "" {
			return ref
		}
	}
	if event.ToolCallRefs != nil {
		if ref := event.ToolCallRefs[position]; ref != "" {
			return ref
		}
	}
	if event.ToolCall != nil && position == 0 && event.ToolCallRef != "" {
		return event.ToolCallRef
	}
	return ""
}

func ValidateEventContract(event Event) error {
	switch event.Type {
	case EventToolStart:
		if event.ToolCallRef == "" || event.ToolCallID == "" {
			return fmt.Errorf("tool_start missing stable tool identity")
		}
	case EventToolUpdate, EventToolEnd:
		if event.ToolCallRef == "" || event.ToolCallID == "" {
			return fmt.Errorf("%s missing stable tool identity", event.Type)
		}
	case EventMessageDelta:
		if event.DeltaKind == DeltaToolArgs || event.DeltaKind == DeltaToolResult {
			if event.ToolCallRef == "" || event.ToolCallID == "" {
				return fmt.Errorf("message_delta %s missing stable tool identity", event.DeltaKind)
			}
		}
	case EventMessageEnd:
		if event.Role == agentcore.RoleTool && (event.ToolCallRef == "" || event.ToolCallID == "") {
			return fmt.Errorf("tool message_end missing stable tool identity")
		}
		for i, tool := range event.ToolCalls {
			if IsInternalToolName(tool.Function.Name) {
				continue
			}
			if tool.ID == "" {
				return fmt.Errorf("message_end tool call missing id at position %d", i)
			}
			if ToolCallRefAt(event, tool, i) == "" {
				return fmt.Errorf("message_end tool call missing ref at position %d", i)
			}
		}
	}
	return nil
}
