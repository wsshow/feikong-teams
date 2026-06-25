package warperror

import (
	"context"
	"errors"
	"testing"

	einoruntime "fkteams/internal/adapters/runtime/eino"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func TestDefaultErrorHandlerFormatsToolError(t *testing.T) {
	got := defaultErrorHandler(context.Background(), &compose.ToolInput{Name: "search"}, errors.New("timeout"))
	want := "Failed to call tool 'search', error message: 'timeout'"
	if got != want {
		t.Fatalf("defaultErrorHandler() = %q, want %q", got, want)
	}
}

func TestInvokableWrapsErrorAsToolOutput(t *testing.T) {
	middleware := newInvokable(func(ctx context.Context, in *compose.ToolInput, err error) string {
		return in.Name + ":" + in.Arguments + ":" + err.Error()
	})
	endpoint := compose.InvokableToolEndpoint(func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
		return nil, errors.New("boom")
	})

	output, err := middleware(endpoint)(context.Background(), &compose.ToolInput{Name: "echo", Arguments: `{"text":"hi"}`})
	if err != nil {
		t.Fatalf("wrapped endpoint error = %v", err)
	}
	want := `echo:{"text":"hi"}:boom`
	if output == nil || output.Result != want {
		t.Fatalf("output = %#v, want result %q", output, want)
	}
}

func TestInvokablePassesThroughSuccess(t *testing.T) {
	middleware := newInvokable(defaultErrorHandler)
	endpoint := compose.InvokableToolEndpoint(func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
		return &compose.ToolOutput{Result: "ok"}, nil
	})

	output, err := middleware(endpoint)(context.Background(), &compose.ToolInput{Name: "echo"})
	if err != nil {
		t.Fatalf("wrapped endpoint error = %v", err)
	}
	if output == nil || output.Result != "ok" {
		t.Fatalf("output = %#v, want ok", output)
	}
}

func TestInvokablePassesThroughInterruptRerun(t *testing.T) {
	interruptErr := compose.NewInterruptAndRerunErr("wait")
	middleware := newInvokable(defaultErrorHandler)
	endpoint := compose.InvokableToolEndpoint(func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
		return nil, interruptErr
	})

	output, err := middleware(endpoint)(context.Background(), &compose.ToolInput{Name: "approval"})
	if output != nil {
		t.Fatalf("output = %#v, want nil", output)
	}
	if _, ok := compose.IsInterruptRerunError(err); !ok {
		t.Fatalf("error = %v, want interrupt rerun", err)
	}
}

func TestStreamableWrapsErrorAsSingleChunk(t *testing.T) {
	middleware := newStreamable(func(ctx context.Context, in *compose.ToolInput, err error) string {
		return "handled:" + in.Name + ":" + err.Error()
	})
	endpoint := compose.StreamableToolEndpoint(func(ctx context.Context, input *compose.ToolInput) (*compose.StreamToolOutput, error) {
		return nil, errors.New("failed")
	})

	output, err := middleware(endpoint)(context.Background(), &compose.ToolInput{Name: "fetch"})
	if err != nil {
		t.Fatalf("wrapped endpoint error = %v", err)
	}
	if output == nil || output.Result == nil {
		t.Fatalf("output = %#v, want stream result", output)
	}
	defer output.Result.Close()

	chunk, err := output.Result.Recv()
	if err != nil {
		t.Fatalf("recv stream chunk: %v", err)
	}
	if chunk != "handled:fetch:failed" {
		t.Fatalf("chunk = %q, want handled result", chunk)
	}
}

func TestNewUsesConfiguredHandler(t *testing.T) {
	middleware, err := einoruntime.AdaptToolMiddlewareForRunner(New(&Config{
		Handler: func(ctx context.Context, in *compose.ToolInput, err error) string {
			return "custom:" + in.CallID
		},
	}))
	if err != nil {
		t.Fatalf("adapt middleware: %v", err)
	}
	endpoint := compose.InvokableToolEndpoint(func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
		return nil, errors.New("boom")
	})

	output, err := middleware.Invokable(endpoint)(context.Background(), &compose.ToolInput{Name: "echo", CallID: "call-1"})
	if err != nil {
		t.Fatalf("wrapped endpoint error = %v", err)
	}
	if output == nil || output.Result != "custom:call-1" {
		t.Fatalf("output = %#v, want custom handler result", output)
	}
}

func TestNewHandlerCreatesAgentMiddleware(t *testing.T) {
	middleware := NewHandler(nil)
	if middleware.Name() != "wrap_tool_error" {
		t.Fatalf("middleware name = %q, want wrap_tool_error", middleware.Name())
	}
	if _, err := einoruntime.AdaptAgentMiddlewareForRunner(middleware); err != nil {
		t.Fatalf("adapt agent middleware: %v", err)
	}
}

func TestAgentHandlerWrapsEnhancedInvokableError(t *testing.T) {
	handler := &agentHandler{
		handler: func(ctx context.Context, in *compose.ToolInput, err error) string {
			return in.Name + ":" + in.Arguments + ":" + in.CallID
		},
	}
	endpoint := adk.EnhancedInvokableToolCallEndpoint(func(ctx context.Context, toolArgument *schema.ToolArgument, opts ...tool.Option) (*schema.ToolResult, error) {
		return nil, errors.New("boom")
	})

	wrapped, err := handler.WrapEnhancedInvokableToolCall(context.Background(), endpoint, &adk.ToolContext{Name: "search", CallID: "call-2"})
	if err != nil {
		t.Fatalf("wrap enhanced endpoint: %v", err)
	}
	result, err := wrapped(context.Background(), &schema.ToolArgument{Text: `{"q":"x"}`})
	if err != nil {
		t.Fatalf("wrapped enhanced endpoint error = %v", err)
	}
	assertToolResultText(t, result, `search:{"q":"x"}:call-2`)
}

func TestAgentHandlerWrapsEnhancedStreamableError(t *testing.T) {
	handler := &agentHandler{
		handler: func(ctx context.Context, in *compose.ToolInput, err error) string {
			return in.Name + ":" + in.Arguments
		},
	}
	endpoint := adk.EnhancedStreamableToolCallEndpoint(func(ctx context.Context, toolArgument *schema.ToolArgument, opts ...tool.Option) (*schema.StreamReader[*schema.ToolResult], error) {
		return nil, errors.New("boom")
	})

	wrapped, err := handler.WrapEnhancedStreamableToolCall(context.Background(), endpoint, &adk.ToolContext{Name: "doc", CallID: "call-3"})
	if err != nil {
		t.Fatalf("wrap enhanced stream endpoint: %v", err)
	}
	stream, err := wrapped(context.Background(), nil)
	if err != nil {
		t.Fatalf("wrapped enhanced stream endpoint error = %v", err)
	}
	defer stream.Close()

	result, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv enhanced stream: %v", err)
	}
	assertToolResultText(t, result, "doc:")
}

func assertToolResultText(t *testing.T, result *schema.ToolResult, want string) {
	t.Helper()
	if result == nil || len(result.Parts) != 1 {
		t.Fatalf("tool result = %#v, want one text part", result)
	}
	part := result.Parts[0]
	if part.Type != schema.ToolPartTypeText || part.Text != want {
		t.Fatalf("tool result part = %#v, want text %q", part, want)
	}
}
