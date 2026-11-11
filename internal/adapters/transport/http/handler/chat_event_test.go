package handler

import (
	"bytes"
	"context"
	"encoding/json"
	domainevent "fkteams/internal/domain/event"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	eventlog "fkteams/internal/adapters/storage/file/history"
	"fkteams/internal/app/agent/catalog/toolmeta"
	"fkteams/internal/app/chat/taskstream"
	"fkteams/internal/app/tools/ask"
	"fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/events"
)

func TestConvertEventToMapKeepsFrontendStreamAndMemberMetadata(t *testing.T) {
	toolIndex := 0
	toolName := "ask_fkagent_event_flow_member"
	displays := toolmeta.NewRegistry()
	displays.RegisterAgentToolDisplay(toolName, "Event Flow Member")
	event := events.NormalizeEvent(events.Event{
		Type:             events.EventAssistantText,
		Sequence:         42,
		CreatedAt:        time.Unix(100, 0).UTC(),
		MessageID:        "msg_member_1",
		AgentName:        "event_flow_member",
		Role:             message.RoleAssistant,
		DeltaKind:        events.DeltaToolArgs,
		Content:          `{"request":`,
		ToolCallID:       "tool-call-1",
		ToolCallRef:      "ref-tool-call-1",
		ToolName:         toolName,
		ToolCallIndex:    &toolIndex,
		MemberCallID:     "member-call-1",
		MemberToolName:   "ask_fkagent_parent_member",
		MemberName:       "Event Flow Member",
		MemberOrder:      &toolIndex,
		ParentToolCallID: "member-call-1",
		ParentToolName:   "ask_fkagent_parent_member",
		ToolCalls: []message.ToolCall{{
			ID:    "tool-call-1",
			Index: &toolIndex,
			Type:  "function",
			Function: message.FunctionCall{
				Name:      toolName,
				Arguments: `{"request":"hello"}`,
			},
		}},
		ToolCallRefs: map[int]string{0: "ref-tool-call-1"},
	})

	got := convertEventToMapWithResolver(event, displays)
	requireMapValue(t, got, "type", events.EventAssistantText)
	requireMapValue(t, got, "sequence", int64(42))
	requireMapValue(t, got, "message_id", "msg_member_1")
	requireMapValue(t, got, "stream_id", "msg_member_1:tool_args")
	requireMapValue(t, got, "chunk_index", int64(42))
	requireMapValue(t, got, "delta_kind", events.DeltaToolArgs)
	requireMapValue(t, got, "content", `{"request":`)
	requireMapValue(t, got, "tool_call_id", "tool-call-1")
	requireMapValue(t, got, "tool_call_ref", "ref-tool-call-1")
	requireMapValue(t, got, "tool_name", toolName)
	requireMapValue(t, got, "tool_display_name", "指派给 Event Flow Member")
	requireMapValue(t, got, "tool_kind", toolmeta.ToolKindAgent)
	requireMapValue(t, got, "tool_target", "Event Flow Member")
	requireMapValue(t, got, "tool_call_index", 0)
	requireMapValue(t, got, "is_member_event", true)
	requireMapValue(t, got, "member_call_id", "member-call-1")
	requireMapValue(t, got, "member_tool_name", "ask_fkagent_parent_member")
	requireMapValue(t, got, "member_name", "Event Flow Member")
	requireMapValue(t, got, "member_order", 0)
	requireMapValue(t, got, "parent_tool_call_id", "member-call-1")
	requireMapValue(t, got, "parent_tool_name", "ask_fkagent_parent_member")

	toolCalls, ok := got["tool_calls"].([]map[string]any)
	if !ok || len(toolCalls) != 1 {
		t.Fatalf("expected one tool call map, got %#v", got["tool_calls"])
	}
	requireMapValue(t, toolCalls[0], "id", "tool-call-1")
	requireMapValue(t, toolCalls[0], "index", 0)
	requireMapValue(t, toolCalls[0], "ref", "ref-tool-call-1")
	requireMapValue(t, toolCalls[0], "name", toolName)
	requireMapValue(t, toolCalls[0], "display_name", "指派给 Event Flow Member")
	requireMapValue(t, toolCalls[0], "kind", toolmeta.ToolKindAgent)
	requireMapValue(t, toolCalls[0], "target", "Event Flow Member")
	requireMapValue(t, toolCalls[0], "arguments", `{"request":"hello"}`)
}

