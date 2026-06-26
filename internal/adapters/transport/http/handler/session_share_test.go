package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"fkteams/internal/adapters/storage/file/history"
	"fkteams/internal/domain/message"
	"fkteams/internal/runtime/env"

	"github.com/gin-gonic/gin"
)

func TestSessionSharesFilePathUsesAppDir(t *testing.T) {
	appDir := t.TempDir()
	t.Setenv(env.AppDir, appDir)

	want := filepath.Join(appDir, "share", sessionShareFileName)
	if got := sessionSharesFilePath(); got != want {
		t.Fatalf("unexpected session share file path: got %q, want %q", got, want)
	}
}

func TestPublicSessionShareRequiresPassword(t *testing.T) {
	rt := newSessionShareTestRuntime(t, map[string]*sessionShareEntry{})
	gin.SetMode(gin.TestMode)

	sessionID := "shared-session"
	writeShareableSession(t, rt, sessionID, "Shared session")
	rt.SessionShares.Lock()
	rt.SessionShares.m["share-1"] = &sessionShareEntry{
		SessionID:    sessionID,
		Title:        "Shared session",
		PasswordHash: hashPassword("secret"),
		MessageCount: 1,
		CreatedAt:    time.Now(),
	}
	rt.SessionShares.Unlock()

	router := gin.New()
	router.POST("/public/session-shares/:shareID/access", rt.AccessPublicSessionShareHandler())

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
	rt := newSessionShareTestRuntime(t, map[string]*sessionShareEntry{})
	gin.SetMode(gin.TestMode)

	rt.SessionShares.Lock()
	rt.SessionShares.m["expired"] = &sessionShareEntry{
		SessionID: "old-session",
		Title:     "Old session",
		ExpiresAt: time.Now().Add(-time.Minute),
		CreatedAt: time.Now().Add(-time.Hour),
	}
	rt.SessionShares.Unlock()

	router := gin.New()
	router.GET("/public/session-shares/:shareID/info", rt.GetPublicSessionShareInfoHandler())

	req := httptest.NewRequest(http.MethodGet, "/public/session-shares/expired/info", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusGone {
		t.Fatalf("expected 410, got %d: %s", resp.Code, resp.Body.String())
	}
	rt.SessionShares.RLock()
	_, exists := rt.SessionShares.m["expired"]
	rt.SessionShares.RUnlock()
	if exists {
		t.Fatal("expected expired share to be removed")
	}
}

func writeShareableSession(t *testing.T, rt *Runtime, sessionID, title string) {
	t.Helper()
	now := time.Now()
	sessionDir := rt.sessionDirPath(sessionID)
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

func newSessionShareTestRuntime(t *testing.T, entries map[string]*sessionShareEntry) *Runtime {
	t.Helper()
	store := NewSessionShareStore(filepath.Join(t.TempDir(), sessionShareFileName))
	store.Lock()
	store.m = entries
	store.Unlock()
	rt := newTestRuntime(t)
	rt.SessionShares = store
	return rt
}
