package standalone

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"

	domainevent "fkteams/internal/domain/event"
	domainmessage "fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/hooks"
)

func TestRunTextBuildsBareAgentAndReturnsText(t *testing.T) {
	runtime := &fakeRuntime{deltas: []string{"标题"}}
	service := NewService(Dependencies{
		AgentRuntime:  runtime,
		RunnerRuntime: runtime,
	})

	title, err := service.RunText(context.Background(), Request{
		Name:        "session_title",
		Description: "title generator",
		Instruction: "return a short title",
		Model:       fakeModel{},
		Input:       "帮我重构 runtime",
	})
	if err != nil {
		t.Fatalf("run text: %v", err)
	}
	if title != "标题" {
		t.Fatalf("title = %q, want 标题", title)
	}
	if runtime.agentConfig == nil {
		t.Fatal("agent was not built")
	}
	if runtime.agentConfig.Name != "session_title" || runtime.agentConfig.Instruction != "return a short title" {
		t.Fatalf("agent config = %#v", runtime.agentConfig)
	}
	if len(runtime.agentConfig.Middlewares) != 0 {
		t.Fatalf("bare agent middlewares = %d, want 0", len(runtime.agentConfig.Middlewares))
	}
	if len(runtime.agentConfig.ToolMiddlewares) != 0 {
		t.Fatalf("bare agent tool middlewares = %d, want 0", len(runtime.agentConfig.ToolMiddlewares))
	}
	if runtime.runnerConfig.EnableStreaming {
		t.Fatal("RunText enabled streaming")
	}
	if runtime.runner.input.Message.Content != "帮我重构 runtime" {
		t.Fatalf("runner input = %#v", runtime.runner.input)
	}
}

