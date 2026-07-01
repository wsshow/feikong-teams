package handler

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	eventlog "fkteams/internal/adapters/storage/file/history"
	"fkteams/internal/app/chat/taskstream"

	"github.com/gin-gonic/gin"
)

func TestSessionCRUDHandlers(t *testing.T) {
	rt := newTestRuntime(t)
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/sessions", rt.ListSessionsHandler())
	router.POST("/sessions", rt.CreateSessionHandler())
	router.DELETE("/sessions/:sessionID", rt.DeleteSessionHandler())
	router.POST("/sessions/rename", rt.RenameSessionHandler())
	router.POST("/sessions/favorite", rt.FavoriteSessionHandler())
	router.POST("/sessions/agent", rt.UpdateSessionAgentHandler())

	longTitle := strings.Repeat("题", 55)
	resp := performJSON(router, http.MethodPost, "/sessions", `{"session_id":"session-1","title":"`+longTitle+`"}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("create session status = %d: %s", resp.Code, resp.Body.String())
	}
	meta, err := eventlog.LoadMetadata(rt.sessionDirPath("session-1"))
	if err != nil {
		t.Fatalf("load metadata after create: %v", err)
	}
	if meta.Title != strings.Repeat("题", 50)+"..." {
		t.Fatalf("created title = %q", meta.Title)
	}

	resp = performJSON(router, http.MethodPost, "/sessions", `{"session_id":"session-1","title":"ignored"}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("create existing session status = %d: %s", resp.Code, resp.Body.String())
	}

	resp = performJSON(router, http.MethodPost, "/sessions/rename", `{"session_id":"session-1","title":"新标题"}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("rename session status = %d: %s", resp.Code, resp.Body.String())
	}
	meta, err = eventlog.LoadMetadata(rt.sessionDirPath("session-1"))
	if err != nil {
		t.Fatalf("load metadata after rename: %v", err)
	}
	if meta.Title != "新标题" {
		t.Fatalf("renamed title = %q", meta.Title)
	}

	resp = performJSON(router, http.MethodPost, "/sessions/favorite", `{"session_id":"session-1","favorite":true}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("favorite session status = %d: %s", resp.Code, resp.Body.String())
	}
	meta, err = eventlog.LoadMetadata(rt.sessionDirPath("session-1"))
	if err != nil {
		t.Fatalf("load metadata after favorite update: %v", err)
	}
	if !meta.Favorite {
		t.Fatal("session should be favorite")
	}

	resp = performJSON(router, http.MethodPost, "/sessions/agent", `{"session_id":"session-1","current_agent":"coder"}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("update agent status = %d: %s", resp.Code, resp.Body.String())
	}
	meta, err = eventlog.LoadMetadata(rt.sessionDirPath("session-1"))
	if err != nil {
		t.Fatalf("load metadata after agent update: %v", err)
	}
	if meta.CurrentAgent != "coder" {
		t.Fatalf("current agent = %q", meta.CurrentAgent)
	}

	resp = performRequest(router, http.MethodGet, "/sessions", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("list sessions status = %d: %s", resp.Code, resp.Body.String())
	}
	var list struct {
		Sessions []SessionInfo `json:"sessions"`
	}
	decodeRawData(t, resp, &list)
	if len(list.Sessions) != 1 {
		t.Fatalf("sessions = %#v", list.Sessions)
	}
	if list.Sessions[0].SessionID != "session-1" || list.Sessions[0].Title != "新标题" || list.Sessions[0].CurrentAgent != "coder" || !list.Sessions[0].Favorite {
		t.Fatalf("listed session = %#v", list.Sessions[0])
	}

	stream := rt.Streams.Register(taskstream.StreamConfig{
		SessionID: "session-1",
		Cancel:    func() {},
	})
	stream.SetStatus("completed")
	resp = performRequest(router, http.MethodGet, "/sessions", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("list sessions with completed stream status = %d: %s", resp.Code, resp.Body.String())
	}
	decodeRawData(t, resp, &list)
	if list.Sessions[0].ActiveTask {
		t.Fatalf("completed stream should not mark session active: %#v", list.Sessions[0])
	}
	stream.SetStatus("processing")
	resp = performRequest(router, http.MethodGet, "/sessions", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("list sessions with processing stream status = %d: %s", resp.Code, resp.Body.String())
	}
	decodeRawData(t, resp, &list)
	if !list.Sessions[0].ActiveTask || list.Sessions[0].Status != "processing" {
		t.Fatalf("processing stream should mark session active: %#v", list.Sessions[0])
	}

	resp = performRequest(router, http.MethodDelete, "/sessions/session-1", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("delete session status = %d: %s", resp.Code, resp.Body.String())
	}
	if _, err := os.Stat(rt.sessionDirPath("session-1")); !os.IsNotExist(err) {
		t.Fatalf("session dir should be deleted, stat err=%v", err)
	}

	resp = performRequest(router, http.MethodDelete, "/sessions/session-1", nil)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("delete missing session status = %d, want 404", resp.Code)
	}
}

func TestSessionHandlersRejectInvalidRequests(t *testing.T) {
	rt := newTestRuntime(t)
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.POST("/sessions", rt.CreateSessionHandler())
	router.POST("/sessions/rename", rt.RenameSessionHandler())
	router.POST("/sessions/favorite", rt.FavoriteSessionHandler())
	router.POST("/sessions/agent", rt.UpdateSessionAgentHandler())
	router.DELETE("/sessions/:sessionID", rt.DeleteSessionHandler())

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "create bad body", method: http.MethodPost, path: "/sessions", body: `{bad json`},
		{name: "create bad id", method: http.MethodPost, path: "/sessions", body: `{"session_id":"../bad"}`},
		{name: "rename bad body", method: http.MethodPost, path: "/sessions/rename", body: `{}`},
		{name: "rename bad id", method: http.MethodPost, path: "/sessions/rename", body: `{"session_id":"../bad","title":"x"}`},
		{name: "favorite bad body", method: http.MethodPost, path: "/sessions/favorite", body: `{}`},
		{name: "favorite bad id", method: http.MethodPost, path: "/sessions/favorite", body: `{"session_id":"../bad","favorite":true}`},
		{name: "agent bad body", method: http.MethodPost, path: "/sessions/agent", body: `{}`},
		{name: "agent bad id", method: http.MethodPost, path: "/sessions/agent", body: `{"session_id":"../bad"}`},
		{name: "delete bad id", method: http.MethodDelete, path: "/sessions/bad%5Cid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var respCode int
			if tt.body == "" {
				respCode = performRequest(router, tt.method, tt.path, nil).Code
			} else {
				respCode = performJSON(router, tt.method, tt.path, tt.body).Code
			}
			if respCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", respCode)
			}
		})
	}

	now := time.Now()
	if err := eventlog.SaveMetadata(rt.sessionDirPath("session-2"), &eventlog.SessionMetadata{
		ID:        "session-2",
		Title:     "title",
		Status:    "idle",
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("save metadata: %v", err)
	}
	resp := performJSON(router, http.MethodPost, "/sessions/rename", `{"session_id":"missing","title":"x"}`)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("rename missing status = %d, want 404", resp.Code)
	}
	resp = performJSON(router, http.MethodPost, "/sessions/agent", `{"session_id":"missing","current_agent":"coder"}`)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("agent missing status = %d, want 404", resp.Code)
	}
	resp = performJSON(router, http.MethodPost, "/sessions/favorite", `{"session_id":"missing","favorite":true}`)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("favorite missing status = %d, want 404", resp.Code)
	}
}
