package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"fkteams/internal/adapters/storage/file/history"
	"fkteams/internal/domain/message"
	runtimeevents "fkteams/internal/runtime/events"

	"github.com/gin-gonic/gin"
)

func TestGetSessionReturnsEmptyEventsWhenHistoryFileMissing(t *testing.T) {
	rt := newTestRuntime(t)
	gin.SetMode(gin.TestMode)

	sessionID := "empty-session"
	if err := eventlog.SaveMetadata(rt.sessionDirPath(sessionID), &eventlog.SessionMetadata{
		ID:           sessionID,
		Title:        "empty",
		Status:       "idle",
		CurrentAgent: "coder",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}); err != nil {
		t.Fatalf("save metadata: %v", err)
	}

	router := gin.New()
	router.GET("/sessions/:sessionID", rt.GetSessionHandler())

	req := httptest.NewRequest(http.MethodGet, "/sessions/"+sessionID, nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var got Response
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	data, ok := got.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected response data object, got %#v", got.Data)
	}
	if data["session_id"] != sessionID {
		t.Fatalf("unexpected session_id: %#v", data["session_id"])
	}
	if data["current_agent"] != "coder" {
		t.Fatalf("unexpected current_agent: %#v", data["current_agent"])
	}
	gotEvents, ok := data["events"].([]any)
	if !ok {
		t.Fatalf("expected events array, got %#v", data["events"])
	}
	if len(gotEvents) != 0 {
		t.Fatalf("expected empty events, got %#v", gotEvents)
	}
}

func TestGetSessionReturnsEventsInHistoryOrder(t *testing.T) {
	rt := newTestRuntime(t)
	gin.SetMode(gin.TestMode)

	sessionID := "ordered-session"
	sessionDir := rt.sessionDirPath(sessionID)
	if err := eventlog.SaveMetadata(sessionDir, &eventlog.SessionMetadata{
		ID:           sessionID,
		Title:        "ordered",
		Status:       "idle",
		CurrentAgent: "coordinator",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}); err != nil {
		t.Fatalf("save metadata: %v", err)
	}
	recorder := eventlog.NewHistoryRecorder()
	recorder.SetSessionDir(sessionDir)
	recorder.RecordEvent(runtimeevents.UserMessage("run-1", runtimeevents.TurnID("run-1", 1), "run-1:user", message.Message{Role: message.RoleUser, Content: "你好"}))
	recorder.RecordEvent(eventlog.Event{Type: eventlog.EventAssistantText, AgentName: "coordinator", Content: "你好！", Sequence: 42})
	recorder.RecordEvent(eventlog.Event{Type: runtimeevents.EventAssistantCompleted, AgentName: "coordinator", Content: "你好！", Sequence: 43})
	recorder.RecordEvent(runtimeevents.UserMessage("run-2", runtimeevents.TurnID("run-2", 1), "run-2:user", message.Message{Role: message.RoleUser, Content: "你是谁"}))
	recorder.RecordEvent(eventlog.Event{Type: eventlog.EventAssistantText, AgentName: "coordinator", Content: "我是协调者", Sequence: 84})
	recorder.RecordEvent(eventlog.Event{Type: runtimeevents.EventAssistantCompleted, AgentName: "coordinator", Content: "我是协调者", Sequence: 85})
	if err := recorder.SaveToFile(filepath.Join(sessionDir, eventlog.TranscriptFileName)); err != nil {
		t.Fatalf("save history: %v", err)
	}

	router := gin.New()
	router.GET("/sessions/:sessionID", rt.GetSessionHandler())

	req := httptest.NewRequest(http.MethodGet, "/sessions/"+sessionID, nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var got Response
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	data, ok := got.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected response data object, got %#v", got.Data)
	}
	gotEvents, ok := data["events"].([]any)
	if !ok {
		t.Fatalf("expected events array, got %#v", data["events"])
	}
	wantTypes := []string{
		string(runtimeevents.EventUserMessage),
		string(runtimeevents.EventAssistantCompleted),
		string(runtimeevents.EventUserMessage),
		string(runtimeevents.EventAssistantCompleted),
	}
	wantContents := []string{"你好", "你好！", "你是谁", "我是协调者"}
	if len(gotEvents) != len(wantTypes) {
		t.Fatalf("expected %d events, got %#v", len(wantTypes), gotEvents)
	}
	for i, raw := range gotEvents {
		event, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("event %d should be object, got %#v", i, raw)
		}
		if event["type"] != wantTypes[i] {
			t.Fatalf("event %d type = %#v, want %q", i, event["type"], wantTypes[i])
		}
		if event["content"] != wantContents[i] {
			t.Fatalf("event %d content = %#v, want %q", i, event["content"], wantContents[i])
		}
		if _, ok := event["sequence"]; !ok {
			t.Fatalf("event %d missing sequence: %#v", i, event)
		}
		if _, ok := event["stream_event_id"]; ok {
			t.Fatalf("event %d should not expose stream_event_id in history: %#v", i, event)
		}
	}
}

