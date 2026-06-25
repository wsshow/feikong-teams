package inject_test

import (
	"context"
	"strings"
	"testing"

	"fkteams/agentcore"
	"fkteams/hooks"
	einoruntime "fkteams/internal/adapters/runtime/eino"
	"fkteams/internal/adapters/runtime/eino/middlewares/inject"
	"fkteams/internal/testmodel"

	"github.com/cloudwego/eino/schema"
)

func TestGenerateInjectsDynamicContext(t *testing.T) {
	ctx := context.Background()
	cm := testmodel.New(testmodel.AssistantMessage("ok"))
	runnerModel, err := einoruntime.AdaptChatModelForRunner(cm)
	if err != nil {
		t.Fatalf("adapt model: %v", err)
	}
	wrapped := inject.New(runnerModel)

	resp, err := wrapped.Generate(ctx, []*schema.Message{schema.UserMessage("hello")})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("unexpected response: %q", resp.Content)
	}

	calls := cm.GenerateCalls()
	if len(calls) != 1 {
		t.Fatalf("expected one generate call, got %d", len(calls))
	}
	assertInjectedContext(t, calls[0].Input)
}

func TestStreamInjectsDynamicContext(t *testing.T) {
	ctx := context.Background()
	cm := testmodel.New()
	cm.EnqueueStream(testmodel.AssistantMessage("chunk"))
	runnerModel, err := einoruntime.AdaptChatModelForRunner(cm)
	if err != nil {
		t.Fatalf("adapt model: %v", err)
	}
	wrapped := inject.New(runnerModel)

	stream, err := wrapped.Stream(ctx, []*schema.Message{schema.UserMessage("hello")})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	defer stream.Close()

	chunk, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv: %v", err)
	}
	if chunk.Content != "chunk" {
		t.Fatalf("unexpected chunk: %q", chunk.Content)
	}

	calls := cm.StreamCalls()
	if len(calls) != 1 {
		t.Fatalf("expected one stream call, got %d", len(calls))
	}
	assertInjectedContext(t, calls[0].Input)
}

func TestGenerateInvokesModelHooks(t *testing.T) {
	bus := hooks.NewBus()
	afterCalled := false
	bus.RegisterFunc("rewrite-model-input", []hooks.HookPoint{hooks.HookBeforeModelRequest}, func(ctx hooks.Context, inv hooks.Invocation) (hooks.Result, error) {
		payload := inv.Payload.(hooks.BeforeModelRequestPayload)
		payload.Messages = append(payload.Messages, agentcore.Message{Role: agentcore.RoleSystem, Content: "hooked"})
		return hooks.Result{Payload: payload}, nil
	}, hooks.Options{})
	bus.RegisterFunc("after-model", []hooks.HookPoint{hooks.HookAfterModelResponse}, func(ctx hooks.Context, inv hooks.Invocation) (hooks.Result, error) {
		payload := inv.Payload.(hooks.AfterModelResponsePayload)
		if payload.Message.Content != "ok" {
			t.Fatalf("response = %q, want ok", payload.Message.Content)
		}
		afterCalled = true
		return hooks.Result{}, nil
	}, hooks.Options{})

	ctx := hooks.WithBus(context.Background(), bus)
	cm := testmodel.New(testmodel.AssistantMessage("ok"))
	runnerModel, err := einoruntime.AdaptChatModelForRunner(cm)
	if err != nil {
		t.Fatalf("adapt model: %v", err)
	}
	wrapped := inject.New(runnerModel)

	resp, err := wrapped.Generate(ctx, []*schema.Message{schema.UserMessage("hello")})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("unexpected response: %q", resp.Content)
	}
	calls := cm.GenerateCalls()
	if len(calls) != 1 {
		t.Fatalf("expected one generate call, got %d", len(calls))
	}
	found := false
	for _, msg := range calls[0].Input {
		if msg.Content == "hooked" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("hooked model message not found: %#v", calls[0].Input)
	}
	if !afterCalled {
		t.Fatal("after hook was not called")
	}
}

func assertInjectedContext(t *testing.T, input []agentcore.Message) {
	t.Helper()

	if len(input) != 2 {
		t.Fatalf("expected original message plus injected context, got %#v", input)
	}
	if input[0].Content != "hello" {
		t.Fatalf("expected original message to stay first, got %#v", input[0])
	}
	injected := input[1]
	if injected.Role != agentcore.RoleUser {
		t.Fatalf("expected injected context to be user message, got %s", injected.Role)
	}
	for _, want := range []string{"<system-reminder>", "当前时间", "工作目录"} {
		if !strings.Contains(injected.Content, want) {
			t.Fatalf("expected injected context to contain %q, got %q", want, injected.Content)
		}
	}
}
