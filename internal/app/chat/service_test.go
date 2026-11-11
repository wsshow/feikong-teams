package chat

import (
	"context"
	"errors"
	"testing"

	"fkteams/internal/app/tools/ask"
	"fkteams/internal/domain/event"
	"fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/approval"
	"fkteams/internal/runtime/hooks"
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
		EventSink: func(event event.Event) error {
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
	if runner.opts.RunID != "run-1" {
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
	history := &recordingHistory{calls: &calls}

	_, err := NewService().RunTurn(context.Background(), TurnRequest{
		SessionID: "session-1",
		Runner:    runner,
		Input:     message.TurnInput{Message: message.Message{Role: message.RoleUser, Content: "ping"}},
		History:   history,
		EventSink: func(event.Event) error {
			calls = append(calls, "publish")
			return nil
		},
	})
	if err != nil {
		t.Fatalf("run turn: %v", err)
	}
	if len(calls) != 2 || calls[0] != "record" || calls[1] != "publish" {
		t.Fatalf("event calls = %#v, want record then publish", calls)
	}
}

type recordingHistory struct {
	failingHistory
	calls *[]string
}

func (h *recordingHistory) RecordEvent(event.Event) { *h.calls = append(*h.calls, "record") }

func TestRunTurnUsesHookBusFromContext(t *testing.T) {
	runner := &fakeRunner{}
	bus := hooks.NewBus()
	bus.RegisterFunc("rewrite", []hooks.HookPoint{hooks.HookBeforeRun}, func(_ hooks.Context, inv hooks.Invocation) (hooks.Result, error) {
		payload := inv.Payload.(hooks.BeforeRunPayload)
		payload.Input.Message.Content = "rewritten"
		return hooks.Result{Payload: payload}, nil
	}, hooks.Options{})
	ctx := hooks.WithBus(context.Background(), bus)

	_, err := NewService().RunTurn(ctx, TurnRequest{
		SessionID: "session-1",
		Runner:    runner,
		Input:     message.TurnInput{Message: message.Message{Role: message.RoleUser, Content: "original"}},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if runner.input.Message.Content != "rewritten" {
		t.Fatalf("runner input = %q", runner.input.Message.Content)
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
		SessionID:        "session-1",
		Runner:           runner,
		Input:            message.TurnInput{Message: message.Message{Role: message.RoleUser, Content: "ping"}},
		ApprovalRegistry: approval.NewAutoApproveRegistry(),
		SteeringSource:   steeringSource,
		AskHandler:       askHandler,
	})
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
	opts = opts.WithDefaults("fake-run")
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

func TestRunTurnReturnsPersistenceError(t *testing.T) {
	history := &failingHistory{err: errors.New("disk full")}
	_, err := NewService().RunTurn(context.Background(), TurnRequest{SessionID: "session", Runner: &fakeRunner{}, History: history})
	if !errors.Is(err, history.err) || !history.finalized {
		t.Fatalf("run = %v, history = %+v", err, history)
	}
}

func TestAssistantTextUsesCompletedMessagesOnce(t *testing.T) {
	got := AssistantText([]event.Event{
		{Type: event.TypeUserMessage, Content: "question"},
		{Type: event.TypeAssistantText, MessageID: "a", Content: "hel"},
		{Type: event.TypeAssistantText, MessageID: "a", Content: "lo"},
		{Type: event.TypeToolCallCompleted, Content: "tool result"},
		{Type: event.TypeAssistantCompleted, MessageID: "a", Content: "hello"},
		{Type: event.TypeAssistantCompleted, MessageID: "member", MemberCallID: "delegate", Content: "private"},
		{Type: event.TypeAssistantCompleted, MessageID: "b", Content: " world"},
	})
	if got != "hello world" {
		t.Fatalf("text = %q", got)
	}
}
