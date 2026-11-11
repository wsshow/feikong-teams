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
		{Type: event.TypeSystemNotice, AgentName: "tasker", Content: "researcher", Notice: &event.NoticePayload{Code: event.NoticeTransfer}},
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

func TestMarkdownCollectorDeduplicatesToolRepresentations(t *testing.T) {
	handle, result := newMarkdownCollector()
	call := message.ToolCall{ID: "call", Function: message.FunctionCall{Name: "read"}}
	if err := handle(event.Event{Type: event.TypeToolCallStarted, ToolCall: &call, ToolCalls: []message.ToolCall{call}}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(result(), "tool:"); got != 1 {
		t.Fatalf("tool entries = %d", got)
	}
}

func TestTruncateMarkdownLinePreservesUTF8AndByteLimit(t *testing.T) {
	for _, value := range []string{"中文摘要 中文参数", "a中b文", "plain ascii"} {
		for limit := -1; limit <= len(value)+1; limit++ {
			got := truncateMarkdownLine(value, limit)
			if !utf8.ValidString(got) || len(got) > max(limit, 0) {
				t.Fatalf("truncate(%q, %d) = %q", value, limit, got)
			}
		}
	}
}
