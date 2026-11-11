package eventlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"fkteams/internal/domain/message"
	"fkteams/internal/runtime/events"
)

func TestCanonicalHistoryPreservesParallelCallsAndMessageMetadata(t *testing.T) {
	dir := t.TempDir()
	recorder := NewHistoryRecorder()
	recorder.SetSessionDir(dir)
	source := message.Message{
		Role: message.RoleAssistant, Name: "provider-name", Content: "calling", ReasoningContent: "reason",
		Extra:        map[string]any{"provider": "message-metadata"},
		ContentParts: []message.ContentPart{{Type: message.ContentPartText, Text: "calling", Extra: map[string]any{"provider": "part-metadata"}}},
		ToolCalls: []message.ToolCall{
			{ID: "call-a", Type: "function", Function: message.FunctionCall{Name: "lookup", Arguments: `{"key":"a"}`}, Extra: map[string]any{"provider": "first"}},
			{ID: "call-b", Type: "function", Function: message.FunctionCall{Name: "lookup", Arguments: `{"key":"b"}`}, Extra: map[string]any{"provider": "second"}},
		},
	}
	recorder.RecordEvent(Event{Type: events.EventAssistantCompleted, AgentName: "assistant", MessageID: "source", Message: &source})
	for _, call := range source.ToolCalls {
		recorder.RecordEvent(events.ToolCallStarted(events.ToolEvent{AgentName: "assistant", ToolCallID: call.ID, ToolCallRef: "tool_call:" + call.ID, ToolName: call.Function.Name, ToolCall: &call}))
	}
	for i := len(source.ToolCalls) - 1; i >= 0; i-- {
		call := source.ToolCalls[i]
		recorder.RecordEvent(events.ToolCallCompleted(events.ToolEvent{AgentName: "assistant", ToolCallID: call.ID, ToolCallRef: "tool_call:" + call.ID, ToolName: call.Function.Name, ToolResult: "result-" + call.ID}))
	}
	recorder.RecordEvent(Event{Type: events.EventAssistantCompleted, AgentName: "assistant", MessageID: "answer", Content: "done"})
	recorder.FinalizeCurrent()
	if err := recorder.Flush(); err != nil {
		t.Fatal(err)
	}
	loaded := NewHistoryRecorder()
	if err := loaded.LoadFromFile(filepath.Join(dir, TranscriptFileName)); err != nil {
		t.Fatal(err)
	}
	for _, history := range []*HistoryRecorder{recorder, loaded} {
		context := history.ProjectContextMessages(0)
		if len(context) != 4 || !reflect.DeepEqual(context[0], source) {
			t.Fatalf("model message was reconstructed from display fields: %#v", context)
		}
		if context[1].ToolCallID != "call-a" || context[2].ToolCallID != "call-b" || context[1].Content != "result-call-a" || context[2].Content != "result-call-b" {
			t.Fatalf("parallel tool results lost their identity: %#v", context)
		}
		// 展示方和模型调用方均不能修改 recorder 内持有的规范消息。
		snapshot := history.GetMessages()
		for _, event := range snapshot[0].Events {
			if event.Message != nil && len(event.Message.ToolCalls) > 0 {
				event.Message.ToolCalls[0].Extra["provider"] = "mutated"
				event.Message.ContentParts[0].Extra["provider"] = "mutated"
			}
			if event.ToolCall != nil {
				event.ToolCall.Name = "display-only"
			}
		}
		context[0].Extra["provider"] = "mutated"
		if !reflect.DeepEqual(history.ProjectContextMessages(0)[0], source) {
			t.Fatal("reading history changed canonical model state")
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, TranscriptFileName))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.SplitN(string(data), "\n", 2)[0]), &raw); err != nil {
		t.Fatal(err)
	}
	if raw["message"] == nil || raw["content"] != nil || raw["reasoning"] != nil || raw["content_parts"] != nil {
		t.Fatalf("transcript contains duplicate message facts: %s", data)
	}
}