func TestConvertEventToMapOmitsStreamMetadataForNonDeltaEvents(t *testing.T) {
	got := convertEventToMap(events.Event{
		Type:      events.EventAssistantCompleted,
		Sequence:  7,
		MessageID: "msg_1",
		Role:      message.RoleAssistant,
		Content:   "done",
	})

	if _, ok := got["stream_id"]; ok {
		t.Fatalf("stream_id should be omitted for non-delta event: %#v", got)
	}
	if _, ok := got["chunk_index"]; ok {
		t.Fatalf("chunk_index should be omitted for non-delta event: %#v", got)
	}
}

func TestConvertEventToMapKeepsRunAndTurnID(t *testing.T) {
	got := convertEventToMap(events.Event{
		Type:   events.EventAssistantStarted,
		RunID:  "run-1",
		TurnID: "run-1:turn:1",
		Role:   message.RoleAssistant,
	})

	requireMapValue(t, got, "run_id", "run-1")
	requireMapValue(t, got, "turn_id", "run-1:turn:1")
}

func TestConvertEventToMapAddsFriendlyErrorFields(t *testing.T) {
	got := convertEventToMap(events.Event{
		Type:  events.EventError,
		Error: "deepseek does not support image_url type",
	})

	requireMapValue(t, got, "error", "deepseek does not support image_url type")
	requireMapValue(t, got, "error_code", "model_unsupported_image_input")
	requireMapValue(t, got, "error_title", "当前模型不支持图片输入")
	if got["display_error"] == "" || got["technical_error"] == "" {
		t.Fatalf("friendly error fields missing: %#v", got)
	}
	if _, ok := got["error_suggestions"].([]string); !ok {
		t.Fatalf("error_suggestions = %#v, want []string", got["error_suggestions"])
	}
}

func TestConvertContentPartsPreservesAttachmentName(t *testing.T) {
	parts := convertContentParts([]ContentPart{
		{Type: "image_base64", Name: "image.png", Base64Data: "abc", MIMEType: "image/png"},
		{Type: "file_url", Name: "report.pdf", URL: "chat-attachments/report.pdf"},
	})

	if len(parts) != 2 {
		t.Fatalf("parts len = %d, want 2", len(parts))
	}
	if parts[0].Name != "image.png" || parts[1].Name != "report.pdf" {
		t.Fatalf("names not preserved: %#v", parts)
	}
}

func TestConvertEventToMapMergesTopLevelToolRefIntoSingleToolCall(t *testing.T) {
	toolIndex := 0
	got := convertEventToMap(events.Event{
		Type:          events.EventToolCallStarted,
		ToolCallID:    "tool-call-1",
		ToolCallRef:   "ref-tool-call-1",
		ToolName:      "single_tool",
		ToolCallIndex: &toolIndex,
		ToolCall: &message.ToolCall{
			ID:    "tool-call-1",
			Index: &toolIndex,
			Type:  "function",
			Function: message.FunctionCall{
				Name:      "single_tool",
				Arguments: `{"ok":true}`,
			},
		},
	})

	toolCalls, ok := got["tool_calls"].([]map[string]any)
	if !ok || len(toolCalls) != 1 {
		t.Fatalf("expected one tool call map, got %#v", got["tool_calls"])
	}
	requireMapValue(t, toolCalls[0], "ref", "ref-tool-call-1")
	requireMapValue(t, toolCalls[0], "arguments", `{"ok":true}`)
	toolCall, ok := got["tool_call"].(map[string]any)
	if !ok {
		t.Fatalf("expected singular tool_call for one call, got %#v", got["tool_call"])
	}
	requireMapValue(t, toolCall, "ref", "ref-tool-call-1")
}

