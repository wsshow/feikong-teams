package eino

import (
	"strings"
	"testing"

	domainevent "fkteams/internal/domain/event"
	domainmessage "fkteams/internal/domain/message"
)

func TestToolIdentityAttachMapsUnknownResultIDToPendingCall(t *testing.T) {
	tracker := newToolIdentityTracker()
	index := 0
	call := domainmessage.ToolCall{
		Index: &index,
		Function: domainmessage.FunctionCall{
			Name:      "echo",
			Arguments: `{"text":"hello"}`,
		},
	}
	ref := tracker.ensure("message-1", 0, MemberScope{}, &call)
	tracker.rememberResult(call.Function.Name, call.ID)

	event := &domainevent.Event{
		Type:       domainevent.TypeToolCallCompleted,
		ToolCallID: "provider-real-id",
		ToolName:   "echo",
	}
	tracker.attach(event)

	if !strings.HasPrefix(call.ID, "fk_tool_") {
		t.Fatalf("generated id = %q, want fk_tool_ prefix", call.ID)
	}
	if event.ToolCallID != call.ID {
		t.Fatalf("event tool id = %q, want normalized id %q", event.ToolCallID, call.ID)
	}
	if event.ToolCallRef != ref {
		t.Fatalf("event tool ref = %q, want %q", event.ToolCallRef, ref)
	}
}
