package events

import (
	"errors"
	"testing"

	"fkteams/internal/domain/message"
)

func TestEmitterFillsRunTurnNormalizesAndSends(t *testing.T) {
	var got []Event
	emitter := NewEmitter("run_1", "turn_1", func(event Event) error {
		got = append(got, event)
		return nil
	})

	event := AssistantDelta(MessageEvent{
		MessageID: "msg_1",
		Role:      message.RoleAssistant,
		AgentName: "coder",
		RunPath:   "root/coder",
		DeltaKind: DeltaOutput,
	}, "hello")
	if err := emitter.Emit(event); err != nil {
		t.Fatalf("emit message delta: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("sink events = %d, want 1", len(got))
	}
	if got[0].RunID != "run_1" || got[0].TurnID != "turn_1" {
		t.Fatalf("run/turn id = %q/%q", got[0].RunID, got[0].TurnID)
	}
	if got[0].EventID == "" || got[0].Sequence == 0 || got[0].CreatedAt.IsZero() {
		t.Fatalf("event was not normalized: %#v", got[0])
	}
	if emitter.LastEvent().Content != "hello" {
		t.Fatalf("last event = %#v", emitter.LastEvent())
	}

	override := TurnStart("run_override", "turn_override")
	if err := emitter.Emit(override); err != nil {
		t.Fatalf("emit override event: %v", err)
	}
	if got[1].RunID != "run_override" || got[1].TurnID != "turn_override" {
		t.Fatalf("explicit run/turn should be preserved: %#v", got[1])
	}
}

func TestEmitterUsesNoopSinkAndValidatesContract(t *testing.T) {
	emitter := NewEmitter("run", "turn", nil)
	if err := emitter.Emit(AgentStart("")); err != nil {
		t.Fatalf("noop sink emit: %v", err)
	}

	err := emitter.Emit(ToolCallStarted(ToolEvent{ToolName: "search"}))
	if err == nil {
		t.Fatal("expected missing tool identity error")
	}
	if emitter.LastEvent().Type != EventToolCallStarted {
		t.Fatalf("last event should still reflect failed event, got %#v", emitter.LastEvent())
	}

	var nilEmitter *Emitter
	if got := nilEmitter.LastEvent(); got.Type != "" {
		t.Fatalf("nil emitter last event = %#v", got)
	}
}

func TestEventConstructors(t *testing.T) {
	toolIndex := 3
	toolCall := &message.ToolCall{
		ID:    "call_1",
		Index: &toolIndex,
		Function: message.FunctionCall{
			Name:      "search",
			Arguments: `{"query":"go"}`,
		},
	}

	constructors := []Event{
		AgentStart("run_1"),
		AgentEnd("run_1"),
		AgentError("run_1", errors.New("boom")),
		TurnStart("run_1", "turn_1"),
		TurnEnd("run_1", "turn_1"),
		AssistantStarted(MessageEvent{
			MessageID:   "msg_1",
			Role:        message.RoleAssistant,
			AgentName:   "coder",
			RunPath:     "root/coder",
			Content:     "start",
			ToolCallID:  "call_1",
			ToolCallRef: "ref_1",
			ToolName:    "search",
		}),
		AssistantDelta(MessageEvent{
			MessageID:   "msg_1",
			Role:        message.RoleAssistant,
			DeltaKind:   DeltaToolArgs,
			ToolCallID:  "call_1",
			ToolCallRef: "ref_1",
			ToolName:    "search",
		}, `{"query":"go"}`),
		AssistantCompleted(MessageEvent{
			MessageID:        "msg_1",
			Role:             message.RoleAssistant,
			Content:          "done",
			ReasoningContent: "reason",
			ToolCalls:        []message.ToolCall{*toolCall},
			ToolCallRefs:     map[int]string{toolIndex: "ref_1"},
		}),
		ToolCallStarted(ToolEvent{
			AgentName:     "coder",
			RunPath:       "root/coder",
			ToolCallID:    "call_1",
			ToolCallRef:   "ref_1",
			ToolName:      "search",
			ToolArgs:      `{"query":"go"}`,
			ToolCall:      toolCall,
			ToolCallIndex: &toolIndex,
		}),
		ToolCallResultDelta(ToolEvent{
			ToolCallID:  "call_1",
			ToolCallRef: "ref_1",
			ToolName:    "search",
			Content:     "partial",
		}),
		ToolCallCompleted(ToolEvent{
			ToolCallID:  "call_1",
			ToolCallRef: "ref_1",
			ToolName:    "search",
			ToolResult:  "result",
		}),
		SystemNotice("coder", "root/coder", "interrupted", "paused"),
		Error("coder", "root/coder", errors.New("failed")),
		Usage("coder", "root/coder", 1, 2, 3),
	}

	wantTypes := []EventType{
		EventAgentStarted,
		EventAgentCompleted,
		EventAgentCompleted,
		EventTurnStarted,
		EventTurnCompleted,
		EventAssistantStarted,
		EventToolCallArguments,
		EventAssistantCompleted,
		EventToolCallStarted,
		EventToolCallResult,
		EventToolCallCompleted,
		EventSystemNotice,
		EventError,
		EventUsageReported,
	}
	for i, event := range constructors {
		if event.Type != wantTypes[i] {
			t.Fatalf("event %d type = %q, want %q", i, event.Type, wantTypes[i])
		}
	}

	if constructors[2].Error != "boom" {
		t.Fatalf("AgentError error = %q", constructors[2].Error)
	}
	if constructors[8].Content != constructors[8].ToolArgs {
		t.Fatalf("ToolCallStarted content should default to args: %#v", constructors[8])
	}
	if constructors[9].DeltaKind != DeltaToolResult {
		t.Fatalf("ToolCallResultDelta delta kind = %q", constructors[9].DeltaKind)
	}
	if constructors[10].Content != "result" || constructors[10].ToolResult != "result" {
		t.Fatalf("ToolCallCompleted result fields = %#v", constructors[10])
	}
	if constructors[13].PromptTokens != 1 || constructors[13].CompletionTokens != 2 || constructors[13].TotalTokens != 3 {
		t.Fatalf("Usage tokens = %#v", constructors[13])
	}
}

func TestUserMessageTurnIDAndFirstNonEmpty(t *testing.T) {
	msg := message.Message{
		Role: message.RoleUser,
		ContentParts: []message.ContentPart{
			{Type: message.ContentPartText, Text: "hello"},
			{Type: message.ContentPartImageURL, URL: "https://example.com/a.png"},
			{Type: message.ContentPartText, Text: "world"},
		},
	}
	event := UserMessage("run_1", "turn_1", "msg_1", msg)
	if event.Type != EventUserMessage {
		t.Fatalf("event type = %q", event.Type)
	}
	if event.RunID != "run_1" || event.TurnID != "turn_1" || event.Content != "hello world" {
		t.Fatalf("user event = %#v", event)
	}
	if event.Message == nil || event.Message.Role != message.RoleUser {
		t.Fatalf("event message = %#v", event.Message)
	}
	if TurnID("run_1", 7) != "run_1:turn:7" {
		t.Fatalf("TurnID returned unexpected value")
	}
	if firstNonEmpty("", "a", "b") != "a" || firstNonEmpty("", "") != "" {
		t.Fatalf("firstNonEmpty returned unexpected value")
	}
}
