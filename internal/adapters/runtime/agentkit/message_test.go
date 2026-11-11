package agentkit

import (
	"bytes"
	"path/filepath"
	"reflect"
	"testing"

	eventlog "fkteams/internal/adapters/storage/file/history"
	appchat "fkteams/internal/app/chat"
	domainmessage "fkteams/internal/domain/message"
	"fkteams/internal/runtime/events"

	geminiModel "github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino/schema"
)

func TestToolMetadataSurvivesPersistedTurnHistory(t *testing.T) {
	signature := []byte("synthetic-thought-signature")
	input := &schema.Message{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{
		ID: "call-1", Type: "function", Index: intPtr(0),
		Function: schema.FunctionCall{Name: "lookup", Arguments: `{}`},
		Extra: map[string]any{
			"gemini_thought_signature": signature,
			"provider":                 map[string]any{"parts": []any{map[string]any{"value": "original"}}},
		},
	}}}
	message := adaptMessageFromRunner(input)
	// SDK 后续修改分片时，已经交给应用的元数据必须保持独立。
	signature[0] = 'x'
	input.ToolCalls[0].Extra["provider"].(map[string]any)["parts"].([]any)[0].(map[string]any)["value"] = "changed"
	if got := message.ToolCalls[0].Extra["provider"].(map[string]any)["parts"].([]any)[0].(map[string]any)["value"]; got != "original" {
		t.Fatalf("metadata was aliased: %v", got)
	}
	dir := t.TempDir()
	recorder := eventlog.NewHistoryRecorder()
	recorder.SetSessionDir(dir)
	call := message.ToolCalls[0]
	recorder.RecordEvent(events.ToolCallStarted(events.ToolEvent{
		AgentName: "assistant", ToolCallID: call.ID, ToolCallRef: "tool_call:" + call.ID,
		ToolName: call.Function.Name, ToolArgs: call.Function.Arguments, ToolCall: &call,
	}))
	recorder.RecordEvent(events.ToolCallCompleted(events.ToolEvent{
		AgentName: "assistant", ToolCallID: call.ID, ToolCallRef: "tool_call:" + call.ID,
		ToolName: call.Function.Name, ToolResult: "result",
	}))
	recorder.FinalizeCurrent()
	if err := recorder.Flush(); err != nil {
		t.Fatal(err)
	}
	loaded := eventlog.NewHistoryRecorder()
	if err := loaded.LoadFromFile(filepath.Join(dir, eventlog.TranscriptFileName)); err != nil {
		t.Fatal(err)
	}
	for _, history := range []*eventlog.HistoryRecorder{recorder, loaded} {
		next := AdaptMessages(appchat.BuildTurnInput(history, "next").Context)
		if len(next) != 2 || len(next[0].ToolCalls) != 1 {
			t.Fatalf("restored messages = %#v", next)
		}
		got, ok := geminiModel.GetThoughtSignatureFromExtra(next[0].ToolCalls[0].Extra)
		if !ok || !bytes.Equal(got, []byte("synthetic-thought-signature")) {
			t.Fatalf("restored signature = %q, exists = %v", got, ok)
		}
		if !reflect.DeepEqual(next[0].ToolCalls[0].Extra["provider"], message.ToolCalls[0].Extra["provider"]) {
			t.Fatalf("nested provider metadata lost: %#v", next[0].ToolCalls[0].Extra)
		}
	}
}

func TestAdaptMediaReferencesRoundTrip(t *testing.T) {
	for _, role := range []domainmessage.Role{domainmessage.RoleUser, domainmessage.RoleAssistant} {
		for _, kind := range []domainmessage.ContentPartType{
			domainmessage.ContentPartImageURL, domainmessage.ContentPartAudioURL,
			domainmessage.ContentPartVideoURL, domainmessage.ContentPartFileURL,
		} {
			for _, source := range []string{"url", "base64", "file_id"} {
				t.Run(string(role)+"/"+string(kind)+"/"+source, func(t *testing.T) {
					part := domainmessage.ContentPart{Type: kind, MIMEType: "application/octet-stream", Extra: map[string]any{"provider": "part-metadata"}}
					switch source {
					case "url":
						part.URL = "https://example.com/media"
					case "base64":
						part.Base64Data = "bWVkaWE="
					case "file_id":
						part.FileID = "files/provider-media"
					}
					input := []domainmessage.Message{{Role: role, ContentParts: []domainmessage.ContentPart{part}, Extra: map[string]any{"provider": "message-metadata"}}}
					got := AdaptMessagesFromRunner(AdaptMessages(input))
					if len(got) != 1 || got[0].Role != role || !reflect.DeepEqual(got[0].ContentParts, input[0].ContentParts) || !reflect.DeepEqual(got[0].Extra, input[0].Extra) {
						t.Fatalf("media round trip = %#v, want %#v", got, input)
					}
				})
			}
		}
	}
}
