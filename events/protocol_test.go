package events

import (
	"fkteams/internal/domain/message"
	"testing"
)

func TestToolCallRefAtUsesIndexThenPositionAndDoesNotSpreadTopLevelRef(t *testing.T) {
	firstIndex := 2
	event := Event{
		ToolCallRef: "top-level-ref",
		ToolCalls: []message.ToolCall{
			{
				ID:    "call_1",
				Index: &firstIndex,
				Function: message.FunctionCall{
					Name: "first",
				},
			},
			{
				ID: "call_2",
				Function: message.FunctionCall{
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

func TestToolCallsFromEventPrependsSingleToolCall(t *testing.T) {
	single := message.ToolCall{
		ID: "call_1",
		Function: message.FunctionCall{
			Name: "first",
		},
	}
	event := Event{
		ToolCall: &single,
		ToolCalls: []message.ToolCall{{
			ID: "call_2",
			Function: message.FunctionCall{
				Name: "second",
			},
		}},
	}

	got := ToolCallsFromEvent(event)
	if len(got) != 2 || got[0].ID != "call_1" || got[1].ID != "call_2" {
		t.Fatalf("ToolCallsFromEvent = %#v", got)
	}

	event.ToolCall = nil
	got = ToolCallsFromEvent(event)
	if len(got) != 1 || got[0].ID != "call_2" {
		t.Fatalf("ToolCallsFromEvent without single call = %#v", got)
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
		ToolCalls: []message.ToolCall{{
			ID: "call_1",
			Function: message.FunctionCall{
				Name: "search",
			},
		}},
	}); err == nil {
		t.Fatalf("message_end tool call without ref should fail")
	}

	if err := ValidateEventContract(Event{
		Type: EventMessageEnd,
		ToolCalls: []message.ToolCall{{
			ID: "call_1",
			Function: message.FunctionCall{
				Name: "search",
			},
		}},
		ToolCallRefs: map[int]string{0: "tool_call:call_1"},
	}); err != nil {
		t.Fatalf("message_end with tool refs failed: %v", err)
	}
}

func TestValidateEventContractRequiresToolIdentityForUpdatesAndDeltas(t *testing.T) {
	invalidEvents := []Event{
		{Type: EventToolUpdate, ToolCallID: "call_1"},
		{Type: EventToolEnd, ToolCallRef: "ref_1"},
		{Type: EventMessageDelta, DeltaKind: DeltaToolArgs, ToolCallID: "call_1"},
		{Type: EventMessageDelta, DeltaKind: DeltaToolResult, ToolCallRef: "ref_1"},
		{Type: EventMessageEnd, Role: message.RoleTool, ToolCallID: "call_1"},
	}
	for i, event := range invalidEvents {
		if err := ValidateEventContract(event); err == nil {
			t.Fatalf("invalid event %d unexpectedly passed: %#v", i, event)
		}
	}

	validEvents := []Event{
		{Type: EventToolUpdate, ToolCallID: "call_1", ToolCallRef: "ref_1"},
		{Type: EventToolEnd, ToolCallID: "call_1", ToolCallRef: "ref_1"},
		{Type: EventMessageDelta, DeltaKind: DeltaToolArgs, ToolCallID: "call_1", ToolCallRef: "ref_1"},
		{Type: EventMessageDelta, DeltaKind: DeltaToolResult, ToolCallID: "call_1", ToolCallRef: "ref_1"},
		{Type: EventMessageEnd, Role: message.RoleTool, ToolCallID: "call_1", ToolCallRef: "ref_1"},
		{Type: EventMessageDelta, DeltaKind: DeltaOutput},
	}
	for i, event := range validEvents {
		if err := ValidateEventContract(event); err != nil {
			t.Fatalf("valid event %d failed: %v", i, err)
		}
	}
}

func TestValidateEventContractSkipsInternalToolCallsAndRequiresIDs(t *testing.T) {
	if err := ValidateEventContract(Event{
		Type: EventMessageEnd,
		ToolCalls: []message.ToolCall{{
			Function: message.FunctionCall{Name: "continue_output"},
		}},
	}); err != nil {
		t.Fatalf("internal tool call should be skipped: %v", err)
	}

	if err := ValidateEventContract(Event{
		Type: EventMessageEnd,
		ToolCalls: []message.ToolCall{{
			Function: message.FunctionCall{Name: "search"},
		}},
		ToolCallRefs: map[int]string{0: "ref_1"},
	}); err == nil {
		t.Fatal("missing tool call id should fail")
	}
}