func TestGetSessionIncludesSubagentTranscriptEvents(t *testing.T) {
	rt := newTestRuntime(t)
	gin.SetMode(gin.TestMode)

	sessionID := "subagent-session"
	sessionDir := rt.sessionDirPath(sessionID)
	if err := eventlog.SaveMetadata(sessionDir, &eventlog.SessionMetadata{
		ID:           sessionID,
		Title:        "subagent",
		Status:       "idle",
		CurrentAgent: "coordinator",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}); err != nil {
		t.Fatalf("save metadata: %v", err)
	}
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	recorder := eventlog.NewHistoryRecorder()
	recorder.SetSessionDir(sessionDir)
	recorder.RecordEvent(eventlog.Event{
		Type:      runtimeevents.EventUserMessage,
		CreatedAt: now,
		Content:   "查资料",
	})
	recorder.RecordEvent(eventlog.Event{
		Type:       eventlog.EventToolCallStarted,
		CreatedAt:  now.Add(time.Second),
		AgentName:  "coordinator",
		ToolCallID: "call_1",
		ToolName:   "ask_fkagent_researcher",
		ToolArgs:   `{"task":"查资料"}`,
	})
	recorder.RecordEvent(eventlog.Event{
		Type:           runtimeevents.EventAssistantCompleted,
		CreatedAt:      now.Add(2 * time.Second),
		AgentName:      "researcher",
		Content:        "子智能体结果",
		MemberCallID:   "call_1",
		MemberToolName: "ask_fkagent_researcher",
		MemberName:     "researcher",
	})
	recorder.RecordEvent(eventlog.Event{
		Type:       eventlog.EventToolCallCompleted,
		CreatedAt:  now.Add(3 * time.Second),
		AgentName:  "coordinator",
		ToolCallID: "call_1",
		ToolName:   "ask_fkagent_researcher",
		Content:    "子智能体结果",
	})
	if err := recorder.SaveToFile(filepath.Join(sessionDir, eventlog.TranscriptFileName)); err != nil {
		t.Fatalf("save history: %v", err)
	}

	router := gin.New()
	router.GET("/sessions/:sessionID", rt.GetSessionHandler())

	req := httptest.NewRequest(http.MethodGet, "/sessions/"+sessionID, nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var got Response
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	data, ok := got.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected response data object, got %#v", got.Data)
	}
	gotEvents, ok := data["events"].([]any)
	if !ok {
		t.Fatalf("expected events array, got %#v", data["events"])
	}
	var memberEvent map[string]any
	for _, raw := range gotEvents {
		event, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("event should be object, got %#v", raw)
		}
		if event["content"] == "子智能体结果" && event["member_call_id"] == "call_1" {
			memberEvent = event
			break
		}
	}
	if memberEvent == nil {
		t.Fatalf("missing subagent member event: %#v", gotEvents)
	}
	if memberEvent["type"] != string(runtimeevents.EventAssistantCompleted) {
		t.Fatalf("member event type = %#v, want assistant_completed", memberEvent["type"])
	}
	if memberEvent["is_member_event"] != true {
		t.Fatalf("member event should be marked as member event: %#v", memberEvent)
	}
	if memberEvent["member_tool_name"] != "ask_fkagent_researcher" || memberEvent["member_name"] != "researcher" {
		t.Fatalf("member metadata mismatch: %#v", memberEvent)
	}
	if memberEvent["parent_tool_call_id"] != "call_1" {
		t.Fatalf("parent tool call id = %#v, want call_1", memberEvent["parent_tool_call_id"])
	}
}

