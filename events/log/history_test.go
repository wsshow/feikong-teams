package eventlog

import (
	"fkteams/agentcore"
	"fkteams/events"
	"testing"
)

func TestHistoryRecorderKeepsParentToolCallBeforeMemberMessage(t *testing.T) {
	recorder := NewHistoryRecorder()
	toolIndex := 0

	recorder.RecordEvent(Event{
		Sequence:    1,
		Type:        EventToolStart,
		AgentName:   "coordinator",
		ToolCallID:  "call_1",
		ToolCallRef: "tool_call:call_1",
		ToolCallRefs: map[int]string{
			0: "tool_call:call_1",
		},
		ToolCalls: []agentcore.ToolCall{{
			ID:    "call_1",
			Index: &toolIndex,
			Function: agentcore.FunctionCall{
				Name:      "ask_fkagent_researcher",
				Arguments: `{"task":"查资料"}`,
			},
		}},
	})
	recorder.RecordEvent(Event{
		Sequence:       2,
		Type:           EventMessageDelta,
		Role:           agentcore.RoleAssistant,
		DeltaKind:      agentcore.DeltaOutput,
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
		Type:           EventMessageDelta,
		Role:           agentcore.RoleAssistant,
		DeltaKind:      agentcore.DeltaReasoning,
		AgentName:      "ask-member",
		Content:        "thinking",
		MemberCallID:   "member-call-1",
		MemberToolName: "ask_fkagent_member",
		MemberName:     "Ask Member",
		MemberOrder:    &memberOrder,
	})
	recorder.RecordEvent(Event{
		Sequence:       11,
		Type:           EventMessageDelta,
		Role:           agentcore.RoleAssistant,
		DeltaKind:      agentcore.DeltaOutput,
		AgentName:      "ask-member",
		Content:        "about to ask",
		MemberCallID:   "member-call-1",
		MemberToolName: "ask_fkagent_member",
		MemberName:     "Ask Member",
		MemberOrder:    &memberOrder,
	})
	recorder.RecordEvent(Event{
		Sequence:       12,
		Type:           EventAction,
		ActionType:     events.ActionAskQuestions,
		AgentName:      "ask-member",
		Content:        "Choose?",
		Detail:         "ask-1",
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
		Type:      EventMessageDelta,
		Role:      agentcore.RoleAssistant,
		DeltaKind: agentcore.DeltaOutput,
		AgentName: "coordinator",
		Content:   "ok",
	})
	recorder.RecordEvent(Event{
		Sequence:         2,
		Type:             EventUsage,
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
	if usageEvent.Type != MsgTypeUsage {
		t.Fatalf("usage event type = %q, want %q", usageEvent.Type, MsgTypeUsage)
	}
	if usageEvent.Action != nil {
		t.Fatalf("usage event action = %#v, want nil", usageEvent.Action)
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
		Type:        EventToolStart,
		AgentName:   "coordinator",
		ToolCallRef: "tool_call:call_1",
		ToolCalls: []agentcore.ToolCall{{
			ID:    "call_1",
			Index: &toolIndex,
			Function: agentcore.FunctionCall{
				Name:      "ask_fkagent_researcher",
				Arguments: `{"task":"查资料"}`,
			},
		}},
	})
	recorder.RecordEvent(Event{
		Sequence:       2,
		Type:           EventMessageDelta,
		Role:           agentcore.RoleAssistant,
		DeltaKind:      agentcore.DeltaReasoning,
		AgentName:      "researcher",
		Content:        "working",
		MemberCallID:   "call_1",
		MemberToolName: "ask_fkagent_researcher",
		MemberName:     "Researcher",
		MemberOrder:    &toolIndex,
	})

	recorder.RecordCancelled("任务已取消")

	messages := recorder.GetMessages()
	if len(messages) != 3 {
		t.Fatalf("message count = %d, want 3", len(messages))
	}
	if !hasEventType(messages[0], MsgTypeCancelled) {
		t.Fatalf("coordinator events = %#v, want cancelled", messages[0].Events)
	}
	if !hasEventType(messages[1], MsgTypeCancelled) {
		t.Fatalf("member events = %#v, want cancelled", messages[1].Events)
	}
	if messages[2].AgentName != "系统" || !hasEventType(messages[2], MsgTypeCancelled) {
		t.Fatalf("system message = %#v, want cancelled notice", messages[2])
	}
}

func TestHistoryRecorderRecordsToolRoleMessageEndAsToolResult(t *testing.T) {
	recorder := NewHistoryRecorder()
	toolIndex := 0

	recorder.RecordEvent(Event{
		Sequence:      1,
		Type:          EventToolStart,
		AgentName:     "assistant",
		ToolCallID:    "call_1",
		ToolCallRef:   "ref_1",
		ToolName:      "echo",
		ToolArgs:      `{"text":"hello"}`,
		ToolCallIndex: &toolIndex,
	})
	recorder.RecordEvent(Event{
		Sequence:    2,
		Type:        agentcore.EventMessageEnd,
		Role:        agentcore.RoleTool,
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
		Type:      agentcore.EventMessageEnd,
		Role:      agentcore.RoleAssistant,
		AgentName: "assistant",
		ToolCalls: []agentcore.ToolCall{
			{
				ID: "call_1",
				Function: agentcore.FunctionCall{
					Name:      "echo",
					Arguments: `{"text":"hello"}`,
				},
			},
		},
		ToolCallRefs: map[int]string{0: "tool_call:call_1"},
	})
	recorder.RecordEvent(Event{
		Sequence:    2,
		Type:        agentcore.EventMessageEnd,
		Role:        agentcore.RoleTool,
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
		Type:        EventToolStart,
		AgentName:   "assistant",
		ToolCallRef: "ref_from_args",
		ToolCallRefs: map[int]string{
			0: "ref_from_args",
		},
		ToolCalls: []agentcore.ToolCall{{
			ID:    "call_1",
			Index: &toolIndex,
			Function: agentcore.FunctionCall{
				Name:      "echo",
				Arguments: `{"text":"hello"}`,
			},
		}},
	})
	recorder.RecordEvent(Event{
		Sequence:    2,
		Type:        EventToolEnd,
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
		Type:          EventToolStart,
		AgentName:     "assistant",
		ToolCallID:    "call_1",
		ToolCallRef:   "ref_1",
		ToolName:      "echo",
		ToolArgs:      `{"text":"hello"}`,
		ToolCallIndex: &toolIndex,
	})
	recorder.RecordEvent(Event{
		Sequence:    2,
		Type:        EventToolEnd,
		AgentName:   "assistant",
		ToolCallID:  "call_1",
		ToolCallRef: "ref_1",
		ToolName:    "echo",
		Content:     "echo: hello",
		ToolResult:  "echo: hello",
	})
	recorder.RecordEvent(Event{
		Sequence:    3,
		Type:        agentcore.EventMessageEnd,
		Role:        agentcore.RoleTool,
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
