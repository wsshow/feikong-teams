package chat

import (
	"context"
	"testing"

	"fkteams/internal/domain/event"
	"fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
)

func TestRunTurnDelegatesToRunnerAndPublishesEvents(t *testing.T) {
	runner := &fakeRunner{}
	service := NewService()
	var gotEvents []event.Event

	result, err := service.RunTurn(context.Background(), TurnRequest{
		SessionID: "session-1",
		RunID:     "run-1",
		Runner:    runner,
		Input: message.TurnInput{
			Message: message.Message{Role: message.RoleUser, Content: "ping"},
		},
		EventHandler: func(event event.Event) error {
			gotEvents = append(gotEvents, event)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("run turn: %v", err)
	}
	if result == nil {
		t.Fatal("expected run result")
	}
	if runner.input.Message.Content != "ping" {
		t.Fatalf("runner input = %#v", runner.input)
	}
	if runner.opts.RunID != "run-1" || runner.opts.CheckpointID != "session-1" {
		t.Fatalf("run options = %#v", runner.opts)
	}
	if len(gotEvents) != 1 || gotEvents[0].Type != event.TypeMessageEnd {
		t.Fatalf("events = %#v, want message_end", gotEvents)
	}
}

func TestRunTurnRejectsMissingDependencies(t *testing.T) {
	service := NewService()
	if _, err := service.RunTurn(context.Background(), TurnRequest{SessionID: "s"}); err == nil {
		t.Fatal("expected missing runner error")
	}
	if _, err := service.RunTurn(context.Background(), TurnRequest{Runner: &fakeRunner{}}); err == nil {
		t.Fatal("expected missing session ID error")
	}
}

type fakeRunner struct {
	input message.TurnInput
	opts  runtimeport.RunOptions
}

func (r *fakeRunner) Run(ctx context.Context, input message.TurnInput, opts runtimeport.RunOptions) (*runtimeport.RunResult, error) {
	r.input = input
	r.opts = opts
	opts = opts.WithDefaults(opts.CheckpointID)
	if opts.Sink != nil {
		if err := opts.Sink(event.Event{
			Type:    event.TypeMessageEnd,
			Content: "done",
		}); err != nil {
			return nil, err
		}
	}
	return &runtimeport.RunResult{LastEvent: event.Event{Type: event.TypeMessageEnd}}, nil
}
