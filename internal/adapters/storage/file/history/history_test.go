package eventlog

import (
	domainevent "fkteams/internal/domain/event"
	"fkteams/internal/domain/message"
	"fkteams/internal/runtime/events"
	"testing"
)

func TestHistoryRecorderKeepsParentToolCallBeforeMemberMessage(t *testing.T) {
	recorder := NewHistoryRecorder()
	toolIndex := 0

	recorder.RecordEvent(Event{
		Sequence:    1,
		Type:        EventToolCallStarted,
		AgentName:   "coordinator",
		ToolCallID:  "call_1",
		ToolCallRef: "tool_call:call_1",
		ToolCallRefs: map[int]string{
			0: "tool_call:call_1",
		},
		ToolCalls: []message.ToolCall{{
			ID:    "call_1",
			Index: &toolIndex,
			Function: message.FunctionCall{
				Name:      "ask_fkagent_researcher",
				Arguments: `{"task":"查资料"}`,
			},
		}},
	})
	recorder.RecordEvent(Event{
		Sequence:       2,
		Type:           EventAssistantText,
		Role:           message.RoleAssistant,
		DeltaKind:      events.DeltaOutput,
		AgentName:      "researcher",
		Content:        "结果",
		MemberCallID:   "call_1",
		MemberToolName: "ask_fkagent_researcher",
		MemberName:     "Researcher",
		MemberOrder:    &toolIndex,
	})
	recorder.FinalizeCurrent()

	messages := recorder.GetMessages()
	if len(messages) != 2 {
		t.Fatalf("message count = %d, want 2", len(messages))
	}
	if messages[0].AgentName != "coordinator" {
		t.Fatalf("first message agent = %q, want coordinator", messages[0].AgentName)
	}
	if len(messages[0].Events) != 1 || messages[0].Events[0].Type != MsgTypeToolCall {
		t.Fatalf("first message event = %#v, want tool call", messages[0].Events)
	}
	if messages[1].MemberCallID != "call_1" {
		t.Fatalf("second message member_call_id = %q, want call_1", messages[1].MemberCallID)
	}
}

func TestHistoryRecorderPreservesMemberEventSequences(t *testing.T) {
	recorder := NewHistoryRecorder()
	memberOrder := 0

	recorder.RecordEvent(Event{
		Sequence:       10,
		Type:           EventAssistantReasoning,
		Role:           message.RoleAssistant,
		DeltaKind:      events.DeltaReasoning,
		AgentName:      "ask-member",
		Content:        "thinking",
		MemberCallID:   "member-call-1",
		MemberToolName: "ask_fkagent_member",
		MemberName:     "Ask Member",
		MemberOrder:    &memberOrder,
	})
	recorder.RecordEvent(Event{
		Sequence:       11,
		Type:           EventAssistantText,
		Role:           message.RoleAssistant,
		DeltaKind:      events.DeltaOutput,
		AgentName:      "ask-member",
		Content:        "about to ask",
		MemberCallID:   "member-call-1",
		MemberToolName: "ask_fkagent_member",
		MemberName:     "Ask Member",
		MemberOrder:    &memberOrder,
	})
	recorder.RecordEvent(Event{
		Sequence:       12,
		Type:           events.EventAskRequested,
		AgentName:      "ask-member",
		Content:        "Choose?",
		Detail:         "ask-1",
		Ask:            &domainevent.AskPayload{ID: "ask-1", Question: "Choose?"},
		MemberCallID:   "member-call-1",
		MemberToolName: "ask_fkagent_member",
		MemberName:     "Ask Member",
		MemberOrder:    &memberOrder,
	})
	recorder.FinalizeCurrent()

	messages := recorder.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(messages))
	}
	events := messages[0].Events
	if len(events) != 3 {
		t.Fatalf("event count = %d, want 3: %#v", len(events), events)
	}
	want := []int64{10, 11, 12}
	for i, sequence := range want {
		if events[i].Sequence != sequence {
			t.Fatalf("event %d sequence = %d, want %d", i, events[i].Sequence, sequence)
		}
	}
}

