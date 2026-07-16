package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestAuthRejectsAPIAndRedirectsPageToLogin(t *testing.T) {
	t.Setenv(env.AppDir, t.TempDir())
	if err := config.Save(&config.Config{Server: config.Server{Auth: config.ServerAuth{
		Enabled:  true,
		Username: "admin",
		Password: "secret",
		Secret:   "token-secret",
	}}}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	router := testRouter()
	router.Use(Auth())
	router.GET("/chat/:sessionID", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	router.GET("/api/fkteams/version", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	pageTarget := "/chat/session-1?panel=details"
	pageReq := httptest.NewRequest(http.MethodGet, pageTarget, nil)
	pageResp := httptest.NewRecorder()
	router.ServeHTTP(pageResp, pageReq)
	if pageResp.Code != http.StatusFound {
		t.Fatalf("page status = %d, want %d", pageResp.Code, http.StatusFound)
	}
	wantLocation := "/login?next=" + url.QueryEscape(pageTarget)
	if got := pageResp.Header().Get("Location"); got != wantLocation {
		t.Fatalf("redirect location = %q, want %q", got, wantLocation)
	}

	apiReq := httptest.NewRequest(http.MethodGet, "/api/fkteams/version", nil)
	apiResp := httptest.NewRecorder()
	router.ServeHTTP(apiResp, apiReq)
	if apiResp.Code != http.StatusUnauthorized {
		t.Fatalf("API status = %d, want %d", apiResp.Code, http.StatusUnauthorized)
	}
}

func TestAuthReadsHotReloadedConfig(t *testing.T) {
	t.Setenv(env.AppDir, t.TempDir())
	if err := config.Save(&config.Config{}); err != nil {
		t.Fatalf("save disabled config: %v", err)
	}

	router := testRouter()
	router.Use(Auth())
	router.GET("/api/fkteams/version", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	request := func() int {
		req := httptest.NewRequest(http.MethodGet, "/api/fkteams/version", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		return resp.Code
	}
	if got := request(); got != http.StatusOK {
		t.Fatalf("disabled auth status = %d, want %d", got, http.StatusOK)
	}

	if err := config.Save(&config.Config{Server: config.Server{Auth: config.ServerAuth{
		Enabled:  true,
		Username: "admin",
		Password: "secret",
		Secret:   "token-secret",
	}}}); err != nil {
		t.Fatalf("save enabled config: %v", err)
	}
	if got := request(); got != http.StatusUnauthorized {
		t.Fatalf("enabled auth status = %d, want %d", got, http.StatusUnauthorized)
	}

	if err := config.Save(&config.Config{}); err != nil {
		t.Fatalf("restore disabled config: %v", err)
	}
	if got := request(); got != http.StatusOK {
		t.Fatalf("re-disabled auth status = %d, want %d", got, http.StatusOK)
	}
}

func testRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}
