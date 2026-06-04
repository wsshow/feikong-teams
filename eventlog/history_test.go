package eventlog

import (
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestHistoryRecorderKeepsParentToolCallBeforeMemberMessage(t *testing.T) {
	recorder := NewHistoryRecorder()
	toolIndex := 0

	recorder.RecordEvent(Event{
		Sequence:  1,
		Type:      EventToolCalls,
		AgentName: "coordinator",
		ToolCalls: []schema.ToolCall{{
			ID:    "call_1",
			Index: &toolIndex,
			Function: schema.FunctionCall{
				Name:      "ask_fkagent_researcher",
				Arguments: `{"task":"查资料"}`,
			},
		}},
	})
	recorder.RecordEvent(Event{
		Sequence:       2,
		Type:           EventStreamChunk,
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

func TestHistoryRecorderStoresUsageAsUsageEvent(t *testing.T) {
	recorder := NewHistoryRecorder()

	recorder.RecordEvent(Event{
		Sequence:  1,
		Type:      EventStreamChunk,
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