func TestConvertEventToMapDoesNotExposeSingularToolCallForMultipleCalls(t *testing.T) {
	got := convertEventToMap(events.Event{
		Type: events.EventAssistantCompleted,
		ToolCalls: []message.ToolCall{
			{ID: "tool-call-1", Function: message.FunctionCall{Name: "first_tool"}},
			{ID: "tool-call-2", Function: message.FunctionCall{Name: "second_tool"}},
		},
	})

	if _, ok := got["tool_call"]; ok {
		t.Fatalf("tool_call should be omitted for multiple calls: %#v", got)
	}
	toolCalls, ok := got["tool_calls"].([]map[string]any)
	if !ok || len(toolCalls) != 2 {
		t.Fatalf("expected two tool call maps, got %#v", got["tool_calls"])
	}
}

func TestConvertEventToMapUsesPositionToolRefsWhenToolCallIndexMissing(t *testing.T) {
	got := convertEventToMap(events.Event{
		Type: events.EventAssistantCompleted,
		ToolCalls: []message.ToolCall{
			{ID: "tool-call-1", Function: message.FunctionCall{Name: "first_tool"}},
			{ID: "tool-call-2", Function: message.FunctionCall{Name: "second_tool"}},
		},
		ToolCallRefs: map[int]string{
			0: "tool_call:tool-call-1",
			1: "tool_call:tool-call-2",
		},
	})

	toolCalls, ok := got["tool_calls"].([]map[string]any)
	if !ok || len(toolCalls) != 2 {
		t.Fatalf("expected two tool call maps, got %#v", got["tool_calls"])
	}
	requireMapValue(t, toolCalls[0], "ref", "tool_call:tool-call-1")
	requireMapValue(t, toolCalls[1], "ref", "tool_call:tool-call-2")
}

func TestAskInterruptIDPrefersRootCause(t *testing.T) {
	got := askInterruptID([]runtimeport.Interrupt{
		{ID: "wrapper"},
		{ID: "root", IsRootCause: true, Info: &ask.AskInfo{Question: "choose"}},
	})
	if got != "root" {
		t.Fatalf("askInterruptID = %q, want root", got)
	}
}

func TestExtractAskInterruptUsesAskRootCauseMetadata(t *testing.T) {
	order := 2
	got := extractAskInterrupt([]runtimeport.Interrupt{
		{ID: "wrapper", IsRootCause: true, Info: "approval"},
		{
			ID:             "ask-root",
			IsRootCause:    true,
			Info:           &ask.AskInfo{Question: "choose"},
			MemberCallID:   "member-call",
			MemberToolName: "ask_fkagent_member",
			MemberName:     "Member",
			MemberOrder:    &order,
		},
	})
	if got == nil {
		t.Fatal("expected ask interrupt")
	}
	if got.ID != "ask-root" || got.Info.Question != "choose" {
		t.Fatalf("unexpected ask interrupt: %#v", got)
	}
	if got.Event.MemberCallID != "member-call" || got.Event.MemberOrder == nil || *got.Event.MemberOrder != order {
		t.Fatalf("member metadata was not preserved: %#v", got.Event)
	}
}

