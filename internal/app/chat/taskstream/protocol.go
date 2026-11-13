package taskstream

import (
	"fkteams/internal/app/agent/catalog/toolmeta"
	domainmessage "fkteams/internal/domain/message"
	"fkteams/internal/runtime/events"
	"fmt"
)

type fallbackToolDisplayResolver struct{}

func (fallbackToolDisplayResolver) FormatToolDisplay(name string) toolmeta.ToolDisplay {
	return toolmeta.FallbackDisplay(name)
}

// RuntimeEventPayload 将领域事件转换为所有入口共用的展示流载荷。
func RuntimeEventPayload(event events.Event, resolver toolmeta.Resolver) map[string]any {
	if resolver == nil {
		resolver = fallbackToolDisplayResolver{}
	}
	result := map[string]any{
		"type":       event.Type,
		"agent_name": event.AgentName,
	}
	if event.Type == events.EventProcessingEnd {
		result["status"] = "completed"
	}
	if event.RunID != "" {
		result["run_id"] = event.RunID
	}
	if event.EventID != "" {
		result["event_id"] = event.EventID
	}
	if event.Sequence != 0 {
		result["sequence"] = event.Sequence
	}
	if !event.CreatedAt.IsZero() {
		result["created_at"] = event.CreatedAt
	}
	if event.TurnID != "" {
		result["turn_id"] = event.TurnID
	}
	if event.MessageID != "" {
		result["message_id"] = event.MessageID
	}
	if event.BlockID != "" {
		result["block_id"] = event.BlockID
	}
	if event.BlockType != "" {
		result["block_type"] = event.BlockType
	}
	if event.Role != "" {
		result["role"] = event.Role
	}
	if event.DeltaKind != "" {
		result["delta_kind"] = event.DeltaKind
	}
	if event.MessageID != "" && event.DeltaKind != "" {
		result["stream_id"] = fmt.Sprintf("%s:%s", event.MessageID, event.DeltaKind)
		if event.Sequence != 0 {
			result["chunk_index"] = event.Sequence
		}
	}
	if event.RunPath != "" {
		result["run_path"] = event.RunPath
	}
	if event.Content != "" {
		result["content"] = event.Content
	}
	if event.Message != nil {
		if parts := event.Message.Clone().ContentParts; len(parts) > 0 {
			result["content_parts"] = parts
		}
	}
	if event.ReasoningContent != "" {
		result["reasoning_content"] = event.ReasoningContent
	}
	if toolCallsFromEvent := events.ToolCallsFromEvent(event); len(toolCallsFromEvent) > 0 {
		toolCalls := make([]map[string]any, 0, len(toolCallsFromEvent))
		for i, tc := range toolCallsFromEvent {
			display := resolver.FormatToolDisplay(tc.Function.Name)
			toolCall := map[string]any{
				"name":         tc.Function.Name,
				"display_name": display.DisplayName,
				"kind":         display.Kind,
			}
			if tc.ID != "" {
				toolCall["id"] = tc.ID
			}
			if ref := events.ToolCallRefAt(event, tc, i); ref != "" {
				toolCall["ref"] = ref
			}
			if tc.Index != nil {
				toolCall["index"] = *tc.Index
			}
			if display.Target != "" {
				toolCall["target"] = display.Target
			}
			if tc.Function.Arguments != "" {
				toolCall["arguments"] = tc.Function.Arguments
			}
			if len(tc.Extra) > 0 {
				toolCall["extra"] = domainmessage.CloneMetadata(tc.Extra)
			}
			toolCalls = append(toolCalls, toolCall)
		}
		result["tool_calls"] = toolCalls
		if len(toolCalls) == 1 {
			result["tool_call"] = toolCalls[0]
		}
	}
	if event.ToolCallID != "" {
		result["tool_call_id"] = event.ToolCallID
	}
	if event.ToolCallRef != "" {
		result["tool_call_ref"] = event.ToolCallRef
	}
	if event.ToolName != "" {
		result["tool_name"] = event.ToolName
		display := resolver.FormatToolDisplay(event.ToolName)
		result["tool_display_name"] = display.DisplayName
		result["tool_kind"] = display.Kind
		if display.Target != "" {
			result["tool_target"] = display.Target
		}
	}
	if event.ToolCallIndex != nil {
		result["tool_call_index"] = *event.ToolCallIndex
	}
	if event.ToolArgs != "" {
		result["tool_args"] = event.ToolArgs
	}
	if event.ToolResult != "" {
		result["tool_result"] = event.ToolResult
	}
	if events.IsMemberEvent(event) {
		result["is_member_event"] = true
	}
	if event.MemberCallID != "" {
		result["member_call_id"] = event.MemberCallID
	}
	if event.MemberToolName != "" {
		result["member_tool_name"] = event.MemberToolName
	}
	if event.MemberName != "" {
		result["member_name"] = event.MemberName
	}
	if event.MemberOrder != nil {
		result["member_order"] = *event.MemberOrder
	}
	if event.ParentToolCallID != "" {
		result["parent_tool_call_id"] = event.ParentToolCallID
	}
	if event.ParentToolName != "" {
		result["parent_tool_name"] = event.ParentToolName
	}
	if event.Detail != "" {
		result["detail"] = event.Detail
	}
	if event.Error != "" {
		result["error"] = event.Error
		attachFriendlyError(result, event.Error)
	}
	if event.PromptTokens > 0 {
		result["prompt_tokens"] = event.PromptTokens
	}
	if event.CompletionTokens > 0 {
		result["completion_tokens"] = event.CompletionTokens
	}
	if event.TotalTokens > 0 {
		result["total_tokens"] = event.TotalTokens
	}
	if event.Ask != nil {
		result["ask_id"] = event.Ask.ID
		if event.Ask.Question != "" {
			result["question"] = event.Ask.Question
		}
		if len(event.Ask.Options) > 0 {
			result["options"] = append([]string(nil), event.Ask.Options...)
		}
		if event.Ask.MultiSelect {
			result["multi_select"] = event.Ask.MultiSelect
		}
		if len(event.Ask.Selected) > 0 {
			result["selected"] = append([]string(nil), event.Ask.Selected...)
		}
		if event.Ask.FreeText != "" {
			result["free_text"] = event.Ask.FreeText
		}
	}
	if event.Usage != nil {
		result["usage"] = event.Usage
	}
	if event.Notice != nil {
		result["notice"] = event.Notice
	}
	if event.Approval != nil {
		result["approval"] = event.Approval
	}
	return result
}

func attachFriendlyError(result map[string]any, raw string) map[string]any {
	if raw == "" {
		return result
	}
	friendly := events.NormalizeFriendlyError(raw)
	result["error_code"] = friendly.Code
	result["error_title"] = friendly.Title
	result["display_error"] = friendly.Message
	result["technical_error"] = friendly.TechnicalDetail
	if len(friendly.Suggestions) > 0 {
		result["error_suggestions"] = friendly.Suggestions
	}
	return result
}
