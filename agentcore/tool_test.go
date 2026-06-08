package agentcore

import (
	"context"
	"testing"
)

type echoToolRequest struct {
	Text string `json:"text"`
}

type echoToolResponse struct {
	Text string `json:"text"`
}

func TestFunctionToolInvokeParsesArgumentsAndSerializesResult(t *testing.T) {
	tool, err := NewTool(ToolInfo{Name: "echo"}, func(_ context.Context, req *echoToolRequest) (*echoToolResponse, error) {
		return &echoToolResponse{Text: req.Text}, nil
	})
	if err != nil {
		t.Fatalf("new tool: %v", err)
	}

	result, err := tool.Invoke(context.Background(), ToolInvocation{Arguments: `{"text":"hello"}`})
	if err != nil {
		t.Fatalf("invoke tool: %v", err)
	}
	if result == nil || result.Content != `{"text":"hello"}` {
		t.Fatalf("result = %#v, want JSON echo", result)
	}
}

func TestFunctionToolInvokeSupportsStringResult(t *testing.T) {
	tool, err := NewTool(ToolInfo{Name: "text"}, func(_ context.Context, req *echoToolRequest) (string, error) {
		return req.Text, nil
	})
	if err != nil {
		t.Fatalf("new tool: %v", err)
	}

	result, err := tool.Invoke(context.Background(), ToolInvocation{Arguments: `{"text":"hello"}`})
	if err != nil {
		t.Fatalf("invoke tool: %v", err)
	}
	if result == nil || result.Content != "hello" {
		t.Fatalf("result = %#v, want hello", result)
	}
}
