package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"fkteams/fkenv"
	"fkteams/internal/adapters/storage/file/history"
	"fkteams/internal/domain/message"

	"github.com/gin-gonic/gin"
)

func TestSessionSharesFilePathUsesAppDir(t *testing.T) {
	appDir := t.TempDir()
	t.Setenv(fkenv.AppDir, appDir)

	want := filepath.Join(appDir, "share", sessionShareFileName)
	if got := sessionSharesFilePath(); got != want {
		t.Fatalf("unexpected session share file path: got %q, want %q", got, want)
	}
}

func TestPublicSessionShareRequiresPassword(t *testing.T) {
	withSessionHistoryDir(t)
	withSessionShareStore(t, map[string]*sessionShareEntry{})
	gin.SetMode(gin.TestMode)

	sessionID := "shared-session"
	writeShareableSession(t, sessionID, "Shared session")
	sessionShareStore.Lock()
	sessionShareStore.m["share-1"] = &sessionShareEntry{
		SessionID:    sessionID,
		Title:        "Shared session",
		PasswordHash: hashPassword("secret"),
		MessageCount: 1,
		CreatedAt:    time.Now(),
	}
	sessionShareStore.Unlock()

	router := gin.New()
	router.POST("/public/session-shares/:shareID/access", AccessPublicSessionShareHandler())

	req := httptest.NewRequest(http.MethodPost, "/public/session-shares/share-1/access", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", resp.Code, resp.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/public/session-shares/share-1/access", bytes.NewBufferString(`{"password":"secret"}`))
	req.Header.Set("Content-Type", "application/json")
	resp = httptest.NewRecorder()
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
	messages, ok := data["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("expected one shared message, got %#v", data["messages"])
	}
}

func TestExpiredSessionShareReturnsGone(t *testing.T) {
	withSessionShareStore(t, map[string]*sessionShareEntry{})
	gin.SetMode(gin.TestMode)

	sessionShareStore.Lock()
	sessionShareStore.m["expired"] = &sessionShareEntry{
		SessionID: "old-session",
		Title:     "Old session",
		ExpiresAt: time.Now().Add(-time.Minute),
		CreatedAt: time.Now().Add(-time.Hour),
	}
	sessionShareStore.Unlock()

	router := gin.New()
	router.GET("/public/session-shares/:shareID/info", GetPublicSessionShareInfoHandler())

	req := httptest.NewRequest(http.MethodGet, "/public/session-shares/expired/info", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusGone {
		t.Fatalf("expected 410, got %d: %s", resp.Code, resp.Body.String())
	}
	sessionShareStore.RLock()
	_, exists := sessionShareStore.m["expired"]
	sessionShareStore.RUnlock()
	if exists {
		t.Fatal("expected expired share to be removed")
	}
}

func writeShareableSession(t *testing.T, sessionID, title string) {
	t.Helper()
	now := time.Now()
	sessionDir := sessionDirPath(sessionID)
	if err := eventlog.SaveMetadata(sessionDir, &eventlog.SessionMetadata{
		ID:        sessionID,
		Title:     title,
		Status:    "completed",
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("save metadata: %v", err)
	}
	recorder := eventlog.NewHistoryRecorder()
	recorder.RecordUserMessage(message.Message{Role: message.RoleUser, Content: "hello"})
	if err := recorder.SaveToFile(filepath.Join(sessionDir, eventlog.HistoryFileName)); err != nil {
		t.Fatalf("save history: %v", err)
	}
}

func withSessionShareStore(t *testing.T, store map[string]*sessionShareEntry) {
	t.Helper()

	sessionShareStore.Lock()
	old := sessionShareStore.m
	sessionShareStore.m = store
	sessionShareStore.Unlock()

	t.Cleanup(func() {
		sessionShareStore.Lock()
		sessionShareStore.m = old
		sessionShareStore.Unlock()
	})
}
