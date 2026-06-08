package eino

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"fkteams/agentcore"
	"fkteams/internal/testmodel"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

func TestAdaptNativeChatModelForRunner(t *testing.T) {
	ctx := context.Background()
	cm := testmodel.New(testmodel.AssistantMessage("ok"))

	runnerModel, err := AdaptChatModelForRunner(cm)
	if err != nil {
		t.Fatalf("adapt model: %v", err)
	}
	bound, err := runnerModel.WithTools([]*schema.ToolInfo{{Name: "test_tool", Desc: "test tool"}})
	if err != nil {
		t.Fatalf("bind tools: %v", err)
	}

	resp, err := bound.Generate(ctx, []*schema.Message{schema.UserMessage("hello")})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("unexpected response: %q", resp.Content)
	}

	calls := cm.GenerateCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 generate call, got %d", len(calls))
	}
	if calls[0].Input[0].Role != agentcore.RoleUser || calls[0].Input[0].Content != "hello" {
		t.Fatalf("unexpected core input: %#v", calls[0].Input)
	}
	if len(calls[0].Tools) != 1 || calls[0].Tools[0].Name != "test_tool" {
		t.Fatalf("expected core tool binding, got %#v", calls[0].Tools)
	}
}

func TestAdaptToolUsesCoreInvoke(t *testing.T) {
	coreTool := &invokeOnlyTool{
		info: agentcore.ToolInfo{Name: "invoke_tool", Desc: "invoke tool"},
	}
	runnerTools, err := AdaptToolsForRunner(context.Background(), []agentcore.Tool{coreTool})
	if err != nil {
		t.Fatalf("adapt tools: %v", err)
	}
	if len(runnerTools) != 1 {
		t.Fatalf("tool count = %d, want 1", len(runnerTools))
	}
	invokable, ok := runnerTools[0].(interface {
		InvokableRun(context.Context, string, ...tool.Option) (string, error)
	})
	if !ok {
		t.Fatalf("tool is not invokable: %T", runnerTools[0])
	}
	result, err := invokable.InvokableRun(context.Background(), `{"text":"hello"}`)
	if err != nil {
		t.Fatalf("run tool: %v", err)
	}
	if result != "invoked:hello" {
		t.Fatalf("result = %q, want invoked:hello", result)
	}
	if !coreTool.invoked {
		t.Fatal("core Invoke was not called")
	}
}

type invokeOnlyTool struct {
	info    agentcore.ToolInfo
	invoked bool
}

func (t *invokeOnlyTool) Info(context.Context) (*agentcore.ToolInfo, error) {
	return &t.info, nil
}

func (t *invokeOnlyTool) InputType() reflect.Type {
	return reflect.TypeOf((*invokeOnlyRequest)(nil))
}

func (t *invokeOnlyTool) Invoke(_ context.Context, invocation agentcore.ToolInvocation) (*agentcore.ToolResult, error) {
	t.invoked = true
	var req invokeOnlyRequest
	if err := json.Unmarshal([]byte(invocation.Arguments), &req); err != nil {
		return nil, err
	}
	return &agentcore.ToolResult{Content: "invoked:" + req.Text}, nil
}

type invokeOnlyRequest struct {
	Text string `json:"text"`
}