func TestMemberAskRuntimeHandlerPublishesOrderedAskNotification(t *testing.T) {
	stream := taskstream.NewManager().Register(taskstream.StreamConfig{
		SessionID: "session-member-ask-order",
		Cancel:    func() {},
	})
	recorder := eventlog.NewHistoryRecorder()
	handler := buildMemberAskRuntimeHandler(stream, recorder, "session-member-ask-order")
	memberOrder := 1

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	respCh := make(chan *ask.AskResponse, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, err := handler(ctx, ask.RuntimeRequest{
			ID:         "ask-order-1",
			ToolCallID: "ask-tool-call-1",
			ToolName:   "ask_questions",
			Info: &ask.AskInfo{
				Question:    "Choose one?",
				Options:     []string{"A", "B"},
				MultiSelect: true,
			},
			Metadata: runtimeport.InterruptMetadata{
				MemberCallID:   "member-call-1",
				MemberToolName: "ask_fkagent_member",
				MemberName:     "Member",
				MemberOrder:    &memberOrder,
			},
		})
		if err != nil {
			errCh <- err
			return
		}
		respCh <- resp
	}()

	var askPayload map[string]any
	deadline := time.After(time.Second)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for askPayload == nil {
		select {
		case err := <-errCh:
			t.Fatalf("handler returned before ask response: %v", err)
		case <-deadline:
			t.Fatal("timed out waiting for ask notification")
		case <-ticker.C:
			for _, item := range stream.EventsSince(0) {
				if item.Data["type"] == events.EventAskRequested {
					askPayload = item.Data
					break
				}
			}
		}
	}

	requireMapValue(t, askPayload, "type", events.EventAskRequested)
	requireMapValue(t, askPayload, "session_id", "session-member-ask-order")
	requireMapValue(t, askPayload, "ask_id", "ask-order-1")
	requireMapValue(t, askPayload, "detail", "ask-order-1")
	requireMapValue(t, askPayload, "question", "Choose one?")
	requireMapValue(t, askPayload, "multi_select", true)
	requireMapValue(t, askPayload, "tool_call_id", "ask-tool-call-1")
	requireMapValue(t, askPayload, "tool_call_ref", "tool_call:ask-tool-call-1")
	requireMapValue(t, askPayload, "tool_name", "ask_questions")
	requireMapValue(t, askPayload, "is_member_event", true)
	requireMapValue(t, askPayload, "member_call_id", "member-call-1")
	requireMapValue(t, askPayload, "member_tool_name", "ask_fkagent_member")
	requireMapValue(t, askPayload, "member_name", "Member")
	requireMapValue(t, askPayload, "member_order", memberOrder)
	if got, ok := askPayload["sequence"].(int64); !ok || got == 0 {
		t.Fatalf("ask notification sequence = %#v, want non-zero int64", askPayload["sequence"])
	}
	if got, ok := askPayload["event_id"].(string); !ok || got == "" {
		t.Fatalf("ask notification event_id = %#v, want non-empty string", askPayload["event_id"])
	}
	if got, ok := askPayload["created_at"].(time.Time); !ok || got.IsZero() {
		t.Fatalf("ask notification created_at = %#v, want non-zero time", askPayload["created_at"])
	}

	if err := stream.SubmitAskResponse("ask-order-1", &ask.AskResponse{
		AskID:    "ask-order-1",
		Selected: []string{"A"},
	}); err != nil {
		t.Fatalf("submit ask response: %v", err)
	}

	select {
	case err := <-errCh:
		t.Fatalf("handler returned error after response: %v", err)
	case resp := <-respCh:
		if resp.AskID != "ask-order-1" || len(resp.Selected) != 1 || resp.Selected[0] != "A" {
			t.Fatalf("unexpected ask response: %#v", resp)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for handler to resume")
	}
}

func requireMapValue(t *testing.T, got map[string]any, key string, want any) {
	t.Helper()
	if got[key] != want {
		t.Fatalf("unexpected %s: got %#v, want %#v; map=%#v", key, got[key], want, got)
	}
}

// TestFrontendEventContractFixture 固定后端实际投影，供 Bun 测试验证相同的线协议。
func TestFrontendEventContractFixture(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	types := []domainevent.Type{
		domainevent.TypeAgentStarted, domainevent.TypeAgentCompleted, domainevent.TypeTurnStarted, domainevent.TypeTurnCompleted,
		domainevent.TypeUserMessage, domainevent.TypeAssistantStarted, domainevent.TypeAssistantReasoning, domainevent.TypeAssistantText, domainevent.TypeAssistantCompleted,
		domainevent.TypeToolCallStarted, domainevent.TypeToolCallArguments, domainevent.TypeToolCallResult, domainevent.TypeToolCallCompleted, domainevent.TypeToolCallFailed,
		domainevent.TypeAskRequested, domainevent.TypeAskAnswered, domainevent.TypeApprovalRequested, domainevent.TypeApprovalAnswered,
		domainevent.TypeSystemNotice, domainevent.TypeUsageReported, domainevent.TypeError, domainevent.TypeProcessingStart, domainevent.TypeProcessingEnd, domainevent.TypeQueueUpdated,
		domainevent.TypeCancelled, domainevent.TypeTurnFailed, domainevent.TypeTurnCancelled, domainevent.TypeMemberStarted, domainevent.TypeMemberCompleted,
		domainevent.Type(domainevent.NotifyConnected), domainevent.Type(domainevent.NotifyPong), domainevent.Type(domainevent.NotifyInvalidAPIKey),
	}
	var live []taskstream.Event
	for i, typ := range types {
		e := events.Event{Type: typ, EventID: fmt.Sprintf("event-%d", i+1), Sequence: int64(i + 1), CreatedAt: at, RunID: "run", TurnID: "turn", AgentName: "assistant"}
		switch typ {
		case domainevent.TypeUserMessage:
			e.Message = &message.Message{Role: message.RoleUser, ContentParts: []message.ContentPart{{Type: message.ContentPartImageURL, URL: "/image.png"}}}
		case domainevent.TypeAssistantStarted, domainevent.TypeAssistantCompleted, domainevent.TypeAssistantText, domainevent.TypeAssistantReasoning:
			e.MessageID = "message"
			e.Role = message.RoleAssistant
			e.Content = "answer"
			if typ == domainevent.TypeAssistantReasoning {
				e.DeltaKind = events.DeltaReasoning
			} else {
				e.DeltaKind = events.DeltaOutput
			}
		case domainevent.TypeToolCallStarted, domainevent.TypeToolCallArguments, domainevent.TypeToolCallResult, domainevent.TypeToolCallCompleted, domainevent.TypeToolCallFailed:
			e.ToolName = "read_file"
			e.ToolCallID = "call"
			e.ToolCallRef = "tool_call:call"
			e.ToolCall = &message.ToolCall{ID: "call", Function: message.FunctionCall{Name: "read_file", Arguments: `{"path":"note.txt"}`}}
			if typ != domainevent.TypeToolCallFailed {
				e.Content = "result"
			}
		case domainevent.TypeAskRequested, domainevent.TypeAskAnswered:
			e.Ask = &events.AskPayload{ID: "ask", Question: "choose", Options: []string{"A", "B"}, MultiSelect: true, Selected: []string{"A"}, FreeText: "answer"}
		case domainevent.TypeApprovalRequested, domainevent.TypeApprovalAnswered:
			e.Detail = "approval"
			e.Approval = &events.ApprovalPayload{ID: "approval", Message: "continue?", Decision: "approved"}
		case domainevent.TypeSystemNotice:
			e.Content = "notice"
			e.Notice = &events.NoticePayload{Level: "info", Message: "notice"}
		case domainevent.TypeUsageReported:
			e.Usage = &events.UsagePayload{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5}
		case domainevent.TypeError, domainevent.TypeTurnFailed, domainevent.Type(domainevent.NotifyInvalidAPIKey):
			e.Error = "request failed"
		case domainevent.TypeMemberStarted, domainevent.TypeMemberCompleted:
			e.MemberCallID = "member"
			e.MemberName = "researcher"
			e.ParentToolCallID = "parent"
		}
		payload := standardEventPayload("session", e, nil)
		if typ == domainevent.TypeQueueUpdated {
			payload["queue"] = []taskstream.QueuedMessage{{ID: "queued", Kind: taskstream.QueueSteering, Text: "next", CreatedAt: at}}
			payload["queued_count"] = 1
		}
		live = append(live, payload)
	}
	rt := &Runtime{RuntimeOptions: RuntimeOptions{ToolDisplays: toolmeta.NewRegistry()}}
	history := rt.transcriptToChatEvents("session", []eventlog.TranscriptEvent{
		{ID: "history-user", At: at, Type: eventlog.TranscriptUserMessage, Content: "question"},
		{ID: "history-answer", At: at, Type: eventlog.TranscriptAssistantMessage, Content: "answer", Agent: "assistant", Usage: &eventlog.UsageRecord{TotalTokens: 5}},
		{ID: "history-tool", At: at, Type: eventlog.TranscriptToolCallEnd, CallID: "call", Name: "read_file", Content: "result"},
		{ID: "history-cancel", At: at, Type: eventlog.TranscriptCancelled, Content: "cancelled"},
	})
	data, err := json.MarshalIndent(struct {
		Live    []taskstream.Event `json:"live"`
		History []map[string]any   `json:"history"`
	}{live, history}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	path := filepath.Join("..", "..", "..", "..", "..", "web", "src", "types", "testdata", "chat-events.json")
	if os.Getenv("FKTEAMS_UPDATE_PROTOCOL_FIXTURE") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(expected, data) {
		t.Fatal("event contract changed; review and regenerate with FKTEAMS_UPDATE_PROTOCOL_FIXTURE=1 go test ./internal/adapters/transport/http/handler -run TestFrontendEventContractFixture")
	}
}