func TestHistoryRecorderStoresUsageAsUsageEvent(t *testing.T) {
	recorder := NewHistoryRecorder()

	recorder.RecordEvent(Event{
		Sequence:  1,
		Type:      EventAssistantText,
		Role:      message.RoleAssistant,
		DeltaKind: events.DeltaOutput,
		AgentName: "coordinator",
		Content:   "ok",
	})
	recorder.RecordEvent(Event{
		Sequence:         2,
		Type:             EventUsageReported,
		AgentName:        "coordinator",
		PromptTokens:     3,
		CompletionTokens: 4,
		TotalTokens:      7,
	})
	recorder.FinalizeCurrent()

	messages := recorder.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(messages))
	}
	if len(messages[0].Events) != 2 {
		t.Fatalf("event count = %d, want 2", len(messages[0].Events))
	}
	usageEvent := messages[0].Events[1]
	if usageEvent.Type != MsgTypeUsageReported {
		t.Fatalf("usage event type = %q, want %q", usageEvent.Type, MsgTypeUsageReported)
	}
	if usageEvent.Usage == nil || usageEvent.Usage.TotalTokens != 7 {
		t.Fatalf("usage event usage = %#v, want total tokens 7", usageEvent.Usage)
	}
}

func TestHistoryRecorderStoresFriendlyError(t *testing.T) {
	recorder := NewHistoryRecorder()

	recorder.RecordEvent(Event{
		Type:      EventError,
		AgentName: "coordinator",
		Error:     "deepseek does not support image_url type",
	})
	recorder.FinalizeCurrent()

	messages := recorder.GetMessages()
	if len(messages) != 1 || len(messages[0].Events) != 1 {
		t.Fatalf("messages = %#v, want one error event", messages)
	}
	event := messages[0].Events[0]
	if event.Type != MsgTypeError || event.Error == nil {
		t.Fatalf("event = %#v, want friendly error record", event)
	}
	if event.Error.Code != "model_unsupported_image_input" {
		t.Fatalf("error code = %q, want model_unsupported_image_input", event.Error.Code)
	}
	if event.Content == "" || event.Content == event.Error.TechnicalDetail {
		t.Fatalf("content = %q, technical = %q, want friendly content", event.Content, event.Error.TechnicalDetail)
	}
}

func TestHistoryRecorderRecordsCancellationForActiveMessages(t *testing.T) {
	recorder := NewHistoryRecorder()
	toolIndex := 0

	recorder.RecordEvent(Event{
		Sequence:    1,
		Type:        EventToolCallStarted,
		AgentName:   "coordinator",
		ToolCallRef: "tool_call:call_1",
		ToolCalls: []message.ToolCall{{
			ID:    "call_1",
			Index: &toolIndex,
			Function: message.FunctionCall{
				Name:      "ask_fkagent_researcher",
				Arguments: `{"task":"查资料"}`,
			},
		}},
	})
	recorder.RecordEvent(Event{
		Sequence:       2,
		Type:           EventAssistantText,
		Role:           message.RoleAssistant,
		DeltaKind:      events.DeltaReasoning,
		AgentName:      "researcher",
		Content:        "working",
		MemberCallID:   "call_1",
		MemberToolName: "ask_fkagent_researcher",
		MemberName:     "Researcher",
		MemberOrder:    &toolIndex,
	})

	recorder.RecordCancelled("任务已取消")

	messages := recorder.GetMessages()
	if len(messages) < 2 {
		t.Fatalf("message count = %d, want at least active message and cancellation notice", len(messages))
	}
	for i, msg := range messages[:len(messages)-1] {
		if hasEventType(msg, MsgTypeCancelled) {
			t.Fatalf("message %d events = %#v, want no cancelled marker", i, msg.Events)
		}
	}
	last := messages[len(messages)-1]
	if last.AgentName != "system" || !hasEventType(last, MsgTypeCancelled) {
		t.Fatalf("last message = %#v, want system cancelled notice", last)
	}
}

func TestHistoryRecorderRecordsToolRoleMessageEndAsToolResult(t *testing.T) {
	recorder := NewHistoryRecorder()
	toolIndex := 0

	recorder.RecordEvent(Event{
		Sequence:      1,
		Type:          EventToolCallStarted,
		AgentName:     "assistant",
		ToolCallID:    "call_1",
		ToolCallRef:   "ref_1",
		ToolName:      "echo",
		ToolArgs:      `{"text":"hello"}`,
		ToolCallIndex: &toolIndex,
	})
	recorder.RecordEvent(Event{
		Sequence:    2,
		Type:        events.EventToolCallCompleted,
		AgentName:   "assistant",
		ToolCallID:  "call_1",
		ToolCallRef: "ref_1",
		ToolName:    "echo",
		Content:     "echo: hello",
	})
	recorder.FinalizeCurrent()

	messages := recorder.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(messages))
	}
	if len(messages[0].Events) != 1 || messages[0].Events[0].ToolCall == nil {
		t.Fatalf("events = %#v, want one tool call", messages[0].Events)
	}
	toolCall := messages[0].Events[0].ToolCall
	if toolCall.Result != "echo: hello" {
		t.Fatalf("tool result = %q, want echo: hello", toolCall.Result)
	}
}

