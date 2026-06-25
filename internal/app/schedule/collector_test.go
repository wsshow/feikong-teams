package schedule

import (
	"strings"
	"testing"

	"fkteams/internal/domain/event"
	"fkteams/internal/domain/message"
)

func TestMarkdownCollectorCollectsOutputDeltas(t *testing.T) {
	callback, result := newMarkdownCollector()

	if err := callback(event.Event{Type: event.TypeMessageDelta, AgentName: "tasker", Content: "hello ", DeltaKind: event.DeltaOutput}); err != nil {
		t.Fatalf("callback: %v", err)
	}
	if err := callback(event.Event{Type: event.TypeMessageDelta, AgentName: "tasker", Content: "world", DeltaKind: event.DeltaOutput}); err != nil {
		t.Fatalf("callback: %v", err)
	}
	if err := callback(event.Event{Type: event.TypeMessageDelta, AgentName: "tasker", Content: "hidden", DeltaKind: event.DeltaReasoning}); err != nil {
		t.Fatalf("callback: %v", err)
	}

	got := result()
	if !strings.Contains(got, "**[tasker]**") {
		t.Fatalf("result missing agent heading: %q", got)
	}
	if !strings.Contains(got, "hello world") {
		t.Fatalf("result missing output delta: %q", got)
	}
	if strings.Contains(got, "hidden") {
		t.Fatalf("result contains non-output delta: %q", got)
	}
}

func TestMarkdownCollectorCollectsToolsActionsAndErrors(t *testing.T) {
	callback, result := newMarkdownCollector()

	toolCall := message.ToolCall{
		ID: "call-1",
		Function: message.FunctionCall{
			Name:      "search",
			Arguments: `{"query":"architecture"}`,
		},
	}
	events := []event.Event{
		{Type: event.TypeToolStart, AgentName: "tasker", ToolCall: &toolCall},
		{Type: event.TypeToolEnd, ToolCallID: "call-1", Content: `{"message":"found 3 results"}`},
		{Type: event.TypeAction, AgentName: "tasker", ActionType: event.ActionTransfer, Content: "researcher"},
		{Type: event.TypeError, AgentName: "tasker", Error: "failed"},
	}
	for _, e := range events {
		if err := callback(e); err != nil {
			t.Fatalf("callback: %v", err)
		}
	}

	got := result()
	for _, want := range []string{
		"tool: `search`",
		"args: `{\"query\":\"architecture\"}`",
		"`search`: found 3 results",
		"-> researcher",
		"**Error [tasker]**: failed",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("result missing %q: %s", want, got)
		}
	}
}
