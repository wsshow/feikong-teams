package handler

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"fkteams/internal/app/config"
	"fkteams/internal/runtime/env"

	"github.com/gin-gonic/gin"
)

func TestAuthEnabledAndValidateToken(t *testing.T) {
	saveHandlerConfig(t, config.Config{})

	enabled, err := AuthEnabled()
	if err != nil {
		t.Fatalf("AuthEnabled disabled error: %v", err)
	}
	if enabled {
		t.Fatal("expected auth disabled by default")
	}

	saveHandlerConfig(t, config.Config{
		Server: config.Server{Auth: config.ServerAuth{Enabled: true}},
	})
	enabled, err = AuthEnabled()
	if err == nil {
		t.Fatal("expected missing secret error")
	}
	if enabled {
		t.Fatal("expected auth disabled when secret is missing")
	}

	saveHandlerConfig(t, config.Config{
		Server: config.Server{Auth: config.ServerAuth{
			Enabled: true,
			Secret:  "test-secret",
		}},
	})
	enabled, err = AuthEnabled()
	if err != nil {
		t.Fatalf("AuthEnabled enabled error: %v", err)
	}
	if !enabled {
		t.Fatal("expected auth enabled")
	}

	token := generateToken("alice")
	if !ValidateToken(token) {
		t.Fatal("expected generated token to be valid")
	}
	if ValidateToken("invalid") {
		t.Fatal("expected malformed token to be invalid")
	}
	if ValidateToken(token + "0") {
		t.Fatal("expected tampered token to be invalid")
	}
	if ValidateToken(signedTestToken(t, "alice", time.Now().Add(-time.Minute))) {
		t.Fatal("expected expired token to be invalid")
	}
}

func TestLoginHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	saveHandlerConfig(t, config.Config{
		Server: config.Server{Auth: config.ServerAuth{
			Username: "admin",
			Password: "secret",
			Secret:   "token-secret",
		}},
	})

	router := gin.New()
	router.POST("/login", LoginHandler())

	resp := performJSON(router, http.MethodPost, "/login", `{bad json`)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected bad json status 400, got %d", resp.Code)
	}

	resp = performJSON(router, http.MethodPost, "/login", `{"username":"admin","password":"wrong"}`)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected wrong password status 401, got %d", resp.Code)
	}

	resp = performJSON(router, http.MethodPost, "/login", `{"username":"admin","password":"secret"}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected login status 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var got Response
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	data, ok := got.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %#v", got.Data)
	}
	token, ok := data["token"].(string)
	if !ok || token == "" {
		t.Fatalf("expected token string, got %#v", data["token"])
	}
	if !ValidateToken(token) {
		t.Fatal("expected login token to be valid")
	}
}

func saveHandlerConfig(t *testing.T, cfg config.Config) {
	t.Helper()

	t.Setenv(env.AppDir, t.TempDir())
	if err := config.Save(&cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
}

func signedTestToken(t *testing.T, username string, expiry time.Time) string {
	t.Helper()

	payload := username + "|" + expiry.Format(time.RFC3339)
	mac := hmac.New(sha256.New, getTokenSecret())
	if _, err := mac.Write([]byte(payload)); err != nil {
		t.Fatalf("write hmac payload: %v", err)
	}
	return hex.EncodeToString([]byte(payload)) + "." + hex.EncodeToString(mac.Sum(nil))
}

func performJSON(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}
