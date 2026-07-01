package handler

import (
	"fmt"

	eventlog "fkteams/internal/adapters/storage/file/history"
	domainmessage "fkteams/internal/domain/message"
	"fkteams/internal/runtime/events"
)

func (rt *Runtime) transcriptToChatEvents(sessionID string, transcript []eventlog.TranscriptEvent) []map[string]any {
	records := make([]eventlog.SessionTranscriptRecord, 0, len(transcript))
	for _, item := range transcript {
		records = append(records, eventlog.SessionTranscriptRecord{Event: item})
	}
	return rt.transcriptRecordsToChatEvents(sessionID, records)
}

func (rt *Runtime) transcriptRecordsToChatEvents(sessionID string, transcript []eventlog.SessionTranscriptRecord) []map[string]any {
	result := make([]map[string]any, 0, len(transcript))
	turn := 0
	for index, record := range transcript {
		item := record.Event
		if item.Type == eventlog.TranscriptUserMessage && record.Member == nil {
			turn++
		}
		for _, event := range transcriptEventToRuntimeEvents(item, transcriptRuntimeTurnID(sessionID, turn)) {
			attachTranscriptMember(&event, record.Member)
			event.Sequence = int64(len(result) + 1)
			payload := rt.convertEventToMap(event)
			payload["session_id"] = sessionID
			payload["transcript_index"] = index
			if item.ResultRef != "" {
				payload["result_ref"] = item.ResultRef
			}
			if item.Summary != "" {
				payload["summary"] = item.Summary
			}
			if item.Truncated {
				payload["truncated"] = true
				payload["original_chars"] = item.OriginalChars
			}
			if len(item.ContentParts) > 0 {
				payload["content_parts"] = append([]domainmessage.ContentPart(nil), item.ContentParts...)
			}
			result = append(result, payload)
		}
	}
	return result
}

func attachTranscriptMember(event *events.Event, metadata *eventlog.SubagentMetadata) {
	if event == nil || metadata == nil || metadata.ParentCallID == "" {
		return
	}
	event.MemberCallID = metadata.ParentCallID
	event.MemberToolName = metadata.ToolName
	event.MemberName = metadata.Agent
	event.ParentToolCallID = metadata.ParentCallID
	event.ParentToolName = metadata.ToolName
}

func transcriptEventToRuntimeEvents(item eventlog.TranscriptEvent, turnID string) []events.Event {
	base := events.Event{
		EventID:    item.ID,
		CreatedAt:  item.At,
		TurnID:     turnID,
		MessageID:  item.ID,
		AgentName:  item.Agent,
		ToolCallID: item.CallID,
	}
	switch item.Type {
	case eventlog.TranscriptUserMessage:
		base.Type = events.EventUserMessage
		base.Role = domainmessage.RoleUser
		base.Content = item.Content
		return []events.Event{base}
	case eventlog.TranscriptAgentStep, eventlog.TranscriptAssistantMessage:
		base.Type = events.EventAssistantCompleted
		base.Role = domainmessage.RoleAssistant
		base.Content = item.Content
		base.ReasoningContent = item.Reasoning
		base.Message = &domainmessage.Message{
			Role:             domainmessage.RoleAssistant,
			Content:          item.Content,
			ReasoningContent: item.Reasoning,
			ContentParts:     append([]domainmessage.ContentPart(nil), item.ContentParts...),
		}
		if item.Usage == nil {
			return []events.Event{base}
		}
		base.PromptTokens = item.Usage.PromptTokens
		base.CompletionTokens = item.Usage.CompletionTokens
		base.TotalTokens = item.Usage.TotalTokens
		base.Usage = &events.UsagePayload{
			PromptTokens:     item.Usage.PromptTokens,
			CompletionTokens: item.Usage.CompletionTokens,
			TotalTokens:      item.Usage.TotalTokens,
		}
		usage := events.Usage(item.Agent, "", item.Usage.PromptTokens, item.Usage.CompletionTokens, item.Usage.TotalTokens)
		usage.EventID = item.ID + ":usage"
		usage.CreatedAt = item.At
		usage.TurnID = base.TurnID
		usage.MessageID = item.ID
		usage.AgentName = item.Agent
		return []events.Event{base, usage}
	case eventlog.TranscriptToolCallStart:
		base.Type = events.EventToolCallStarted
		attachTranscriptToolCall(&base, item)
		return []events.Event{base}
	case eventlog.TranscriptToolCallEnd:
		base.Type = events.EventToolCallCompleted
		attachTranscriptToolCall(&base, item)
		base.Content = transcriptResultContent(item)
		base.ToolResult = base.Content
		return []events.Event{base}
	case eventlog.TranscriptAskRequested, eventlog.TranscriptAskAnswered:
		base.Type = events.EventAskRequested
		if item.Type == eventlog.TranscriptAskAnswered {
			base.Type = events.EventAskAnswered
		}
		if item.Ask != nil {
			base.Ask = &events.AskPayload{
				ID:          item.Ask.ID,
				Question:    item.Ask.Question,
				Options:     append([]string(nil), item.Ask.Options...),
				MultiSelect: item.Ask.MultiSelect,
				Selected:    append([]string(nil), item.Ask.Selected...),
				FreeText:    item.Ask.FreeText,
			}
		}
		base.Content = item.Content
		return []events.Event{base}
	case eventlog.TranscriptSystemNotice:
		base.Type = events.EventSystemNotice
		base.Content = item.Content
		base.Detail = item.Detail
		base.Notice = &events.NoticePayload{Message: item.Content}
		return []events.Event{base}
	case eventlog.TranscriptError:
		base.Type = events.EventError
		base.Content = item.Content
		if item.Error != nil {
			base.Error = item.Error.Message
		}
		return []events.Event{base}
	case eventlog.TranscriptCancelled:
		base.Type = events.EventCancelled
		base.Content = item.Content
		return []events.Event{base}
	default:
		return nil
	}
}

func attachTranscriptToolCall(event *events.Event, item eventlog.TranscriptEvent) {
	if event == nil {
		return
	}
	id := item.CallID
	name := item.Name
	args := item.Args
	event.ToolCallID = id
	if id != "" {
		event.ToolCallRef = "tool_call:" + id
	}
	event.ToolName = name
	event.ToolArgs = args
	event.ToolCall = &domainmessage.ToolCall{
		ID: id,
		Function: domainmessage.FunctionCall{
			Name:      name,
			Arguments: args,
		},
	}
	event.ToolCalls = []domainmessage.ToolCall{*event.ToolCall}
}

func transcriptResultContent(item eventlog.TranscriptEvent) string {
	if item.Result != "" {
		return item.Result
	}
	if item.Summary != "" {
		return item.Summary
	}
	if item.ResultRef != "" {
		return fmt.Sprintf("[tool result stored at %s]", item.ResultRef)
	}
	return ""
}

func transcriptRuntimeTurnID(sessionID string, turn int) string {
	if turn <= 0 {
		turn = 1
	}
	if sessionID == "" {
		return fmt.Sprintf("history:turn:%d", turn)
	}
	return fmt.Sprintf("%s:history:turn:%d", sessionID, turn)
}