func TestTranscriptToChatEventsUsesAppendOrder(t *testing.T) {
	rt := newTestRuntime(t)
	transcript := []eventlog.TranscriptEvent{
		{
			ID:    "tool-1",
			At:    time.Now(),
			Type:  eventlog.TranscriptToolCallStart,
			Agent: "coordinator",
			Name:  "ask_fkagent_researcher",
		},
		{
			ID:      "msg-1",
			At:      time.Now(),
			Type:    eventlog.TranscriptAssistantMessage,
			Agent:   "coordinator",
			Content: "最终回复",
		},
	}

	got := rt.transcriptToChatEvents("session-1", transcript)
	wantTypes := []string{
		string(runtimeevents.EventToolCallStarted),
		string(runtimeevents.EventAssistantCompleted),
	}
	wantContents := []string{"", "最终回复"}
	if len(got) != len(wantTypes) {
		t.Fatalf("expected %d events, got %#v", len(wantTypes), got)
	}
	for i := range got {
		if fmt.Sprint(got[i]["type"]) != wantTypes[i] {
			t.Fatalf("event %d type = %#v, want %q", i, got[i]["type"], wantTypes[i])
		}
		content := ""
		if raw, ok := got[i]["content"]; ok {
			content, _ = raw.(string)
		}
		if content != wantContents[i] {
			t.Fatalf("event %d content = %q, want %q", i, content, wantContents[i])
		}
		if got[i]["sequence"] != int64(i+1) {
			t.Fatalf("event %d sequence = %#v, want %d", i, got[i]["sequence"], i+1)
		}
	}
}

func TestTranscriptToChatEventsProjectsCancellationInAppendOrder(t *testing.T) {
	rt := newTestRuntime(t)
	transcript := []eventlog.TranscriptEvent{
		{
			ID:      "msg-1",
			At:      time.Now(),
			Type:    eventlog.TranscriptUserMessage,
			Content: "任务",
		},
		{
			ID:      "cancel-1",
			At:      time.Now(),
			Type:    eventlog.TranscriptCancelled,
			Agent:   "system",
			Content: "任务已取消",
		},
	}

	got := rt.transcriptToChatEvents("session-1", transcript)
	wantTypes := []string{
		string(runtimeevents.EventUserMessage),
		string(runtimeevents.EventCancelled),
	}
	if len(got) != len(wantTypes) {
		t.Fatalf("expected %d events, got %#v", len(wantTypes), got)
	}
	for i, want := range wantTypes {
		if fmt.Sprint(got[i]["type"]) != want {
			t.Fatalf("event %d type = %#v, want %q; events=%#v", i, got[i]["type"], want, got)
		}
		if got[i]["sequence"] != int64(i+1) {
			t.Fatalf("event %d sequence = %#v, want %d", i, got[i]["sequence"], i+1)
		}
	}
}

func TestGetSessionReturnsNotFoundWhenSessionMissing(t *testing.T) {
	rt := newTestRuntime(t)
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/sessions/:sessionID", rt.GetSessionHandler())

	req := httptest.NewRequest(http.MethodGet, "/sessions/missing-session", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", resp.Code, resp.Body.String())
	}
}

func newTestRuntime(t *testing.T) *Runtime {
	t.Helper()

	return NewRuntime(RuntimeOptions{HistoryDir: filepath.Join(t.TempDir(), "sessions")})
}
