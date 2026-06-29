package chat

import (
	"context"
	"testing"

	"fkteams/internal/app/tools/ask"
	"fkteams/internal/domain/event"
	"fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/approval"
)

func TestRunTurnDelegatesToRunnerAndPublishesEvents(t *testing.T) {
	runner := &fakeRunner{}
	service := NewService()
	var gotEvents []event.Event

	result, err := service.RunTurn(context.Background(), TurnRequest{
		SessionID: "session-1",
		Runner:    runner,
		Input: message.TurnInput{
			Message: message.Message{Role: message.RoleUser, Content: "ping"},
		},
	},
		WithRunID("run-1"),
		OnEvent(func(event event.Event) error {
			gotEvents = append(gotEvents, event)
			return nil
		}),
	)
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
	if len(gotEvents) != 1 || gotEvents[0].Type != event.TypeAssistantCompleted {
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

func TestRunTurnRecordsEventsBeforePublishing(t *testing.T) {
	runner := &fakeRunner{}
	var calls []string
	recorder := EventRecorderFunc(func(event.Event) {
		calls = append(calls, "record")
	})

	_, err := NewService().RunTurn(context.Background(), TurnRequest{
		SessionID: "session-1",
		Runner:    runner,
		Input:     message.TurnInput{Message: message.Message{Role: message.RoleUser, Content: "ping"}},
	},
		WithEventRecorder(recorder),
		OnEvent(func(event.Event) error {
			calls = append(calls, "publish")
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("run turn: %v", err)
	}
	if len(calls) != 2 || calls[0] != "record" || calls[1] != "publish" {
		t.Fatalf("event calls = %#v, want record then publish", calls)
	}
}

func TestRunTurnInjectsTypedRuntimeCapabilities(t *testing.T) {
	runner := &contextProbeRunner{}
	steeringSource := func(context.Context) ([]message.Message, error) {
		return []message.Message{{Role: message.RoleUser, Content: "steer"}}, nil
	}
	askHandler := func(context.Context, ask.RuntimeRequest) (*ask.AskResponse, error) {
		return &ask.AskResponse{FreeText: "answer"}, nil
	}

	_, err := NewService().RunTurn(context.Background(), TurnRequest{
		SessionID: "session-1",
		Runner:    runner,
		Input:     message.TurnInput{Message: message.Message{Role: message.RoleUser, Content: "ping"}},
	},
		WithApprovalRegistry(approval.NewAutoApproveRegistry()),
		WithSteeringSource(steeringSource),
		WithAskRuntimeHandler(askHandler),
	)
	if err != nil {
		t.Fatalf("run turn: %v", err)
	}
	if !runner.sawApproval {
		t.Fatal("runner did not observe approval registry")
	}
	if runner.steeringContent != "steer" {
		t.Fatalf("steering content = %q, want steer", runner.steeringContent)
	}
	if runner.askAnswer != "answer" {
		t.Fatalf("ask answer = %q, want answer", runner.askAnswer)
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
			Type:    event.TypeAssistantCompleted,
			Content: "done",
		}); err != nil {
			return nil, err
		}
	}
	return &runtimeport.RunResult{LastEvent: event.Event{Type: event.TypeAssistantCompleted}}, nil
}

type contextProbeRunner struct {
	sawApproval     bool
	steeringContent string
	askAnswer       string
}

func (r *contextProbeRunner) Run(ctx context.Context, input message.TurnInput, opts runtimeport.RunOptions) (*runtimeport.RunResult, error) {
	if err := approval.Require(ctx, approval.StoreCommand, "echo", "echo"); err != nil {
		return nil, err
	}
	r.sawApproval = true

	source, ok := runtimeport.SteeringSourceFromContext(ctx)
	if !ok {
		tail := event.Event{Type: event.TypeError, Error: "missing steering source"}
		return &runtimeport.RunResult{LastEvent: tail}, nil
	}
	messages, err := source(ctx)
	if err != nil {
		return nil, err
	}
	if len(messages) > 0 {
		r.steeringContent = messages[0].Content
	}

	askCtx := runtimeport.WithInterruptMetadata(ctx, runtimeport.InterruptMetadata{MemberCallID: "member-1"})
	resp, err := ask.AskQuestions(askCtx, &ask.AskRequest{Question: "continue?"})
	if err != nil {
		return nil, err
	}
	r.askAnswer = resp.FreeText
	return &runtimeport.RunResult{LastEvent: event.Event{Type: event.TypeAssistantCompleted}}, nil
}
