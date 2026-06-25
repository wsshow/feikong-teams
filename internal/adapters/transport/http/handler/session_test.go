package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"fkteams/internal/adapters/storage/file/history"

	"github.com/gin-gonic/gin"
)

func TestGetSessionReturnsEmptyMessagesWhenHistoryFileMissing(t *testing.T) {
	withSessionHistoryDir(t)
	gin.SetMode(gin.TestMode)

	sessionID := "empty-session"
	if err := eventlog.SaveMetadata(sessionDirPath(sessionID), &eventlog.SessionMetadata{
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
	router.GET("/sessions/:sessionID", GetSessionHandler())

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
	messages, ok := data["messages"].([]any)
	if !ok {
		t.Fatalf("expected messages array, got %#v", data["messages"])
	}
	if len(messages) != 0 {
		t.Fatalf("expected empty messages, got %#v", messages)
	}
}

func TestGetSessionReturnsNotFoundWhenSessionMissing(t *testing.T) {
	withSessionHistoryDir(t)
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/sessions/:sessionID", GetSessionHandler())

	req := httptest.NewRequest(http.MethodGet, "/sessions/missing-session", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", resp.Code, resp.Body.String())
	}
}

func withSessionHistoryDir(t *testing.T) {
	t.Helper()

	oldHistoryDir := historyDir
	oldStreams := GlobalStreams
	historyDir = filepath.Join(t.TempDir(), "sessions")
	GlobalStreams = newGlobalStreams()
	t.Cleanup(func() {
		historyDir = oldHistoryDir
		GlobalStreams = oldStreams
	})
}
