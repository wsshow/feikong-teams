package hooks

import (
	"context"
	"testing"

	projecthooks "fkteams/hooks"
	einoruntime "fkteams/internal/adapters/runtime/eino"

	"github.com/cloudwego/eino/compose"
)

func TestToolHooksCanRewriteArgumentsAndObserveResult(t *testing.T) {
	bus := projecthooks.NewBus()
	afterCalled := false
	bus.RegisterFunc("rewrite-tool", []projecthooks.HookPoint{projecthooks.HookBeforeToolCall}, func(ctx projecthooks.Context, inv projecthooks.Invocation) (projecthooks.Result, error) {
		payload := inv.Payload.(projecthooks.BeforeToolCallPayload)
		payload.Args = `{"text":"hooked"}`
		return projecthooks.Result{Payload: payload}, nil
	}, projecthooks.Options{})
	bus.RegisterFunc("after-tool", []projecthooks.HookPoint{projecthooks.HookAfterToolCall}, func(ctx projecthooks.Context, inv projecthooks.Invocation) (projecthooks.Result, error) {
		payload := inv.Payload.(projecthooks.AfterToolCallPayload)
		if payload.Args != `{"text":"hooked"}` {
			t.Fatalf("args = %q, want hooked args", payload.Args)
		}
		if payload.Result != "ok" {
			t.Fatalf("result = %q, want ok", payload.Result)
		}
		afterCalled = true
		return projecthooks.Result{}, nil
	}, projecthooks.Options{})

	middleware, err := einoruntime.AdaptToolMiddlewareForRunner(New())
	if err != nil {
		t.Fatalf("adapt middleware: %v", err)
	}
	endpoint := compose.InvokableToolEndpoint(func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
		if input.Arguments != `{"text":"hooked"}` {
			t.Fatalf("endpoint args = %q, want hooked args", input.Arguments)
		}
		return &compose.ToolOutput{Result: "ok"}, nil
	})

	wrapped := middleware.Invokable(endpoint)
	_, err = wrapped(projecthooks.WithBus(context.Background(), bus), &compose.ToolInput{Name: "echo", Arguments: `{"text":"original"}`})
	if err != nil {
		t.Fatalf("invoke tool: %v", err)
	}
	if !afterCalled {
		t.Fatal("after hook was not called")
	}
}

func TestToolHookCanRejectCall(t *testing.T) {
	bus := projecthooks.NewBus()
	bus.RegisterFunc("reject-tool", []projecthooks.HookPoint{projecthooks.HookBeforeToolCall}, func(ctx projecthooks.Context, inv projecthooks.Invocation) (projecthooks.Result, error) {
		return projecthooks.Result{Action: projecthooks.ActionReject, Message: "blocked"}, nil
	}, projecthooks.Options{})

	middleware, err := einoruntime.AdaptToolMiddlewareForRunner(New())
	if err != nil {
		t.Fatalf("adapt middleware: %v", err)
	}
	called := false
	endpoint := compose.InvokableToolEndpoint(func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
		called = true
		return &compose.ToolOutput{Result: "ok"}, nil
	})

	wrapped := middleware.Invokable(endpoint)
	if _, err = wrapped(projecthooks.WithBus(context.Background(), bus), &compose.ToolInput{Name: "echo"}); err == nil {
		t.Fatal("expected reject error")
	}
	if called {
		t.Fatal("endpoint should not be called")
	}
}
