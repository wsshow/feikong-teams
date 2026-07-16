package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fkteams/internal/app/chat/taskstream"

	"github.com/gin-gonic/gin"
)

func TestStreamSnapshotReturnsTailByDefault(t *testing.T) {
	rt := newTestRuntime(t)
	gin.SetMode(gin.TestMode)

	stream := rt.Streams.Register(taskstream.StreamConfig{SessionID: "session-1", Cancel: func() {}})
	for _, content := range []string{"event-0", "event-1", "event-2", "event-3", "event-4"} {
		stream.Publish(taskstream.Event{"type": "system_notice", "content": content})
	}

	router := gin.New()
	router.GET("/stream/snapshot/:sessionID", rt.StreamSnapshotHandler())

	resp := performRequest(router, http.MethodGet, "/stream/snapshot/session-1?limit=2", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d: %s", resp.Code, resp.Body.String())
	}
	var got streamSnapshotResponse
	decodeRawData(t, resp, &got)
	if got.EventCount != 5 || got.SnapshotOffset != 3 || got.NextOffset != 5 || got.MoreAvailable {
		t.Fatalf("unexpected snapshot metadata: %#v", got)
	}
	if len(got.Events) != 2 || got.Events[0]["content"] != "event-3" || got.Events[1]["content"] != "event-4" {
		t.Fatalf("unexpected snapshot events: %#v", got.Events)
	}
}

func TestStreamSnapshotReturnsPageFromOffset(t *testing.T) {
	rt := newTestRuntime(t)
	gin.SetMode(gin.TestMode)

	stream := rt.Streams.Register(taskstream.StreamConfig{SessionID: "session-1", Cancel: func() {}})
	for _, content := range []string{"event-0", "event-1", "event-2", "event-3", "event-4"} {
		stream.Publish(taskstream.Event{"type": "system_notice", "content": content})
	}

	router := gin.New()
	router.GET("/stream/snapshot/:sessionID", rt.StreamSnapshotHandler())

	resp := performRequest(router, http.MethodGet, "/stream/snapshot/session-1?offset=1&limit=2", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d: %s", resp.Code, resp.Body.String())
	}
	var got streamSnapshotResponse
	decodeRawData(t, resp, &got)
	if got.EventCount != 5 || got.SnapshotOffset != 1 || got.NextOffset != 3 || !got.MoreAvailable {
		t.Fatalf("unexpected snapshot metadata: %#v", got)
	}
	if len(got.Events) != 2 || got.Events[0]["content"] != "event-1" || got.Events[1]["content"] != "event-2" {
		t.Fatalf("unexpected snapshot events: %#v", got.Events)
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
	var got streamSnapshotResponse
	decodeRawData(t, snapshot, &got)
	if !got.ReplayTruncated || got.EarliestOffset != 1 || got.EventCount != 3 || got.RetainedCount != 2 {
		t.Fatalf("snapshot metadata = %#v", got)
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
		"/stream/snapshot/session-1?offset=invalid",
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

type streamSnapshotResponse struct {
	EventCount      int              `json:"event_count"`
	RetainedCount   int              `json:"retained_count"`
	EarliestOffset  uint64           `json:"earliest_offset"`
	ReplayTruncated bool             `json:"replay_truncated"`
	NextOffset      uint64           `json:"next_offset"`
	SnapshotOffset  uint64           `json:"snapshot_offset"`
	MoreAvailable   bool             `json:"more_available"`
	Events          []map[string]any `json:"events"`
}
