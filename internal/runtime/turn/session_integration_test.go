package turn

import (
	"context"
	"fkteams/agentcore"
	"fkteams/events"
	"fkteams/hooks"
	"testing"
)

type recordingRunner struct {
	input agentcore.TurnInput
}

func (r *recordingRunner) Run(_ context.Context, input agentcore.TurnInput, opts agentcore.RunOptions) (*agentcore.RunResult, error) {
	r.input = input
	event := agentcore.Event{
		Type:      agentcore.EventMessageDelta,
		Role:      agentcore.RoleAssistant,
		DeltaKind: agentcore.DeltaOutput,
		Content:   "pong",
	}
	if opts.Sink != nil {
		if err := opts.Sink(event); err != nil {
			return nil, err
		}
	}
	return &agentcore.RunResult{LastEvent: event}, nil
}

func TestSessionRunsCoreRunner(t *testing.T) {
	ctx := context.Background()
	r := &recordingRunner{}

	var collectedEvents []events.Event
	_, err := NewSession(r, "test-session").
		WithInput(TurnInput{Message: agentcore.Message{Role: agentcore.RoleUser, Content: "ping"}}).
		OnEvent(func(event events.Event) error {
			collectedEvents = append(collectedEvents, event)
			return nil
		}).
		Run(ctx)
	if err != nil {
		t.Fatalf("run session: %v", err)
	}

	messages := r.input.AllMessages()
	if len(messages) == 0 || messages[len(messages)-1].Content != "ping" {
		t.Fatalf("expected user input to reach runner, got %#v", messages)
	}

	found := false
	for _, event := range collectedEvents {
		if event.Content == "pong" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected pong event, got %#v", collectedEvents)
	}
}

func TestSessionInvokesRunHooks(t *testing.T) {
	ctx := context.Background()
	r := &recordingRunner{}
	bus := hooks.NewBus()
	afterCalled := false

	bus.RegisterFunc("rewrite-input", []hooks.HookPoint{hooks.HookBeforeRun}, func(ctx hooks.Context, inv hooks.Invocation) (hooks.Result, error) {
		payload := inv.Payload.(hooks.BeforeRunPayload)
		payload.Input.Message.Content = "hooked"
		return hooks.Result{Payload: payload}, nil
	}, hooks.Options{})
	bus.RegisterFunc("after-run", []hooks.HookPoint{hooks.HookAfterRun}, func(ctx hooks.Context, inv hooks.Invocation) (hooks.Result, error) {
		payload := inv.Payload.(hooks.AfterRunPayload)
		if payload.Input.Message.Content != "hooked" {
			t.Fatalf("after input = %q, want hooked", payload.Input.Message.Content)
		}
		if payload.Result == nil {
			t.Fatal("after hook result is nil")
		}
		afterCalled = true
		return hooks.Result{}, nil
	}, hooks.Options{})

	_, err := NewSession(r, "test-session").
		WithHookBus(bus).
		WithInput(TurnInput{Message: agentcore.Message{Role: agentcore.RoleUser, Content: "ping"}}).
		Run(ctx)
	if err != nil {
		t.Fatalf("run session: %v", err)
	}
	if r.input.Message.Content != "hooked" {
		t.Fatalf("runner input = %q, want hooked", r.input.Message.Content)
	}
	if !afterCalled {
		t.Fatal("after hook was not called")
	}
}