func TestCanonicalHistoryFinalizesCancelledDeltas(t *testing.T) {
	dir := t.TempDir()
	recorder := NewHistoryRecorder()
	recorder.SetSessionDir(dir)
	recorder.RecordEvent(Event{Type: EventAssistantReasoning, AgentName: "assistant", MessageID: "partial", Content: "reason"})
	recorder.RecordEvent(Event{Type: EventAssistantText, AgentName: "assistant", MessageID: "partial", Content: "partial"})
	recorder.RecordCancelled("cancelled")
	if err := recorder.Flush(); err != nil {
		t.Fatal(err)
	}
	loaded := NewHistoryRecorder()
	if err := loaded.LoadFromFile(filepath.Join(dir, TranscriptFileName)); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(recorder.ProjectContextMessages(0), loaded.ProjectContextMessages(0)) {
		t.Fatalf("cancelled history changed after reload: live=%#v restored=%#v", recorder.ProjectContextMessages(0), loaded.ProjectContextMessages(0))
	}
}

func TestCanonicalHistoryPairsInterruptedToolCalls(t *testing.T) {
	for _, exit := range []string{"cancel", "error"} {
		t.Run(exit, func(t *testing.T) {
			dir := t.TempDir()
			recorder := NewHistoryRecorder()
			recorder.SetSessionDir(dir)
			source := message.Message{Role: message.RoleAssistant, ToolCalls: []message.ToolCall{{
				ID: "unfinished", Function: message.FunctionCall{Name: "lookup", Arguments: `{}`},
			}}}
			recorder.RecordEvent(Event{Type: events.EventAssistantCompleted, AgentName: "assistant", MessageID: "tools", Message: &source})
			if exit == "cancel" {
				recorder.RecordCancelled("cancelled before executing tool")
			} else {
				recorder.RecordEvent(Event{Type: events.EventError, AgentName: "assistant", Error: "model failed before executing tool"})
				recorder.FinalizeCurrent()
			}
			loaded := NewHistoryRecorder()
			if err := loaded.LoadFromFile(filepath.Join(dir, TranscriptFileName)); err != nil {
				t.Fatal(err)
			}
			for _, history := range []*HistoryRecorder{recorder, loaded} {
				context := history.ProjectContextMessages(0)
				if len(context) < 2 || context[0].Role != message.RoleAssistant || len(context[0].ToolCalls) != 1 || context[1].Role != message.RoleTool || context[1].ToolCallID != "unfinished" {
					t.Fatalf("interrupted calls produced an invalid model history: %#v", context)
				}
			}
		})
	}
}

func TestLegacyTranscriptNormalizesBeforeContextProjection(t *testing.T) {
	dir := t.TempDir()
	legacy := `{"id":"user","at":"2026-01-01T00:00:00Z","type":"user_message","content":"question"}
{"id":"answer","at":"2026-01-01T00:00:01Z","type":"assistant_message","agent":"assistant","content":"answer","reasoning":"reason"}
{"id":"tool-start","at":"2026-01-01T00:00:02Z","type":"tool_call_start","agent":"assistant","call_id":"call","name":"lookup","args":"{}","extra":{"provider":"legacy-metadata"}}
{"id":"tool-end","at":"2026-01-01T00:00:03Z","type":"tool_call_end","agent":"assistant","call_id":"call","result":"result"}
`
	path := filepath.Join(dir, TranscriptFileName)
	if err := os.WriteFile(path, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}
	loaded := NewHistoryRecorder()
	if err := loaded.LoadFromFile(path); err != nil {
		t.Fatal(err)
	}
	context := loaded.ProjectContextMessages(0)
	if len(context) != 4 || context[0].Content != "question" || context[1].Content != "answer" || context[1].ReasoningContent != "reason" {
		t.Fatalf("legacy message context = %#v", context)
	}
	if len(context[2].ToolCalls) != 1 || context[2].ToolCalls[0].Extra["provider"] != "legacy-metadata" || context[3].ToolCallID != "call" || context[3].ToolName != "lookup" || context[3].Content != "result" {
		t.Fatalf("legacy tool context = %#v", context)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != legacy {
		t.Fatal("reading a legacy transcript rewrote stored history")
	}
}
