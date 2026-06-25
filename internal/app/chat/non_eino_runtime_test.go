package chat

import (
	"context"
	"encoding/json"
	"testing"

	"fkteams/internal/domain/event"
	"fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/hooks"
)

func TestServiceRunsThroughNonEinoRuntime(t *testing.T) {
	engine := &fakeRuntimeEngine{}
	tool, err := runtimeport.InferTool("fake_echo", "echo input", fakeEchoTool)
	if err != nil {
		t.Fatalf("create tool: %v", err)
	}
	agent, err := engine.NewChatModelAgent(context.Background(), &runtimeport.ChatAgentConfig{
		Name:        "fake",
		Description: "fake runtime agent",
		Tools:       []runtimeport.Tool{tool},
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	runner, err := engine.NewRunner(context.Background(), runtimeport.RunnerConfig{Agent: agent})
	if err != nil {
		t.Fatalf("create runner: %v", err)
	}

	bus := hooks.NewBus()
	afterRunCalled := false
	bus.RegisterFunc("rewrite-input", []hooks.HookPoint{hooks.HookBeforeRun}, func(ctx hooks.Context, inv hooks.Invocation) (hooks.Result, error) {
		payload := inv.Payload.(hooks.BeforeRunPayload)
		payload.Input.Message.Content = "hooked"
		return hooks.Result{Payload: payload}, nil
	}, hooks.Options{})
	bus.RegisterFunc("after-run", []hooks.HookPoint{hooks.HookAfterRun}, func(ctx hooks.Context, inv hooks.Invocation) (hooks.Result, error) {
		payload := inv.Payload.(hooks.AfterRunPayload)
		if payload.Result == nil || payload.Result.LastEvent.Type != event.TypeMessageEnd {
			t.Fatalf("after-run result = %#v, want message_end", payload.Result)
		}
		afterRunCalled = true
		return hooks.Result{}, nil
	}, hooks.Options{})

	var recorded []event.Event
	var published []event.Event
	_, err = NewService().RunTurn(context.Background(), TurnRequest{
		SessionID: "session-1",
		Runner:    runner,
		Input:     message.TurnInput{Message: message.Message{Role: message.RoleUser, Content: "original"}},
	},
		WithHookBus(bus),
		WithEventRecorderFunc(func(event event.Event) {
			recorded = append(recorded, event)
		}),
		OnEvent(func(event event.Event) error {
			published = append(published, event)
			return nil
		}),
		OnInterrupt(func(ctx context.Context, interrupts []runtimeport.Interrupt) (map[string]any, error) {
			if len(interrupts) != 1 || interrupts[0].ID != "approval-1" {
				t.Fatalf("interrupts = %#v, want approval-1", interrupts)
			}
			return map[string]any{"approval-1": "approved"}, nil
		}),
	)
	if err != nil {
		t.Fatalf("run turn: %v", err)
	}

	fakeRunner := runner.(*fakeRuntimeRunner)
	if fakeRunner.input.Message.Content != "hooked" {
		t.Fatalf("runner input = %q, want hooked", fakeRunner.input.Message.Content)
	}
	if fakeRunner.interruptDecision != "approved" {
		t.Fatalf("interrupt decision = %q, want approved", fakeRunner.interruptDecision)
	}
	if fakeRunner.toolResult != `{"text":"tool:hooked"}` {
		t.Fatalf("tool result = %q, want tool response", fakeRunner.toolResult)
	}
	if !afterRunCalled {
		t.Fatal("after-run hook was not called")
	}
	if len(recorded) != 3 || len(published) != 3 {
		t.Fatalf("recorded/published event counts = %d/%d, want 3/3", len(recorded), len(published))
	}
	for i := range recorded {
		if recorded[i].Type != published[i].Type {
			t.Fatalf("event %d recorded=%s published=%s", i, recorded[i].Type, published[i].Type)
		}
	}
}

type fakeRuntimeEngine struct{}

func (fakeRuntimeEngine) NewChatModelAgent(ctx context.Context, cfg *runtimeport.ChatAgentConfig) (runtimeport.Agent, error) {
	return &fakeRuntimeAgent{name: cfg.Name, description: cfg.Description, tools: append([]runtimeport.Tool(nil), cfg.Tools...)}, nil
}

func (fakeRuntimeEngine) NewLoopAgent(context.Context, *runtimeport.LoopAgentConfig) (runtimeport.Agent, error) {
	return &fakeRuntimeAgent{name: "loop"}, nil
}

func (fakeRuntimeEngine) NewDeepAgent(context.Context, *runtimeport.DeepAgentConfig) (runtimeport.Agent, error) {
	return &fakeRuntimeAgent{name: "deep"}, nil
}

func (fakeRuntimeEngine) NewRunner(ctx context.Context, cfg runtimeport.RunnerConfig) (runtimeport.Runner, error) {
	agent := cfg.Agent.(*fakeRuntimeAgent)
	return &fakeRuntimeRunner{tools: append([]runtimeport.Tool(nil), agent.tools...)}, nil
}

func (fakeRuntimeEngine) NewAgentTools(context.Context, []runtimeport.Agent, runtimeport.AgentToolConfig) ([]runtimeport.Tool, error) {
	return nil, nil
}

type fakeRuntimeAgent struct {
	name        string
	description string
	tools       []runtimeport.Tool
}

func (a *fakeRuntimeAgent) Name() string {
	return a.name
}

func (a *fakeRuntimeAgent) Description() string {
	return a.description
}

type fakeRuntimeRunner struct {
	tools             []runtimeport.Tool
	input             message.TurnInput
	interruptDecision string
	toolResult        string
}

func (r *fakeRuntimeRunner) Run(ctx context.Context, input message.TurnInput, opts runtimeport.RunOptions) (*runtimeport.RunResult, error) {
	r.input = input
	opts = opts.WithDefaults(opts.CheckpointID)
	if opts.InterruptHandler != nil {
		decisions, err := opts.InterruptHandler(ctx, []runtimeport.Interrupt{{ID: "approval-1", Info: "approve?"}})
		if err != nil {
			return nil, err
		}
		if value, ok := decisions["approval-1"].(string); ok {
			r.interruptDecision = value
		}
	}

	if len(r.tools) > 0 {
		args, _ := json.Marshal(fakeEchoRequest{Text: input.Message.Content})
		result, err := r.tools[0].Invoke(ctx, runtimeport.ToolInvocation{
			Name:      "fake_echo",
			CallID:    "tool-1",
			Arguments: string(args),
		})
		if err != nil {
			return nil, err
		}
		r.toolResult = result.Content
	}

	events := []event.Event{
		{Type: event.TypeToolStart, ToolCallID: "tool-1", ToolName: "fake_echo"},
		{Type: event.TypeToolEnd, ToolCallID: "tool-1", ToolName: "fake_echo", ToolResult: r.toolResult},
		{Type: event.TypeMessageEnd, Message: &message.Message{Role: message.RoleAssistant, Content: "done"}},
	}
	for _, event := range events {
		if err := opts.Sink(event); err != nil {
			return nil, err
		}
	}
	return &runtimeport.RunResult{LastEvent: events[len(events)-1]}, nil
}

type fakeEchoRequest struct {
	Text string `json:"text"`
}

type fakeEchoResponse struct {
	Text string `json:"text"`
}

func fakeEchoTool(_ context.Context, req *fakeEchoRequest) (*fakeEchoResponse, error) {
	return &fakeEchoResponse{Text: "tool:" + req.Text}, nil
}
