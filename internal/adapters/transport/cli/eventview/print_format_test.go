package eventview

import (
	domainevent "fkteams/internal/domain/event"
	domainmessage "fkteams/internal/domain/message"
	"strings"
	"testing"
)

func TestFormatSearchResultsLimitsAndRendersEntries(t *testing.T) {
	content := `{"message":"搜索完成","results":[
		{"title":"A","url":"https://example.com/a","summary":"第一条\n摘要"},
		{"title":"B","url":"https://example.com/b","summary":"第二条"},
		{"title":"C","url":"https://example.com/c","summary":"第三条"},
		{"title":"D","url":"https://example.com/d","summary":"第四条"},
		{"title":"E","url":"https://example.com/e","summary":"第五条"},
		{"title":"F","url":"https://example.com/f","summary":"第六条"}
	]}`

	out := formatSearchResults(content)

	for _, want := range []string{"搜索完成", "1. A", "URL: https://example.com/a", "第一条 摘要", "... 还有 1 条结果"} {
		if !strings.Contains(out, want) {
			t.Fatalf("formatted search result missing %q: %q", want, out)
		}
	}
	if strings.Contains(out, "6. F") {
		t.Fatalf("formatted search result should limit entries: %q", out)
	}
	if formatSearchResults("bad json") != "" {
		t.Fatal("invalid search JSON should return empty output")
	}
}

func TestFormatCommandResultHandlesSuccessAndError(t *testing.T) {
	success := formatCommandResult(`{"stdout":"line1\nline2\n","exit_code":0,"execution_time":"1ms"}`)
	if !strings.Contains(success, "执行成功") || !strings.Contains(success, "line1") {
		t.Fatalf("success command output = %q", success)
	}

	failed := formatCommandResult(`{"stderr":"boom\n","exit_code":2,"execution_time":"2ms"}`)
	if !strings.Contains(failed, "执行失败") || !strings.Contains(failed, "标准错误") || !strings.Contains(failed, "boom") {
		t.Fatalf("failed command output = %q", failed)
	}

	withError := formatCommandResult(`{"error_message":"denied"}`)
	if !strings.Contains(withError, "denied") || strings.Contains(withError, "执行成功") {
		t.Fatalf("error command output = %q", withError)
	}
}

func TestFormatFileOpResultRendersSuccessAndFailure(t *testing.T) {
	success := formatFileOpResult(`{"message":"已读取","file_path":"a.txt","content":"one\ntwo","size":12}`)
	for _, want := range []string{"操作成功", "已读取", "路径: a.txt", "内容:", "one"} {
		if !strings.Contains(success, want) {
			t.Fatalf("file success output missing %q: %q", want, success)
		}
	}

	failure := formatFileOpResult(`{"error_message":"missing"}`)
	if !strings.Contains(failure, "操作失败") || !strings.Contains(failure, "missing") {
		t.Fatalf("file failure output = %q", failure)
	}
}

func TestFormatToolResultForPrintRoutesKnownTools(t *testing.T) {
	if got := formatToolResultForPrint("search", `{"message":"ok"}`); !strings.Contains(got, "ok") {
		t.Fatalf("search result = %q", got)
	}
	if got := formatToolResultForPrint("execute", `{"exit_code":0,"execution_time":"1ms"}`); !strings.Contains(got, "执行成功") {
		t.Fatalf("command result = %q", got)
	}
	if got := formatToolResultForPrint("unknown", `{}`); got != "" {
		t.Fatalf("unknown tool result = %q, want empty", got)
	}
}

func TestMarkdownCollectorCollectsMessagesToolsAndErrors(t *testing.T) {
	callback, result := NewMarkdownCollector()

	eventsToSend := []Event{
		{Type: EventMessageDelta, AgentName: "assistant", Content: "hello"},
		{Type: EventMessageDelta, AgentName: "assistant", Content: " world"},
		{
			Type:      EventToolStart,
			AgentName: "assistant",
			ToolCalls: []domainmessage.ToolCall{{
				ID: "tool-1",
				Function: domainmessage.FunctionCall{
					Name:      "search",
					Arguments: `{"query":"go"}`,
				},
			}},
		},
		{Type: EventToolEnd, AgentName: "assistant", ToolCallID: "tool-1", Content: `{"message":"搜索完成","results":[{"title":"Go","url":"https://go.dev","summary":"语言"}]}`},
		{Type: EventAction, ActionType: domainevent.ActionTransfer, AgentName: "assistant", Content: "coder"},
		{Type: EventError, AgentName: "assistant", Error: "boom"},
	}
	for _, event := range eventsToSend {
		if err := callback(event); err != nil {
			t.Fatalf("callback error: %v", err)
		}
	}

	out := result()
	for _, want := range []string{"**[assistant]**", "hello world", "调用:", "搜索完成", "**Go** <https://go.dev>", "→ coder", "错误 [assistant]"} {
		if !strings.Contains(out, want) {
			t.Fatalf("markdown output missing %q: %q", want, out)
		}
	}
}

func TestMarkdownCollectorIgnoresNonOutputDeltaAndInternalTool(t *testing.T) {
	callback, result := NewMarkdownCollector()
	if err := callback(Event{Type: EventMessageDelta, DeltaKind: domainevent.DeltaReasoning, AgentName: "assistant", Content: "hidden"}); err != nil {
		t.Fatal(err)
	}
	if err := callback(Event{
		Type:      EventToolStart,
		AgentName: "assistant",
		ToolCalls: []domainmessage.ToolCall{{
			ID: "tool-1",
			Function: domainmessage.FunctionCall{
				Name:      "continue_output",
				Arguments: "{}",
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if got := result(); got != "" {
		t.Fatalf("collector output = %q, want empty", got)
	}
}
