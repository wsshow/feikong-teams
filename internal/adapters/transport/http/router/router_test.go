package router

import (
	"fkteams/internal/adapters/transport/http/handler"
	"fkteams/internal/app/config"
	"fkteams/internal/runtime/env"
	"fkteams/web"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegisterAPIRoutesIncludesCoreEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	registerAPIRoutesWithRuntime(engine, false, nil, handler.NewRuntime())

	routes := routeSet(engine.Routes())
	for _, route := range []string{
		"GET /health",
		"GET /live",
		"GET /ready",
		"GET /ws",
		"POST /api/fkteams/login",
		"POST /api/fkteams/logout",
		"GET /v1/models",
		"POST /v1/chat/completions",
		"GET /api/fkteams/version",
		"GET /api/fkteams/agents",
		"GET /api/fkteams/favicon",
		"POST /api/fkteams/ai/skills/draft",
		"POST /api/fkteams/chat",
		"POST /api/fkteams/stream/start",
		"PATCH /api/fkteams/stream/queue/:sessionID/:queueID",
		"DELETE /api/fkteams/stream/queue/:sessionID/:queueID",
		"GET /api/fkteams/files/serve/*filepath",
		"GET /api/fkteams/preview/:linkId/render/*filepath",
		"POST /api/fkteams/preview/:linkId/auth",
		"POST /api/fkteams/session-shares",
		"GET /api/fkteams/public/session-shares/:shareID/info",
		"GET /api/fkteams/sessions/:sessionID",
		"PATCH /api/fkteams/sessions/:sessionID",
		"POST /api/fkteams/schedules",
		"PUT /api/fkteams/schedules/:id",
		"DELETE /api/fkteams/schedules/:id",
		"GET /api/fkteams/schedules/:id/history/:filename",
		"POST /api/fkteams/skills",
		"POST /api/fkteams/skills/:slug/files",
		"GET /api/fkteams/skills/:slug/file",
		"PUT /api/fkteams/skills/:slug/file",
		"DELETE /api/fkteams/skills/:slug/file",
		"POST /api/fkteams/memory/clear",
		"GET /api/fkteams/config/tool-catalog",
		"GET /api/fkteams/config/template-vars",
		"POST /api/fkteams/providers/models",
		"POST /api/fkteams/shutdown",
		"POST /api/fkteams/restart",
	} {
		if !routes[route] {
			t.Fatalf("route %s was not registered", route)
		}
	}
}

func TestRegisterAPIRoutesKeepsLoginAvailableForAuthHotReload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	registerAPIRoutesWithRuntime(engine, true, nil, handler.NewRuntime())

	routes := routeSet(engine.Routes())
	if !routes["POST /api/fkteams/login"] {
		t.Fatal("login route should be registered when auth is enabled")
	}
}

func TestNewEngineAddsMiddlewareAndRoutesCanBeRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := newEngine(false)
	registerAPIRoutesWithRuntime(engine, false, nil, handler.NewRuntime())

	if len(engine.Routes()) == 0 {
		t.Fatal("engine should have registered routes")
	}
}

func TestControlRouteRejectsOversizedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	registerAPIRoutesWithRuntime(engine, false, nil, handler.NewRuntime())

	request := httptest.NewRequest(http.MethodPost, "/api/fkteams/login", strings.NewReader(strings.Repeat("x", int(controlBodyLimit)+1)))
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestNewEngineOnlyTrustsConfiguredProxyHeaders(t *testing.T) {
	t.Setenv(env.AppDir, t.TempDir())
	gin.SetMode(gin.TestMode)

	clientIP := func(engine *gin.Engine) string {
		engine.GET("/client-ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })
		request := httptest.NewRequest(http.MethodGet, "/client-ip", nil)
		request.RemoteAddr = "203.0.113.10:4321"
		request.Header.Set("X-Forwarded-For", "198.51.100.20")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		return response.Body.String()
	}

	if err := config.Save(&config.Config{}); err != nil {
		t.Fatalf("save default config: %v", err)
	}
	if got := clientIP(newEngine(false)); got != "203.0.113.10" {
		t.Fatalf("untrusted proxy client IP = %q, want remote address", got)
	}

	if err := config.Save(&config.Config{Server: config.Server{TrustedProxies: []string{"203.0.113.10"}}}); err != nil {
		t.Fatalf("save trusted proxy config: %v", err)
	}
	if got := clientIP(newEngine(false)); got != "198.51.100.20" {
		t.Fatalf("trusted proxy client IP = %q, want forwarded address", got)
	}
}

func TestServeHTMLServesSPAEntry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/", func(c *gin.Context) {
		serveHTML(c, web.GetFS())
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("expected no-cache html response, got %q", got)
	}

	body := recorder.Body.String()
	for _, ref := range []string{
		"/assets/",
		`id="root"`,
	} {
		if !strings.Contains(body, ref) {
			t.Fatalf("expected html to contain %q", ref)
		}
	}
}

func TestChatDeepLinkServesSPAEntry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	webFS := web.GetFS()
	serveIndex := func(c *gin.Context) {
		serveHTML(c, webFS)
	}
	engine.GET("/chat", serveIndex)
	engine.GET("/chat/:sessionID", serveIndex)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/chat/bf8e4112-3255-4b31-ba16-3fc7f486f206", nil)
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("expected html content type, got %q", got)
	}
	if body := recorder.Body.String(); !strings.Contains(body, `id="root"`) {
		t.Fatalf("expected SPA entry body, got %q", body)
	}
}

func TestSPAFallbackServesClientRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.NoRoute(spaFallback(web.GetFS()))

	for _, target := range []string{
		"/login",
		"/settings/profile",
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, target, nil)
		engine.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Fatalf("expected status %d for %s, got %d", http.StatusOK, target, recorder.Code)
		}
		if got := recorder.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
			t.Fatalf("expected html content type for %s, got %q", target, got)
		}
		if body := recorder.Body.String(); !strings.Contains(body, `id="root"`) {
			t.Fatalf("expected SPA entry body for %s, got %q", target, body)
		}
	}
}

func TestSPAFallbackSkipsAPIAssetsAndNonGETRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.NoRoute(spaFallback(web.GetFS()))

	tests := []struct {
		method string
		target string
	}{
		{method: http.MethodGet, target: "/api/fkteams/missing"},
		{method: http.MethodGet, target: "/v1/missing"},
		{method: http.MethodGet, target: "/assets/missing.js"},
		{method: http.MethodPost, target: "/login"},
	}

	for _, tt := range tests {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(tt.method, tt.target, nil)
		engine.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusNotFound {
			t.Fatalf("expected status %d for %s %s, got %d", http.StatusNotFound, tt.method, tt.target, recorder.Code)
		}
		if got := recorder.Body.String(); strings.Contains(got, `id="root"`) {
			t.Fatalf("expected non-SPA response for %s %s", tt.method, tt.target)
		}
	}
}

func TestServeAssetsUsesImmutableCache(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/assets/*filepath", serveAssets(web.GetFS()))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/assets/fkteams-logo.svg", nil)
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("expected immutable static response, got %q", got)
	}
}

func routeSet(routes gin.RoutesInfo) map[string]bool {
	result := make(map[string]bool, len(routes))
	for _, route := range routes {
		result[route.Method+" "+route.Path] = true
	}
	return result
}
