package turn

import (
	"context"
	"fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/events"
	"fkteams/internal/runtime/hooks"
	"fkteams/tools/approval"
	"testing"
)

type historySinkStub struct {
	count int
}

func (h *historySinkStub) GetMessageCount() int {
	return h.count
}

func (h *historySinkStub) RecordUserMessage(message.Message) {}

func (h *historySinkStub) SetSummary(string, int) {}

type runnerStub struct {
	input message.TurnInput
	opts  runtimeport.RunOptions
}

func (r *runnerStub) Run(_ context.Context, input message.TurnInput, opts runtimeport.RunOptions) (*runtimeport.RunResult, error) {
	r.input = input
	r.opts = opts
	return &runtimeport.RunResult{}, nil
}

func TestSessionBuilderConfiguresRunConfig(t *testing.T) {
	messages := []message.Message{{Role: message.RoleUser, Content: "hello"}}
	history := &historySinkStub{}
	approvalReg := approval.NewDefaultRegistry()
	eventHandler := func(events.Event) error { return nil }
	startHandler := func(context.Context) {}
	interruptHandler := FixedDecisionHandler(approval.Reject)
	finishHandler := func(context.Context, *runtimeport.RunResult, error) {}

	session := NewSession(&runnerStub{}, "session-1").
		WithMessages(messages).
		OnEvent(eventHandler).
		WithHistory(history).
		OnStart(startHandler).
		OnInterrupt(interruptHandler).
		NonInteractive().
		WithContext(approval.RegistryContext(approvalReg)).
		OnFinish(finishHandler)

	if len(session.cfg.Input.Context) != 1 || session.cfg.Input.Context[0].Content != "hello" {
		t.Fatal("messages were not configured")
	}
	if session.cfg.EventCallback == nil {
		t.Fatal("event handler was not configured")
	}
	if session.cfg.Recorder != history {
		t.Fatal("history sink was not configured")
	}
	if session.cfg.OnStart == nil {
		t.Fatal("start handler was not configured")
	}
	if session.cfg.OnInterrupt == nil {
		t.Fatal("interrupt handler was not configured")
	}
	if !session.cfg.NonInteractive {
		t.Fatal("non-interactive flag was not configured")
	}
	if len(session.cfg.ContextHooks) != 1 {
		t.Fatal("context hook was not configured")
	}
	if session.cfg.OnFinish == nil {
		t.Fatal("finish handler was not configured")
	}
}

func TestSessionBuilderConfiguresRunID(t *testing.T) {
	session := NewSession(&runnerStub{}, "session-1").WithRunID("run-1")
	if session.cfg.RunID != "run-1" {
		t.Fatalf("run id = %q, want run-1", session.cfg.RunID)
	}
}

func TestSessionBuilderConfiguresHookBus(t *testing.T) {
	bus := hooks.NewBus()
	session := NewSession(&runnerStub{}, "session-1").WithHookBus(bus)
	if session.cfg.HookBus != bus {
		t.Fatal("hook bus was not configured")
	}
}

func TestSessionBuilderConfiguresTurnInput(t *testing.T) {
	input := TurnInput{
		Context: []message.Message{{Role: message.RoleSystem, Content: "context"}},
		Message: message.Message{Role: message.RoleUser, Content: "hello"},
	}

	session := NewSession(&runnerStub{}, "session-1").WithInput(input)

	if len(session.cfg.Input.Context) != 1 || session.cfg.Input.Context[0].Content != "context" {
		t.Fatal("input context was not configured")
	}
	if session.cfg.Input.Message.Content != "hello" {
		t.Fatalf("input message = %q, want hello", session.cfg.Input.Message.Content)
	}
}

func TestSessionRunUsesConfiguredRunID(t *testing.T) {
	runner := &runnerStub{}
	_, err := NewSession(runner, "session-1").
		WithRunID("run-1").
		WithText("hello").
		Run(context.Background())
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if runner.opts.RunID != "run-1" {
		t.Fatalf("run id = %q, want run-1", runner.opts.RunID)
	}
	if runner.opts.CheckpointID != "session-1" {
		t.Fatalf("checkpoint id = %q, want session-1", runner.opts.CheckpointID)
	}
}

func TestSessionBuilderConfiguresPromptMessage(t *testing.T) {
	session := NewSession(&runnerStub{}, "session-1").
		WithMessages([]message.Message{{Role: message.RoleSystem, Content: "context"}}).
		WithText("hello")

	if session.cfg.Input.Message.Role != message.RoleUser {
		t.Fatalf("input role = %q, want user", session.cfg.Input.Message.Role)
	}
	if session.cfg.Input.Message.Content != "hello" {
		t.Fatalf("input message = %q, want hello", session.cfg.Input.Message.Content)
	}

	msg := message.Message{Role: message.RoleUser, Content: "override"}
	session.WithMessage(msg)
	if session.cfg.Input.Message.Content != "override" {
		t.Fatalf("input message = %q, want override", session.cfg.Input.Message.Content)
	}
}
