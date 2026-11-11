package handler

import (
	"bytes"
	"encoding/json"
	sharestore "fkteams/internal/adapters/storage/file/share"
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

func TestPublicSessionShareRequiresPassword(t *testing.T) {
	rt := newSessionShareTestRuntime(t, map[string]*sharestore.SessionEntry{})
	gin.SetMode(gin.TestMode)

	sessionID := "shared-session"
	writeShareableSession(t, rt, sessionID, "Shared session")
	if err := rt.SessionShares.Put("share-1", &sharestore.SessionEntry{
		SessionID:    sessionID,
		Title:        "Shared session",
		PasswordHash: hashPassword("secret"),
		MessageCount: 1,
		CreatedAt:    time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

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
	events, ok := data["events"].([]any)
	if !ok || len(events) != 1 {
		t.Fatalf("expected one shared event, got %#v", data["events"])
	}
}

func TestExpiredSessionShareReturnsGone(t *testing.T) {
	rt := newSessionShareTestRuntime(t, map[string]*sharestore.SessionEntry{})
	gin.SetMode(gin.TestMode)
	if err := rt.SessionShares.Put("expired", &sharestore.SessionEntry{
		SessionID: "old-session",
		Title:     "Old session",
		ExpiresAt: time.Now().Add(-time.Minute),
		CreatedAt: time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	router.GET("/public/session-shares/:shareID/info", rt.GetPublicSessionShareInfoHandler())

	req := httptest.NewRequest(http.MethodGet, "/public/session-shares/expired/info", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusGone {
		t.Fatalf("expected 410, got %d: %s", resp.Code, resp.Body.String())
	}
	_, exists := rt.SessionShares.Snapshot()["expired"]
	if exists {
		t.Fatal("expected expired share to be removed")
	}
}

func TestPublicSessionShareRateLimitsInvalidPasswords(t *testing.T) {
	rt := newSessionShareTestRuntime(t, map[string]*sharestore.SessionEntry{
		"rate-limited": {
			SessionID:    "session",
			PasswordHash: hashPassword("secret"),
			CreatedAt:    time.Now(),
		},
	})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/public/session-shares/:shareID/access", rt.AccessPublicSessionShareHandler())

	const remoteIP = "198.51.100.33"
	attemptKey := "session-share:rate-limited:" + remoteIP
	publicShareAttempts.Reset(attemptKey)
	t.Cleanup(func() { publicShareAttempts.Reset(attemptKey) })

	for i := 0; i < 8; i++ {
		req := httptest.NewRequest(http.MethodPost, "/public/session-shares/rate-limited/access", bytes.NewBufferString(`{"password":"bad"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = remoteIP + ":1234"
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if resp.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d, want 401: %s", i+1, resp.Code, resp.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/public/session-shares/rate-limited/access", bytes.NewBufferString(`{"password":"bad"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteIP + ":1234"
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusTooManyRequests || resp.Header().Get("Retry-After") == "" {
		t.Fatalf("rate-limited status = %d, Retry-After=%q: %s", resp.Code, resp.Header().Get("Retry-After"), resp.Body.String())
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
	recorder.SetSessionDir(sessionDir)
	recorder.RecordEvent(runtimeevents.UserMessage("run-1", runtimeevents.TurnID("run-1", 1), "run-1:user", message.Message{Role: message.RoleUser, Content: "hello"}))
	if err := recorder.SaveToFile(filepath.Join(sessionDir, eventlog.TranscriptFileName)); err != nil {
		t.Fatalf("save history: %v", err)
	}
}

func newSessionShareTestRuntime(t *testing.T, entries map[string]*sharestore.SessionEntry) *Runtime {
	t.Helper()
	store := sharestore.NewSessionShareStore(filepath.Join(t.TempDir(), "session_share.json"))
	for id, entry := range entries {
		if err := store.Put(id, entry); err != nil {
			t.Fatal(err)
		}
	}
	rt := newTestRuntime(t)
	rt.SessionShares = store
	return rt
}
