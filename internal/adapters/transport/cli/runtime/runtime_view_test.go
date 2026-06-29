package runtime

import (
	"fkteams/internal/runtime/events"
	"testing"
)

func TestRuntimeDeltaMergeKeepsSameMessageTogether(t *testing.T) {
	base := events.Event{
		Type:      events.EventAssistantText,
		DeltaKind: events.DeltaOutput,
		AgentName: "assistant",
		MessageID: "msg-1",
		Content:   "hel",
	}
	next := events.Event{
		Type:      events.EventAssistantText,
		DeltaKind: events.DeltaOutput,
		AgentName: "assistant",
		MessageID: "msg-1",
		Content:   "lo",
	}

	if !runtimeCanMergeDelta(base, next) {
		t.Fatal("same message output deltas should merge")
	}
	runtimeMergeDelta(&base, next)
	if base.Content != "hello" {
		t.Fatalf("expected merged content, got %q", base.Content)
	}
}

func TestRuntimeDeltaMergeRejectsDifferentMessages(t *testing.T) {
	base := events.Event{
		Type:      events.EventAssistantText,
		DeltaKind: events.DeltaOutput,
		AgentName: "assistant",
		MessageID: "msg-1",
		Content:   "one",
	}
	next := events.Event{
		Type:      events.EventAssistantText,
		DeltaKind: events.DeltaOutput,
		AgentName: "assistant",
		MessageID: "msg-2",
		Content:   "two",
	}

	if runtimeCanMergeDelta(base, next) {
		t.Fatal("different messages should not merge")
	}
}