func TestStreamTextEmitsDeltasAndReturnsFinalText(t *testing.T) {
	runtime := &fakeRuntime{deltas: []string{"会话", "标题"}}
	service := NewService(Dependencies{
		AgentRuntime:  runtime,
		RunnerRuntime: runtime,
	})

	var deltas []string
	var events []domainevent.Event
	title, err := service.StreamText(context.Background(), Request{
		Name:  "session_title",
		Model: fakeModel{},
		Input: "hello",
		EventSink: func(event domainevent.Event) error {
			events = append(events, event)
			return nil
		},
	}, func(delta string) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil {
		t.Fatalf("stream text: %v", err)
	}
	if title != "会话标题" {
		t.Fatalf("title = %q, want 会话标题", title)
	}
	if !runtime.runnerConfig.EnableStreaming {
		t.Fatal("StreamText did not enable streaming")
	}
	if !reflect.DeepEqual(deltas, []string{"会话", "标题"}) {
		t.Fatalf("deltas = %#v", deltas)
	}
	if len(events) != 3 {
		t.Fatalf("events = %d, want 3", len(events))
	}
}

func TestRunTextRejectsMissingRequiredFields(t *testing.T) {
	runtime := &fakeRuntime{}
	service := NewService(Dependencies{AgentRuntime: runtime, RunnerRuntime: runtime})

	if _, err := service.RunText(context.Background(), Request{Name: "x", Input: "hello"}); err == nil {
		t.Fatal("expected missing model error")
	}
	if _, err := service.RunText(context.Background(), Request{Name: "x", Model: fakeModel{}}); err == nil {
		t.Fatal("expected missing input error")
	}
	if _, err := NewService(Dependencies{}).RunText(context.Background(), Request{Name: "x", Model: fakeModel{}, Input: "hello"}); err == nil {
		t.Fatal("expected missing runtime error")
	}
}

type fakeRuntime struct {
	events       []domainevent.Event
	deltas       []string
	agentConfig  *runtimeport.ChatAgentConfig
	runnerConfig runtimeport.RunnerConfig
	runner       *fakeStandaloneRunner
}

func (r *fakeRuntime) NewChatModelAgent(_ context.Context, cfg *runtimeport.ChatAgentConfig) (runtimeport.Agent, error) {
	copied := *cfg
	r.agentConfig = &copied
	return fakeAgent{name: cfg.Name, description: cfg.Description}, nil
}

func (r *fakeRuntime) NewLoopAgent(context.Context, *runtimeport.LoopAgentConfig) (runtimeport.Agent, error) {
	return nil, nil
}

func (r *fakeRuntime) NewDeepAgent(context.Context, *runtimeport.DeepAgentConfig) (runtimeport.Agent, error) {
	return nil, nil
}

func (r *fakeRuntime) NewRunner(_ context.Context, cfg runtimeport.RunnerConfig) (runtimeport.Runner, error) {
	r.runnerConfig = cfg
	r.runner = &fakeStandaloneRunner{deltas: append([]string(nil), r.deltas...), events: r.events}
	return r.runner, nil
}

type fakeAgent struct {
	name        string
	description string
}

func (a fakeAgent) Name() string {
	return a.name
}

func (a fakeAgent) Description() string {
	return a.description
}

type fakeStandaloneRunner struct {
	events []domainevent.Event
	deltas []string
	input  domainmessage.TurnInput
	opts   runtimeport.RunOptions
}

func (r *fakeStandaloneRunner) Run(_ context.Context, input domainmessage.TurnInput, opts runtimeport.RunOptions) (*runtimeport.RunResult, error) {
	r.input = input
	r.opts = opts
	if len(r.events) > 0 {
		for _, e := range r.events {
			if err := opts.Sink(e); err != nil {
				return nil, err
			}
		}
		return &runtimeport.RunResult{LastEvent: r.events[len(r.events)-1]}, nil
	}
	for _, delta := range r.deltas {
		if err := opts.Sink(domainevent.Event{Type: domainevent.TypeAssistantText, Content: delta}); err != nil {
			return nil, err
		}
	}
	completed := domainevent.Event{Type: domainevent.TypeAssistantCompleted, Content: joinDeltas(r.deltas)}
	if err := opts.Sink(completed); err != nil {
		return nil, err
	}
	return &runtimeport.RunResult{LastEvent: completed}, nil
}

type fakeModel struct{}

func (fakeModel) Generate(context.Context, []domainmessage.Message) (domainmessage.Message, error) {
	return domainmessage.Message{}, nil
}

func (fakeModel) Stream(context.Context, []domainmessage.Message) (runtimeport.MessageStream, error) {
	return emptyStream{}, nil
}

func (fakeModel) WithTools([]runtimeport.ToolInfo) (runtimeport.ChatModel, error) {
	return fakeModel{}, nil
}

type emptyStream struct{}

func (emptyStream) Recv() (domainmessage.Message, error) {
	return domainmessage.Message{}, io.EOF
}

func (emptyStream) Close() {}

func joinDeltas(deltas []string) string {
	var result string
	for _, delta := range deltas {
		result += delta
	}
	return result
}

func TestStandaloneUsesSharedTurnLifecycle(t *testing.T) {
	runtime := &fakeRuntime{}
	service := NewService(Dependencies{AgentRuntime: runtime, RunnerRuntime: runtime})
	bus := hooks.NewBus()
	afterRuns := 0
	bus.RegisterFunc("rewrite", []hooks.HookPoint{hooks.HookBeforeRun}, func(_ hooks.Context, inv hooks.Invocation) (hooks.Result, error) {
		payload := inv.Payload.(hooks.BeforeRunPayload)
		payload.Input.Message.Content = "rewritten"
		return hooks.Result{Payload: payload}, nil
	}, hooks.Options{})
	bus.RegisterFunc("observe", []hooks.HookPoint{hooks.HookAfterRun}, func(hooks.Context, hooks.Invocation) (hooks.Result, error) {
		afterRuns++
		return hooks.Result{}, nil
	}, hooks.Options{})
	ctx := hooks.WithBus(context.Background(), bus)
	req := Request{Name: "helper", Model: fakeModel{}, Input: "original"}
	var previousID string
	for i := 0; i < 2; i++ {
		if _, err := service.RunText(ctx, req); err != nil {
			t.Fatal(err)
		}
		runner := runtime.runner
		if runner.input.Message.Content != "rewritten" {
			t.Fatalf("before-run hook was skipped: %+v", runner.input)
		}
		if runner.opts.RunID == "" || runner.opts.RunID == previousID {
			t.Fatalf("run ID was reused: %q", runner.opts.RunID)
		}
		previousID = runner.opts.RunID
		if runner.opts.InterruptHandler == nil {
			t.Fatal("missing shared interrupt default")
		}
		decisions, err := runner.opts.InterruptHandler(ctx, []runtimeport.Interrupt{{ID: "approval", IsRootCause: true}})
		if err != nil || decisions["approval"] != 0 {
			t.Fatalf("unexpected default decision: %v, %v", decisions, err)
		}
	}
	if afterRuns != 2 {
		t.Fatalf("after-run calls = %d", afterRuns)
	}
	req.RunID = "explicit-run"
	if _, err := service.RunText(ctx, req); err != nil || runtime.runner.opts.RunID != req.RunID {
		t.Fatalf("explicit run ID was not preserved: %v", err)
	}
}

func TestStandaloneCollectsCanonicalMainMessages(t *testing.T) {
	runtime := &fakeRuntime{events: []domainevent.Event{
		{Type: domainevent.TypeAssistantText, MessageID: "first", Content: "draft"},
		{Type: domainevent.TypeAssistantText, MessageID: "member", MemberCallID: "call", Content: "private"},
		{Type: domainevent.TypeAssistantCompleted, MessageID: "first", Content: "revised"},
		{Type: domainevent.TypeAssistantText, MessageID: "first", Content: "late"},
		{Type: domainevent.TypeAssistantCompleted, MessageID: "second", Content: " answer"},
	}}
	service := NewService(Dependencies{AgentRuntime: runtime, RunnerRuntime: runtime})
	req := Request{Name: "helper", Model: fakeModel{}, Input: "hello"}
	var deltas []string
	got, err := service.StreamText(context.Background(), req, func(delta string) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil || got != "revised answer" || !reflect.DeepEqual(deltas, []string{"draft"}) {
		t.Fatalf("stream result = %q, deltas = %v, err = %v", got, deltas, err)
	}
	got, err = service.RunText(context.Background(), req)
	if err != nil || got != "revised answer" {
		t.Fatalf("non-stream result = %q, err = %v", got, err)
	}
}

func TestStandalonePropagatesSinkErrors(t *testing.T) {
	want := errors.New("sink failed")
	runtime := &fakeRuntime{deltas: []string{"text"}}
	service := NewService(Dependencies{AgentRuntime: runtime, RunnerRuntime: runtime})
	req := Request{Name: "helper", Model: fakeModel{}, Input: "hello"}
	_, err := service.StreamText(context.Background(), req, func(string) error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("delta error = %v", err)
	}
	req.EventSink = func(domainevent.Event) error { return want }
	_, err = service.RunText(context.Background(), req)
	if !errors.Is(err, want) {
		t.Fatalf("event error = %v", err)
	}
}