func TestHistoryRecorderUsesPositionToolRefsWhenToolCallIndexMissing(t *testing.T) {
	recorder := NewHistoryRecorder()

	recorder.RecordEvent(Event{
		Sequence:  1,
		Type:      events.EventAssistantCompleted,
		Role:      message.RoleAssistant,
		AgentName: "assistant",
		ToolCalls: []message.ToolCall{
			{
				ID: "call_1",
				Function: message.FunctionCall{
					Name:      "echo",
					Arguments: `{"text":"hello"}`,
				},
			},
		},
		ToolCallRefs: map[int]string{0: "tool_call:call_1"},
	})
	recorder.RecordEvent(Event{
		Sequence:    2,
		Type:        events.EventToolCallCompleted,
		AgentName:   "assistant",
		ToolCallID:  "call_1",
		ToolCallRef: "tool_call:call_1",
		ToolName:    "echo",
		Content:     "echo: hello",
	})
	recorder.FinalizeCurrent()

	messages := recorder.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(messages))
	}
	if len(messages[0].Events) != 1 || messages[0].Events[0].ToolCall == nil {
		t.Fatalf("events = %#v, want one tool call", messages[0].Events)
	}
	toolCall := messages[0].Events[0].ToolCall
	if toolCall.Ref != "tool_call:call_1" {
		t.Fatalf("tool ref = %q, want tool_call:call_1", toolCall.Ref)
	}
	if toolCall.Result != "echo: hello" {
		t.Fatalf("tool result = %q, want echo: hello", toolCall.Result)
	}
}

func TestHistoryRecorderMergesToolResultByIDWhenRefDiffers(t *testing.T) {
	recorder := NewHistoryRecorder()
	toolIndex := 0

	recorder.RecordEvent(Event{
		Sequence:    1,
		Type:        EventToolCallStarted,
		AgentName:   "assistant",
		ToolCallRef: "ref_from_args",
		ToolCallRefs: map[int]string{
			0: "ref_from_args",
		},
		ToolCalls: []message.ToolCall{{
			ID:    "call_1",
			Index: &toolIndex,
			Function: message.FunctionCall{
				Name:      "echo",
				Arguments: `{"text":"hello"}`,
			},
		}},
	})
	recorder.RecordEvent(Event{
		Sequence:    2,
		Type:        EventToolCallCompleted,
		AgentName:   "assistant",
		ToolCallID:  "call_1",
		ToolCallRef: "ref_from_result",
		ToolName:    "echo",
		Content:     "echo: hello",
	})
	recorder.FinalizeCurrent()

	messages := recorder.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(messages))
	}
	if len(messages[0].Events) != 1 || messages[0].Events[0].ToolCall == nil {
		t.Fatalf("events = %#v, want one merged tool call", messages[0].Events)
	}
	toolCall := messages[0].Events[0].ToolCall
	if toolCall.Arguments != `{"text":"hello"}` {
		t.Fatalf("tool args = %q, want original args", toolCall.Arguments)
	}
	if toolCall.Result != "echo: hello" {
		t.Fatalf("tool result = %q, want echo: hello", toolCall.Result)
	}
}

func TestHistoryRecorderDoesNotDuplicateToolEndAndToolRoleMessageEnd(t *testing.T) {
	recorder := NewHistoryRecorder()
	toolIndex := 0

	recorder.RecordEvent(Event{
		Sequence:      1,
		Type:          EventToolCallStarted,
		AgentName:     "assistant",
		ToolCallID:    "call_1",
		ToolCallRef:   "ref_1",
		ToolName:      "echo",
		ToolArgs:      `{"text":"hello"}`,
		ToolCallIndex: &toolIndex,
	})
	recorder.RecordEvent(Event{
		Sequence:    2,
		Type:        EventToolCallCompleted,
		AgentName:   "assistant",
		ToolCallID:  "call_1",
		ToolCallRef: "ref_1",
		ToolName:    "echo",
		Content:     "echo: hello",
		ToolResult:  "echo: hello",
	})
	recorder.RecordEvent(Event{
		Sequence:    3,
		Type:        events.EventAssistantCompleted,
		Role:        message.RoleTool,
		AgentName:   "assistant",
		ToolCallID:  "call_1",
		ToolCallRef: "ref_1",
		ToolName:    "echo",
		Content:     "echo: hello",
	})
	recorder.FinalizeCurrent()

	messages := recorder.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(messages))
	}
	if len(messages[0].Events) != 1 {
		t.Fatalf("event count = %d, want 1: %#v", len(messages[0].Events), messages[0].Events)
	}
}

func hasEventType(msg AgentMessage, typ MsgEventType) bool {
	for _, event := range msg.Events {
		if event.Type == typ {
			return true
		}
	}
	return false
}
