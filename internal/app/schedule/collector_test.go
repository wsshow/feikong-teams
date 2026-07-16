package schedule

import (
	"strings"
	"testing"
	"unicode/utf8"

	"fkteams/internal/domain/event"
	"fkteams/internal/domain/message"
)

func TestMarkdownCollectorCollectsOutputDeltas(t *testing.T) {
	callback, result := newMarkdownCollector()

	if err := callback(event.Event{Type: event.TypeAssistantText, AgentName: "tasker", Content: "hello ", DeltaKind: event.DeltaOutput}); err != nil {
		t.Fatalf("callback: %v", err)
	}
	if err := callback(event.Event{Type: event.TypeAssistantText, AgentName: "tasker", Content: "world", DeltaKind: event.DeltaOutput}); err != nil {
		t.Fatalf("callback: %v", err)
	}
	if err := callback(event.Event{Type: event.TypeAssistantReasoning, AgentName: "tasker", Content: "hidden", DeltaKind: event.DeltaReasoning}); err != nil {
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

func TestMarkdownCollectorBoundsOutput(t *testing.T) {
	handle, result := newMarkdownCollectorWithLimit(128)
	if err := handle(event.Event{
		Type:      event.TypeAssistantText,
		Content:   strings.Repeat("你", 100),
		DeltaKind: event.DeltaOutput,
	}); err != nil {
		t.Fatal(err)
	}
	output := result()
	if len(output) > 128 {
		t.Fatalf("bounded output length = %d, want <= 128", len(output))
	}
	if !strings.Contains(output, "Output truncated") || !utf8.ValidString(output) {
		t.Fatalf("bounded output is invalid: %q", output)
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
		{Type: event.TypeToolCallStarted, AgentName: "tasker", ToolCall: &toolCall},
		{Type: event.TypeToolCallCompleted, ToolCallID: "call-1", Content: `{"message":"found 3 results"}`},
		{Type: event.TypeSystemNotice, AgentName: "tasker", Content: "researcher", Notice: &event.NoticePayload{Code: "transfer"}},
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
