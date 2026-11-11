package runtime

import (
	"context"
	"errors"
	"reflect"
	"strings"
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

func TestFunctionToolInfoAndInputType(t *testing.T) {
	tool, err := InferTool("echo", "echo desc", func(context.Context, *echoToolRequest) (string, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("infer tool: %v", err)
	}
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("tool info: %v", err)
	}
	if info.Name != "echo" || info.Desc != "echo desc" || info.Extra != nil {
		t.Fatalf("unexpected tool info: %#v", info)
	}
	typed, ok := tool.(ToolInputTypeProvider)
	if !ok {
		t.Fatal("function tool should expose input type")
	}
	if typed.InputType() != reflect.TypeOf((*echoToolRequest)(nil)) {
		t.Fatalf("input type = %v", typed.InputType())
	}
}

func TestFunctionToolInvokeErrorCases(t *testing.T) {
	handlerErr := errors.New("handler failed")
	tool, err := NewTool(ToolInfo{Name: "maybe"}, func(_ context.Context, req *echoToolRequest) (*echoToolResponse, error) {
		if req.Text == "fail" {
			return nil, handlerErr
		}
		return nil, nil
	})
	if err != nil {
		t.Fatalf("new tool: %v", err)
	}

	result, err := tool.Invoke(context.Background(), ToolInvocation{})
	if err != nil {
		t.Fatalf("invoke empty args: %v", err)
	}
	if result == nil || result.Content != "" {
		t.Fatalf("nil output result = %#v", result)
	}

	if _, err := tool.Invoke(context.Background(), ToolInvocation{Arguments: `{bad json`}); err == nil {
		t.Fatal("expected invalid json error")
	}
	if _, err := tool.Invoke(context.Background(), ToolInvocation{Arguments: `{"text":"fail"}`}); !errors.Is(err, handlerErr) {
		t.Fatalf("handler error = %v", err)
	}
}

func TestNewToolRejectsInvalidHandlers(t *testing.T) {
	tests := []struct {
		name    string
		handler any
		want    string
	}{
		{name: "nil", handler: nil, want: "handler is nil"},
		{name: "not func", handler: 1, want: "handler must be func"},
		{name: "wrong inputs", handler: func(context.Context) (string, error) { return "", nil }, want: "handler must be func"},
		{name: "wrong context", handler: func(string, *echoToolRequest) (string, error) { return "", nil }, want: "first argument"},
		{name: "non pointer input", handler: func(context.Context, echoToolRequest) (string, error) { return "", nil }, want: "second argument"},
		{name: "non struct pointer input", handler: func(context.Context, *string) (string, error) { return "", nil }, want: "second argument"},
		{name: "non error output", handler: func(context.Context, *echoToolRequest) (string, string) { return "", "" }, want: "second return value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewTool(ToolInfo{Name: "bad"}, tt.handler)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want containing %q", err.Error(), tt.want)
			}
		})
	}
}

type declaredInput struct {
	Value string `json:"value"`
}

func TestToolDefinitionsPreserveSchemaPolicyAndOrder(t *testing.T) {
	handler := func(_ context.Context, input *declaredInput) (string, error) { return input.Value, nil }
	definitions := []ToolDefinition{
		{Name: "read", Description: "read value", Handler: handler, Policy: ToolPolicyMetadata{ReadOnly: true}},
		{Name: "write", Description: "write value", Handler: handler, Policy: ToolPolicyMetadata{ApprovalStore: "file"}},
	}
	tools, err := NewTools(definitions)
	if err != nil {
		t.Fatal(err)
	}
	for i, tool := range tools {
		info, err := tool.Info(context.Background())
		if err != nil || info.Name != definitions[i].Name || info.Policy != definitions[i].Policy {
			t.Fatalf("tool %d: %+v, %v", i, info, err)
		}
		if tool.(ToolInputTypeProvider).InputType().Elem().Name() != "declaredInput" {
			t.Fatal("input schema type was lost")
		}
		result, err := tool.Invoke(context.Background(), ToolInvocation{Arguments: `{"value":"ok"}`})
		if err != nil || result.Content != "ok" {
			t.Fatalf("invoke: %+v, %v", result, err)
		}
	}
}

func TestToolDefinitionsRejectInvalidListsAtomically(t *testing.T) {
	handler := func(context.Context, *declaredInput) (string, error) { return "", nil }
	var nilHandler func(context.Context, *declaredInput) (string, error)
	for _, definitions := range [][]ToolDefinition{
		{{Name: "same", Handler: handler}, {Name: "same", Handler: handler}},
		{{Name: "valid", Handler: handler}, {Name: "bad", Handler: nilHandler}},
		{{Name: " ", Handler: handler}},
	} {
		if tools, err := NewTools(definitions); err == nil || tools != nil {
			t.Fatalf("tools=%v, err=%v", tools, err)
		}
	}
}
