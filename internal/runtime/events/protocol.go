package events

import (
	"fmt"

	"fkteams/internal/domain/message"
)

func ToolCallsFromEvent(event Event) []message.ToolCall {
	if event.ToolCall == nil {
		return event.ToolCalls
	}
	toolCalls := make([]message.ToolCall, 0, len(event.ToolCalls)+1)
	toolCalls = append(toolCalls, *event.ToolCall)
	toolCalls = append(toolCalls, event.ToolCalls...)
	return toolCalls
}

func ToolCallRefAt(event Event, tool message.ToolCall, position int) string {
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
	if event.Type == "" {
		return fmt.Errorf("event type is required")
	}
	switch event.Type {
	case EventTurnStarted, EventTurnCompleted:
		if event.RunID == "" || event.TurnID == "" {
			return fmt.Errorf("%s missing run or turn identity", event.Type)
		}
	case EventUserMessage:
		if event.Content == "" && event.Message == nil {
			return fmt.Errorf("user_message missing content")
		}
	case EventAssistantStarted, EventAssistantCompleted, EventAssistantReasoning, EventAssistantText:
		if event.MessageID == "" {
			return fmt.Errorf("%s missing message identity", event.Type)
		}
		if event.Role == "" {
			return fmt.Errorf("%s missing message role", event.Type)
		}
		if event.Type == EventAssistantCompleted {
			if err := validateMessageToolCalls(event, "assistant_completed"); err != nil {
				return err
			}
		}
	case EventAskRequested:
		if event.Detail == "" && (event.Ask == nil || event.Ask.ID == "") {
			return fmt.Errorf("ask_requested missing ask identity")
		}
		if event.Content == "" && (event.Ask == nil || event.Ask.Question == "") {
			return fmt.Errorf("ask_requested missing question")
		}
	case EventAskAnswered:
		if event.Detail == "" && (event.Ask == nil || event.Ask.ID == "") {
			return fmt.Errorf("ask_answered missing ask identity")
		}
	case EventToolCallStarted, EventToolCallResult, EventToolCallCompleted, EventToolCallArguments, EventToolCallFailed:
		if event.ToolName == "" {
			return fmt.Errorf("%s missing tool name", event.Type)
		}
		if event.ToolCallRef == "" || event.ToolCallID == "" {
			return fmt.Errorf("%s missing stable tool identity", event.Type)
		}
	case EventError:
		if event.Error == "" && event.Content == "" {
			return fmt.Errorf("error event missing error content")
		}
	case EventSystemNotice:
		if event.Content == "" && (event.Notice == nil || event.Notice.Message == "") {
			return fmt.Errorf("system_notice missing notice content")
		}
	case EventUsageReported:
		if event.PromptTokens == 0 && event.CompletionTokens == 0 && event.TotalTokens == 0 {
			return fmt.Errorf("usage event missing token counts")
		}
	}
	return nil
}

func validateMessageToolCalls(event Event, eventName string) error {
	for i, tool := range event.ToolCalls {
		if IsInternalToolName(tool.Function.Name) {
			continue
		}
		if tool.ID == "" {
			return fmt.Errorf("%s tool call missing id at position %d", eventName, i)
		}
		if ToolCallRefAt(event, tool, i) == "" {
			return fmt.Errorf("%s tool call missing ref at position %d", eventName, i)
		}
	}
	return nil
}
