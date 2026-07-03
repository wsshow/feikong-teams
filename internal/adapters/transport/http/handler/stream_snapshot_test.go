package handler

import (
	"net/http"
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

func TestCompletedStreamSubscribeFromZeroReturnsDoneOnly(t *testing.T) {
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
	if !strings.Contains(body, "data: [DONE]") || strings.Contains(body, "event-0") {
		t.Fatalf("completed zero-offset subscribe should only return done frame, got %q", body)
	}
}

type streamSnapshotResponse struct {
	EventCount     int              `json:"event_count"`
	NextOffset     uint64           `json:"next_offset"`
	SnapshotOffset uint64           `json:"snapshot_offset"`
	MoreAvailable  bool             `json:"more_available"`
	Events         []map[string]any `json:"events"`
}
