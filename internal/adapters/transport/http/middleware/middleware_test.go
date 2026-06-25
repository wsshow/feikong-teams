package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"fkteams/internal/app/config"
	"fkteams/internal/runtime/env"

	"github.com/gin-gonic/gin"
)

func TestMaxBodySizeRejectsLargeContentLength(t *testing.T) {
	router := testRouter()
	router.Use(MaxBodySize(4))
	router.POST("/", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("12345"))
	req.ContentLength = 5
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestCorsAllowsConfiguredOriginAndRejectsUnlisted(t *testing.T) {
	t.Setenv(env.AppDir, t.TempDir())
	if err := config.Save(&config.Config{Server: config.Server{
		AllowOrigins: []string{"https://app.example"},
	}}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	router := testRouter()
	router.Use(Cors())
	router.GET("/", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	router.OPTIONS("/", func(c *gin.Context) { c.String(http.StatusOK, "should not reach") })

	req := httptest.NewRequest(http.MethodGet, "http://api.example/", nil)
	req.Host = "api.example"
	req.Header.Set("Origin", "https://app.example")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("allowed status = %d, want 200", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example" {
		t.Fatalf("allow origin header = %q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "http://api.example/", nil)
	req.Host = "api.example"
	req.Header.Set("Origin", "https://evil.example")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("rejected status = %d, want 403", w.Code)
	}

	req = httptest.NewRequest(http.MethodOptions, "http://api.example/", nil)
	req.Host = "api.example"
	req.Header.Set("Origin", "https://app.example")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", w.Code)
	}
}

func TestAPIKeyAuth(t *testing.T) {
	t.Setenv(env.AppDir, t.TempDir())
	if err := config.Save(&config.Config{
		OpenAIAPI: config.OpenAIAPI{APIKeys: []string{"sk-valid"}},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	router := testRouter()
	router.Use(APIKeyAuth())
	router.GET("/v1/models", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	tests := []struct {
		name   string
		auth   string
		status int
	}{
		{name: "missing", status: http.StatusUnauthorized},
		{name: "wrong scheme", auth: "Token sk-valid", status: http.StatusUnauthorized},
		{name: "wrong key", auth: "Bearer sk-wrong", status: http.StatusUnauthorized},
		{name: "valid key", auth: "Bearer sk-valid", status: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			if tt.auth != "" {
				req.Header.Set("Authorization", tt.auth)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != tt.status {
				t.Fatalf("status = %d, want %d body=%s", w.Code, tt.status, w.Body.String())
			}
			if tt.status == http.StatusUnauthorized {
				var body map[string]any
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
					t.Fatalf("unauthorized body is not JSON: %v", err)
				}
			}
		})
	}
}

func testRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}
