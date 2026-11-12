package handler

import (
	"encoding/json"
	eventlog "fkteams/internal/adapters/storage/file/history"
	"fkteams/internal/app/chat/presentation"
	"fkteams/internal/app/chat/taskstream"
	"fkteams/internal/domain/event"
	"fkteams/internal/domain/message"
	"github.com/gin-gonic/gin"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func newTestRuntime(t *testing.T) *Runtime {
	t.Helper()
	return NewRuntime(RuntimeOptions{HistoryDir: filepath.Join(t.TempDir(), "sessions")})
}
func TestGetSessionReturnsEmptyPresentationWithoutHistory(t *testing.T) {
	rt := newTestRuntime(t)
	rt.Streams.Register(taskstream.StreamConfig{SessionID: "empty", Cancel: func() {}})
	router := gin.New()
	router.GET("/sessions/:sessionID", rt.GetSessionHandler())
	response := performRequest(router, http.MethodGet, "/sessions/empty", nil)
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	var got struct {
		View presentation.Snapshot `json:"view"`
	}
	decodeRawData(t, response, &got)
	if got.View.Items == nil || len(got.View.Items) != 0 {
		t.Fatalf("view=%#v", got)
	}
}
func TestGetSessionReturnsNotFoundWhenSessionMissing(t *testing.T) {
	rt := newTestRuntime(t)
	router := gin.New()
	router.GET("/sessions/:sessionID", rt.GetSessionHandler())
	response := performRequest(router, http.MethodGet, "/sessions/missing", nil)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d %s", response.Code, response.Body.String())
	}
}

