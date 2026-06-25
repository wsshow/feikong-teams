package dispatch

import (
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

func TestExtractMessage(t *testing.T) {
	msg := schema.AssistantMessage("完成", nil)
	event := &adk.AgentEvent{
		Output: &adk.AgentOutput{
			MessageOutput: &adk.MessageVariant{Message: msg},
		},
	}

	if got := extractMessage(event); got != msg {
		t.Fatalf("extractMessage = %#v, want original message", got)
	}
	if got := extractMessage(&adk.AgentEvent{}); got != nil {
		t.Fatalf("extractMessage empty event = %#v, want nil", got)
	}
}

func TestExtractOperations(t *testing.T) {
	longArgs := strings.Repeat("参数", 70)
	msg := schema.AssistantMessage("", []schema.ToolCall{
		{
			Function: schema.FunctionCall{Name: "read_file", Arguments: `{"path":"README.md"}`},
		},
		{
			Function: schema.FunctionCall{Name: "search", Arguments: longArgs},
		},
	})
	event := &adk.AgentEvent{
		Output: &adk.AgentOutput{
			MessageOutput: &adk.MessageVariant{Message: msg},
		},
	}

	got := extractOperations(event)
	if len(got) != 2 {
		t.Fatalf("extractOperations count = %d, want 2: %#v", len(got), got)
	}
	if got[0] != `read_file({"path":"README.md"})` {
		t.Fatalf("first operation = %q", got[0])
	}
	if !strings.HasPrefix(got[1], "search(") || !strings.HasSuffix(got[1], "...)") {
		t.Fatalf("second operation should be truncated, got %q", got[1])
	}
	if len([]rune(strings.TrimPrefix(strings.TrimSuffix(got[1], ")"), "search("))) != 123 {
		t.Fatalf("truncated args length = %d, want 123 including ellipsis", len([]rune(strings.TrimPrefix(strings.TrimSuffix(got[1], ")"), "search("))))
	}
}
