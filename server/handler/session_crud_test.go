package handler

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	eventlog "fkteams/events/log"

	"github.com/gin-gonic/gin"
)

func TestSessionCRUDHandlers(t *testing.T) {
	withSessionHistoryDir(t)
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/sessions", ListSessionsHandler())
	router.POST("/sessions", CreateSessionHandler())
	router.DELETE("/sessions/:sessionID", DeleteSessionHandler())
	router.POST("/sessions/rename", RenameSessionHandler())
	router.POST("/sessions/agent", UpdateSessionAgentHandler())

	longTitle := strings.Repeat("题", 55)
	resp := performJSON(router, http.MethodPost, "/sessions", `{"session_id":"session-1","title":"`+longTitle+`"}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("create session status = %d: %s", resp.Code, resp.Body.String())
	}
	meta, err := eventlog.LoadMetadata(sessionDirPath("session-1"))
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
	meta, err = eventlog.LoadMetadata(sessionDirPath("session-1"))
	if err != nil {
		t.Fatalf("load metadata after rename: %v", err)
	}
	if meta.Title != "新标题" {
		t.Fatalf("renamed title = %q", meta.Title)
	}

	resp = performJSON(router, http.MethodPost, "/sessions/agent", `{"session_id":"session-1","current_agent":"coder"}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("update agent status = %d: %s", resp.Code, resp.Body.String())
	}
	meta, err = eventlog.LoadMetadata(sessionDirPath("session-1"))
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
	if list.Sessions[0].SessionID != "session-1" || list.Sessions[0].Title != "新标题" || list.Sessions[0].CurrentAgent != "coder" {
		t.Fatalf("listed session = %#v", list.Sessions[0])
	}

	resp = performRequest(router, http.MethodDelete, "/sessions/session-1", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("delete session status = %d: %s", resp.Code, resp.Body.String())
	}
	if _, err := os.Stat(sessionDirPath("session-1")); !os.IsNotExist(err) {
		t.Fatalf("session dir should be deleted, stat err=%v", err)
	}

	resp = performRequest(router, http.MethodDelete, "/sessions/session-1", nil)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("delete missing session status = %d, want 404", resp.Code)
	}
}

func TestSessionHandlersRejectInvalidRequests(t *testing.T) {
	withSessionHistoryDir(t)
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.POST("/sessions", CreateSessionHandler())
	router.POST("/sessions/rename", RenameSessionHandler())
	router.POST("/sessions/agent", UpdateSessionAgentHandler())
	router.DELETE("/sessions/:sessionID", DeleteSessionHandler())

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
	if err := eventlog.SaveMetadata(sessionDirPath("session-2"), &eventlog.SessionMetadata{
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
}
