package handler

import (
	eventlog "fkteams/internal/adapters/storage/file/history"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fkteams/internal/app/chat/presentation"
	"fkteams/internal/app/chat/taskstream"
	"fkteams/internal/runtime/events"

	"github.com/gin-gonic/gin"
)

func TestStreamSnapshotFallsBackToCommittedHistoryAfterTaskExpires(t *testing.T) {
	rt := newTestRuntime(t)
	const sessionID = "expired-task"
	recorder := eventlog.NewHistoryRecorder()
	recorder.SetSessionDir(rt.sessionDirPath(sessionID))
	stream := rt.Streams.Register(taskstream.StreamConfig{SessionID: sessionID, Cancel: func() {}})
	e := events.Event{Type: events.EventAssistantCompleted, MessageID: "answer", Content: "committed answer"}
	if err := recorder.RecordAndEmit(e, func(e events.Event) error {
		stream.PublishRuntime(e, standardEventPayload(sessionID, e, rt.ToolDisplays))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := recorder.Flush(); err != nil {
		t.Fatal(err)
	}
	want, _ := stream.PresentationSnapshot()
	stream.SetStatus("completed")
	stream.Done()
	rt.Streams.Remove(sessionID)

	router := gin.New()
	router.GET("/stream/snapshot/:sessionID", rt.StreamSnapshotHandler())
	response := performRequest(router, http.MethodGet, "/stream/snapshot/"+sessionID, nil)
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	var got struct {
		View       presentation.Snapshot `json:"view"`
		NextOffset uint64                `json:"next_offset"`
		HasTask    bool                  `json:"has_task"`
	}
	decodeRawData(t, response, &got)
	if got.HasTask || got.NextOffset != 0 || len(got.View.Items) != len(want.Items) || got.View.Items[0].Node.Message.Content != "committed answer" {
		t.Fatalf("durable snapshot=%#v", got)
	}
}

func TestStreamSnapshotIsCompleteAfterReplayWindowExpires(t *testing.T) {
	rt := newTestRuntime(t)
	stream := rt.Streams.Register(taskstream.StreamConfig{SessionID: "session-1", Cancel: func() {}, MaxRetainedEvents: 1})
	for i, content := range []string{"first", "second", "third"} {
		e := events.Event{Type: events.EventUserMessage, EventID: content, MessageID: content, Sequence: int64(i + 1), Content: content}
		stream.PublishRuntime(e, standardEventPayload("session-1", e, nil))
	}
	router := gin.New()
	router.GET("/stream/snapshot/:sessionID", rt.StreamSnapshotHandler())
	response := performRequest(router, http.MethodGet, "/stream/snapshot/session-1", nil)
	var got struct {
		View       presentation.Snapshot `json:"view"`
		NextOffset uint64                `json:"next_offset"`
	}
	decodeRawData(t, response, &got)
	if len(got.View.Items) != 3 || got.NextOffset != 3 {
		t.Fatalf("snapshot=%#v", got)
	}
	e := events.Event{Type: events.EventUserMessage, EventID: "fourth", MessageID: "fourth", Sequence: 4, Content: "fourth"}
	stream.PublishRuntime(e, standardEventPayload("session-1", e, nil))
	page := stream.EventsPage(got.NextOffset, 10)
	if len(page.Events) != 1 || page.Events[0].ID != got.NextOffset {
		t.Fatalf("snapshot/subscribe boundary=%#v", page)
	}
}

func TestCompletedStreamSubscribeFromZeroReplaysEventsBeforeDone(t *testing.T) {
	rt := newTestRuntime(t)
	gin.SetMode(gin.TestMode)

	stream := rt.Streams.Register(taskstream.StreamConfig{SessionID: "session-1", Cancel: func() {}})
	stream.Publish(taskstream.Event{"type": "system_notice", "content": "event-0"})
	stream.SetStatus("completed")
	stream.Done()

	router := gin.New()
	router.GET("/stream/subscribe/:sessionID", rt.StreamSubscribeHandler())

	resp := performRequest(router, http.MethodGet, "/stream/subscribe/session-1?offset=0", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("subscribe status = %d: %s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	eventIndex := strings.Index(body, "event-0")
	doneIndex := strings.Index(body, "data: [DONE]")
	if eventIndex < 0 || doneIndex < 0 || eventIndex > doneIndex {
		t.Fatalf("completed zero-offset subscribe should replay events before done, got %q", body)
	}
}

func TestStreamEndpointsReportExpiredReplayWindow(t *testing.T) {
	rt := newTestRuntime(t)
	gin.SetMode(gin.TestMode)
	stream := rt.Streams.Register(taskstream.StreamConfig{
		SessionID:             "session-1",
		Cancel:                func() {},
		MaxRetainedEvents:     2,
		MaxRetainedEventBytes: 1 << 20,
	})
	for i := 0; i < 3; i++ {
		stream.Publish(taskstream.Event{"type": "system_notice", "index": i})
	}

	router := gin.New()
	router.GET("/stream/snapshot/:sessionID", rt.StreamSnapshotHandler())
	router.GET("/stream/subscribe/:sessionID", rt.StreamSubscribeHandler())

	snapshot := performRequest(router, http.MethodGet, "/stream/snapshot/session-1?offset=0&limit=10", nil)
	if snapshot.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d: %s", snapshot.Code, snapshot.Body.String())
	}
	var got struct {
		NextOffset uint64 `json:"next_offset"`
	}
	decodeRawData(t, snapshot, &got)
	if got.NextOffset != 3 {
		t.Fatalf("cursor=%d", got.NextOffset)
	}

	subscribe := performRequest(router, http.MethodGet, "/stream/subscribe/session-1?offset=0", nil)
	if subscribe.Code != http.StatusConflict {
		t.Fatalf("subscribe status = %d, want 409: %s", subscribe.Code, subscribe.Body.String())
	}
}

func TestStreamEndpointsRejectInvalidOffsets(t *testing.T) {
	rt := newTestRuntime(t)
	gin.SetMode(gin.TestMode)
	rt.Streams.Register(taskstream.StreamConfig{SessionID: "session-1", Cancel: func() {}})
	router := gin.New()
	router.GET("/stream/snapshot/:sessionID", rt.StreamSnapshotHandler())
	router.GET("/stream/events/:sessionID", rt.StreamEventsHandler())
	router.GET("/stream/subscribe/:sessionID", rt.StreamSubscribeHandler())

	for _, path := range []string{
		"/stream/events/session-1?offset=invalid",
		"/stream/subscribe/session-1?offset=invalid",
	} {
		resp := performRequest(router, http.MethodGet, path, nil)
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want 400: %s", path, resp.Code, resp.Body.String())
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/stream/subscribe/session-1", nil)
	request.Header.Set("Last-Event-ID", "18446744073709551615")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, request)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("overflowing Last-Event-ID status = %d, want 400: %s", resp.Code, resp.Body.String())
	}
}

func TestStreamEventsHandlerPaginatesBufferedEvents(t *testing.T) {
	rt := newTestRuntime(t)
	gin.SetMode(gin.TestMode)
	stream := rt.Streams.Register(taskstream.StreamConfig{SessionID: "session-1", Cancel: func() {}})
	for i := 0; i < 5; i++ {
		stream.Publish(taskstream.Event{"type": "system_notice", "index": i})
	}
	router := gin.New()
	router.GET("/stream/events/:sessionID", rt.StreamEventsHandler())

	resp := performRequest(router, http.MethodGet, "/stream/events/session-1?offset=1&limit=2", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("events status = %d: %s", resp.Code, resp.Body.String())
	}
	var got struct {
		EventCount    int                       `json:"event_count"`
		NextOffset    uint64                    `json:"next_offset"`
		MoreAvailable bool                      `json:"more_available"`
		Limit         int                       `json:"limit"`
		Events        []taskstream.IndexedEvent `json:"events"`
	}
	decodeRawData(t, resp, &got)
	if got.EventCount != 5 || got.NextOffset != 3 || !got.MoreAvailable || got.Limit != 2 {
		t.Fatalf("events metadata = %#v", got)
	}
	if len(got.Events) != 2 || got.Events[0].ID != 1 || got.Events[1].ID != 2 {
		t.Fatalf("events page = %#v", got.Events)
	}
}