func TestLegacySessionReturnsHelpfulFormatErrorWithoutChangingHistory(t *testing.T) {
	rt := newTestRuntime(t)
	dir := rt.sessionDirPath("legacy")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, eventlog.TranscriptFileName)
	before := []byte(`{"type":"user_message","id":"private-record-id","content":"old message"}` + "\n")
	if err := os.WriteFile(path, before, 0600); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.GET("/sessions/:sessionID", rt.GetSessionHandler())
	router.GET("/stream/snapshot/:sessionID", rt.StreamSnapshotHandler())
	for _, endpoint := range []string{"/sessions/legacy", "/stream/snapshot/legacy"} {
		response := performRequest(router, http.MethodGet, endpoint, nil)
		if response.Code != http.StatusConflict {
			t.Fatalf("status=%d: %s", response.Code, response.Body.String())
		}
		var got Response
		if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.ErrorCode != "conflict" || !strings.Contains(got.Message, "记录格式较旧") || !strings.Contains(got.Message, "请新建会话") || strings.Contains(got.Message, "private-record-id") {
			t.Fatalf("unhelpful response: %#v", got)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatalf("legacy history changed: %v", err)
	}
}
func TestConversationLiveReplayAndRestartUseIdenticalFacts(t *testing.T) {
	rt := newTestRuntime(t)
	const sessionID = "conversation"
	dir := rt.sessionDirPath(sessionID)
	recorder := eventlog.NewHistoryRecorder()
	recorder.SetSessionDir(dir)
	live := presentation.New(sessionID, rt.ToolDisplays)
	stream := rt.Streams.Register(taskstream.StreamConfig{SessionID: sessionID, Cancel: func() {}, MaxRetainedEvents: 2})
	stream.SetPresentation(live)
	at := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	counter := 0
	feed := func(e event.Event) {
		counter++
		e.EventID = stringID(counter)
		e.Sequence = int64(counter)
		e.CreatedAt = at.Add(time.Duration(counter) * time.Millisecond)
		if e.RunID == "" {
			e.RunID = "run"
		}
		e = recorder.SequenceEvent(e)
		recorder.RecordEvent(e)
		stream.PublishRuntime(e, standardEventPayload(sessionID, e, rt.ToolDisplays))
	}
	feed(event.Event{Type: event.TypeUserMessage, MessageID: "user", Content: "research"})
	feed(event.Event{Type: event.TypeAssistantStarted, MessageID: "parent", AgentName: "assistant"})
	feed(event.Event{Type: event.TypeAssistantReasoning, MessageID: "parent", AgentName: "assistant", BlockID: "thinking", Content: "think"})
	feed(event.Event{Type: event.TypeAssistantText, MessageID: "parent", AgentName: "assistant", BlockID: "plan", Content: "draft"})
	parent := message.Message{Role: message.RoleAssistant, Content: "corrected", ReasoningContent: "think", ToolCalls: []message.ToolCall{{ID: "member", Function: message.FunctionCall{Name: "ask_fkagent_researcher"}}, {ID: "search", Function: message.FunctionCall{Name: "search"}}}}
	feed(event.Event{Type: event.TypeAssistantCompleted, MessageID: "parent", AgentName: "assistant", Message: &parent})
	feed(event.Event{Type: event.TypeToolCallStarted, MessageID: "parent", ToolCallID: "member", ToolCallRef: "tool_call:member", ToolName: "ask_fkagent_researcher", ToolCall: &parent.ToolCalls[0]})
	feed(event.Event{Type: event.TypeToolCallStarted, MessageID: "parent", ToolCallID: "search", ToolCallRef: "tool_call:search", ToolName: "search", ToolCall: &parent.ToolCalls[1]})
	feed(event.Event{Type: event.TypeAssistantStarted, MessageID: "child", MemberCallID: "member", MemberName: "researcher"})
	feed(event.Event{Type: event.TypeAssistantText, MessageID: "child", MemberCallID: "member", MemberName: "researcher", BlockID: "child:text", Content: "child draft"})
	feed(event.Event{Type: event.TypeAskRequested, MemberCallID: "member", MemberName: "researcher", Ask: &event.AskPayload{ID: "question", Question: "which?", Options: []string{"one", "two"}}})
	feed(event.Event{Type: event.TypeAskAnswered, MemberCallID: "member", MemberName: "researcher", Ask: &event.AskPayload{ID: "question", Selected: []string{"one"}}})
	feed(event.Event{Type: event.TypeAssistantCompleted, MessageID: "child", MemberCallID: "member", MemberName: "researcher", Content: "child final"})
	feed(event.Event{Type: event.TypeApprovalRequested, Detail: "approval", Content: "continue?"})
	feed(event.Event{Type: event.TypeApprovalAnswered, Detail: "approval", Approval: &event.ApprovalPayload{ID: "approval", Decision: "denied"}})
	feed(event.Event{Type: event.TypeToolCallFailed, MessageID: "parent", ToolCallID: "search", ToolCallRef: "tool_call:search", ToolName: "search", Content: "failed"})
	feed(event.Event{Type: event.TypeToolCallCompleted, MessageID: "parent", ToolCallID: "member", ToolCallRef: "tool_call:member", ToolName: "ask_fkagent_researcher", ToolResult: "child final"})
	feed(event.Event{Type: event.TypeTurnCompleted})
	recorder.FinalizeCurrent()
	if err := recorder.Flush(); err != nil {
		t.Fatal(err)
	}
	records, err := eventlog.LoadSessionTranscriptRecords(dir)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := rt.transcriptPresentation(sessionID, records)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := stream.PresentationSnapshot()
	after := replayed.Snapshot()
	if !reflect.DeepEqual(before, after) {
		a, _ := json.MarshalIndent(before, "", " ")
		b, _ := json.MarshalIndent(after, "", " ")
		t.Fatalf("live=%s\nhistory=%s", a, b)
	}
	// 主干的正文校正、并行工具状态、成员交互都必须由后端明确给出。
	parentView := after.Items[1].Node
	if parentView.Parts[1].Content != "corrected" || parentView.Parts[3].Tool.Status != "error" || parentView.Parts[2].Member == nil {
		t.Fatalf("parent=%#v", parentView)
	}
	if cursor := stream.EventsPage(0, 10); !cursor.ReplayTruncated {
		t.Fatal("test did not exceed event window")
	}
	// 新记录器模拟进程重启，时钟或运行时序号回退不影响会话顺序。
	reloaded := eventlog.NewHistoryRecorder()
	if err := reloaded.LoadFromFile(filepath.Join(dir, eventlog.TranscriptFileName)); err != nil {
		t.Fatal(err)
	}
	next := reloaded.SequenceEvent(event.Event{Type: event.TypeUserMessage, EventID: "new-input", Sequence: 1, CreatedAt: at.Add(-time.Hour), RunID: "new-run", MessageID: "new-user", Content: "next"})
	reloaded.RecordEvent(next)
	if err := reloaded.Flush(); err != nil {
		t.Fatal(err)
	}
	records, err = eventlog.LoadSessionTranscriptRecords(dir)
	if err != nil {
		t.Fatal(err)
	}
	again, err := rt.transcriptPresentation(sessionID, records)
	if err != nil {
		t.Fatal(err)
	}
	items := again.Snapshot().Items
	if items[len(items)-1].ID != "new-user" {
		t.Fatal("new run moved before prior history")
	}
}
func TestPartialCancellationIsStableAfterReload(t *testing.T) {
	rt := newTestRuntime(t)
	dir := rt.sessionDirPath("cancelled")
	recorder := eventlog.NewHistoryRecorder()
	recorder.SetSessionDir(dir)
	recorder.RecordEvent(event.Event{Type: event.TypeAssistantReasoning, EventID: "thinking", MessageID: "partial", BlockID: "reason", Content: "think"})
	recorder.RecordEvent(event.Event{Type: event.TypeAssistantText, EventID: "draft", MessageID: "partial", BlockID: "text", Content: "partial answer"})
	recorder.RecordCancelled("任务已取消")
	recorder.FinalizeCurrent()
	if err := recorder.Flush(); err != nil {
		t.Fatal(err)
	}
	records, err := eventlog.LoadSessionTranscriptRecords(dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := rt.transcriptPresentation("cancelled", records)
	if err != nil {
		t.Fatal(err)
	}
	view := p.Snapshot()
	if len(view.Items) != 2 || view.Items[0].Node.Parts[0].Key != "reason" || view.Items[0].Node.Parts[1].Content != "partial answer" || view.Items[1].Node.Message.Role != message.RoleSystem {
		t.Fatalf("view=%#v", view)
	}
}
func stringID(index int) string { return time.Unix(int64(index), 0).UTC().Format(time.RFC3339Nano) }

func TestCancelledToolArgumentsRemainVisibleWithoutEnteringModelContext(t *testing.T) {
	rt := newTestRuntime(t)
	dir := rt.sessionDirPath("partial-tool")
	recorder := eventlog.NewHistoryRecorder()
	recorder.SetSessionDir(dir)
	recorder.RecordEvent(event.Event{Type: event.TypeToolCallArguments, MessageID: "draft", Role: message.RoleAssistant, ToolCallID: "call", ToolName: "execute", BlockID: "draft:tool:call", Content: `{"command":"echo`})
	recorder.RecordCancelled("cancelled")
	if err := recorder.Flush(); err != nil {
		t.Fatal(err)
	}
	for _, msg := range recorder.ProjectContextMessages(0) {
		if len(msg.ToolCalls) > 0 {
			t.Fatal("unfinished arguments entered model context")
		}
	}
	records, err := eventlog.LoadSessionTranscriptRecords(dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := rt.transcriptPresentation("partial-tool", records)
	if err != nil {
		t.Fatal(err)
	}
	tool := p.Snapshot().Items[0].Node.Parts[0].Tool
	if tool.Arguments != `{"command":"echo` || tool.Status != "cancelled" {
		t.Fatalf("tool=%#v", tool)
	}
	reloaded := eventlog.NewHistoryRecorder()
	if err := reloaded.LoadFromFile(filepath.Join(dir, eventlog.TranscriptFileName)); err != nil {
		t.Fatal(err)
	}
	for _, msg := range reloaded.ProjectContextMessages(0) {
		if len(msg.ToolCalls) > 0 {
			t.Fatal("reload restored unfinished arguments into context")
		}
	}
}

func TestLongBusinessToolFailureKeepsStatusAfterReload(t *testing.T) {
	rt := newTestRuntime(t)
	const sessionID = "business-failure"
	dir := rt.sessionDirPath(sessionID)
	recorder := eventlog.NewHistoryRecorder()
	recorder.SetSessionDir(dir)
	result := `{"success":false,"error_message":"` + strings.Repeat("failed operation; ", 1000) + `"}`
	e := event.Event{Type: event.TypeToolCallCompleted, MessageID: "owner", ToolCallID: "call", ToolName: "execute", ToolResult: result}
	if err := recorder.RecordAndEmit(e, nil); err != nil {
		t.Fatal(err)
	}
	if err := recorder.Flush(); err != nil {
		t.Fatal(err)
	}
	records, err := eventlog.LoadSessionTranscriptRecords(dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := rt.transcriptPresentation(sessionID, records)
	if err != nil {
		t.Fatal(err)
	}
	tool := p.Snapshot().Items[0].Node.Parts[0].Tool
	if tool.Result == result {
		t.Fatal("test result did not cross the artifact summary boundary")
	}
	if tool.Status != "error" {
		t.Fatalf("summarized failure was reported as %s", tool.Status)
	}
}
