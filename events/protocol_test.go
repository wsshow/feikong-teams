package events

import (
	"fkteams/agentcore"
	"testing"
)

func TestToolCallRefAtUsesIndexThenPositionAndDoesNotSpreadTopLevelRef(t *testing.T) {
	firstIndex := 2
	event := Event{
		ToolCallRef: "top-level-ref",
		ToolCalls: []agentcore.ToolCall{
			{
				ID:    "call_1",
				Index: &firstIndex,
				Function: agentcore.FunctionCall{
					Name: "first",
				},
			},
			{
				ID: "call_2",
				Function: agentcore.FunctionCall{
					Name: "second",
				},
			},
		},
		ToolCallRefs: map[int]string{
			0: "position-ref",
			2: "index-ref",
		},
	}

	if got := ToolCallRefAt(event, event.ToolCalls[0], 0); got != "index-ref" {
		t.Fatalf("first ref = %q, want index-ref", got)
	}
	if got := ToolCallRefAt(event, event.ToolCalls[1], 1); got != "" {
		t.Fatalf("second ref = %q, want empty", got)
	}

	event.ToolCall = &event.ToolCalls[0]
	if got := ToolCallRefAt(event, event.ToolCalls[0], 0); got != "index-ref" {
		t.Fatalf("single tool ref = %q, want index-ref", got)
	}
}

func TestValidateEventContractRequiresStableToolIdentity(t *testing.T) {
	if err := ValidateEventContract(Event{
		Type:     EventToolStart,
		ToolName: "search",
	}); err == nil {
		t.Fatalf("tool_start without identity should fail")
	}

	if err := ValidateEventContract(Event{
		Type:        EventToolStart,
		ToolCallID:  "call_1",
		ToolCallRef: "tool_call:call_1",
		ToolName:    "search",
	}); err != nil {
		t.Fatalf("tool_start with identity failed: %v", err)
	}
}

func TestValidateEventContractRequiresMessageEndToolCallRefs(t *testing.T) {
	if err := ValidateEventContract(Event{
		Type: EventMessageEnd,
		ToolCalls: []agentcore.ToolCall{{
			ID: "call_1",
			Function: agentcore.FunctionCall{
				Name: "search",
			},
		}},
	}); err == nil {
		t.Fatalf("message_end tool call without ref should fail")
	}

	if err := ValidateEventContract(Event{
		Type: EventMessageEnd,
		ToolCalls: []agentcore.ToolCall{{
			ID: "call_1",
			Function: agentcore.FunctionCall{
				Name: "search",
			},
		}},
		ToolCallRefs: map[int]string{0: "tool_call:call_1"},
	}); err != nil {
		t.Fatalf("message_end with tool refs failed: %v", err)
	}
}
