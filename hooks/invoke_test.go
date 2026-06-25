package hooks

import (
	"context"
	"strings"
	"testing"

	"fkteams/agentcore"
)

func TestInvokeEventCanRewriteAndSkipDispatch(t *testing.T) {
	bus := NewBus()
	bus.RegisterFunc("rewrite-event", []HookPoint{HookOnEvent}, func(ctx Context, inv Invocation) (Result, error) {
		payload := inv.Payload.(EventPayload)
		payload.Event.Content = "rewritten"
		return Result{Payload: payload}, nil
	}, Options{})
	bus.RegisterFunc("skip-event", []HookPoint{HookOnEvent}, func(ctx Context, inv Invocation) (Result, error) {
		return Result{Action: ActionSkip}, nil
	}, Options{})

	event, emit, err := bus.InvokeEvent(context.Background(), agentcore.Event{Content: "original"})
	if err != nil {
		t.Fatalf("invoke event: %v", err)
	}
	if emit {
		t.Fatal("expected event dispatch to be skipped")
	}
	if event.Content != "rewritten" {
		t.Fatalf("event content = %q, want rewritten", event.Content)
	}
}

func TestInvokeBeforeToolCallCanReject(t *testing.T) {
	bus := NewBus()
	bus.RegisterFunc("reject-tool", []HookPoint{HookBeforeToolCall}, func(ctx Context, inv Invocation) (Result, error) {
		return Result{Action: ActionReject, Message: "blocked"}, nil
	}, Options{})

	_, err := bus.InvokeBeforeToolCall(context.Background(), BeforeToolCallPayload{ToolName: "command"})
	if err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("error = %v, want blocked reject error", err)
	}
}

func TestInvokeBeforeModelRequestCanRewriteMessages(t *testing.T) {
	bus := NewBus()
	bus.RegisterFunc("rewrite-model", []HookPoint{HookBeforeModelRequest}, func(ctx Context, inv Invocation) (Result, error) {
		payload := inv.Payload.(BeforeModelRequestPayload)
		payload.Messages = append(payload.Messages, agentcore.Message{
			Role:    agentcore.RoleSystem,
			Content: "extra",
		})
		return Result{Payload: payload}, nil
	}, Options{})

	messages, err := bus.InvokeBeforeModelRequest(context.Background(), []agentcore.Message{
		{Role: agentcore.RoleUser, Content: "ping"},
	})
	if err != nil {
		t.Fatalf("invoke before model request: %v", err)
	}
	if len(messages) != 2 || messages[1].Content != "extra" {
		t.Fatalf("messages = %#v, want appended system message", messages)
	}
}
