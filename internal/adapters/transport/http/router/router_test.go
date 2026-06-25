package router

import (
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

	registerAPIRoutes(engine, false)

	routes := routeSet(engine.Routes())
	for _, route := range []string{
		"GET /health",
		"GET /ws",
		"GET /v1/models",
		"POST /v1/chat/completions",
		"GET /api/fkteams/version",
		"GET /api/fkteams/agents",
		"GET /api/fkteams/favicon",
		"POST /api/fkteams/chat",
		"POST /api/fkteams/stream/start",
		"PATCH /api/fkteams/stream/queue/:sessionID/:queueID",
		"DELETE /api/fkteams/stream/queue/:sessionID/:queueID",
		"GET /api/fkteams/files/serve/*filepath",
		"GET /api/fkteams/preview/:linkId/render/*filepath",
		"POST /api/fkteams/session-shares",
		"GET /api/fkteams/public/session-shares/:shareID/info",
		"GET /api/fkteams/sessions/:sessionID",
		"GET /api/fkteams/schedules/:id/history/:filename",
		"GET /api/fkteams/skills/:slug/file",
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

	if routes["POST /api/fkteams/login"] {
		t.Fatal("login route should not be registered when auth is disabled")
	}
}

func TestRegisterAPIRoutesAddsLoginWhenAuthEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	registerAPIRoutes(engine, true)

	routes := routeSet(engine.Routes())
	if !routes["POST /api/fkteams/login"] {
		t.Fatal("login route should be registered when auth is enabled")
	}
}

func TestNewEngineAddsMiddlewareAndRoutesCanBeRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := newEngine(false)
	registerAPIRoutes(engine, false)

	if len(engine.Routes()) == 0 {
		t.Fatal("engine should have registered routes")
	}
}

func TestServeHTMLVersionsStaticAssets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/", func(c *gin.Context) {
		serveHTML(c, web.GetFS(), "index.html")
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
		"/static/css/style.css?v=",
		"/static/js/history.js?v=",
		"/static/assets/fkteams-logo.svg?v=",
	} {
		if !strings.Contains(body, ref) {
			t.Fatalf("expected html to contain versioned ref %q", ref)
		}
	}
}

func TestServeStaticStyleVersionsLocalCSSImports(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/static/*filepath", serveStatic(web.GetFS()))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/static/css/style.css?v=test", nil)
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("expected immutable static response, got %q", got)
	}

	body := recorder.Body.String()
	for _, ref := range []string{
		"url('variables.css?v=",
		"url('layout.css?v=",
		"url('messages.css?v=",
	} {
		if !strings.Contains(body, ref) {
			t.Fatalf("expected css to contain versioned import %q", ref)
		}
	}
	if strings.Contains(body, "fonts.googleapis.com/css2?family=Caveat") {
		t.Fatal("style.css should not inline external font imports")
	}
}

func TestStaticAssetVersionUsesCompactBuildTime(t *testing.T) {
	got := compactBuildTime("2026-06-10 16:53:39")
	if got != "20260610165339" {
		t.Fatalf("compact build time = %q, want 20260610165339", got)
	}

	got = compactBuildTime("20260610165339")
	if got != "20260610165339" {
		t.Fatalf("numeric build time = %q, want 20260610165339", got)
	}
}

func TestSanitizeAssetVersionPartAvoidsEscapedQueryCharacters(t *testing.T) {
	got := sanitizeAssetVersionPart("0.0.1 beta+local")
	if got != "0.0.1-beta-local" {
		t.Fatalf("sanitized version = %q, want 0.0.1-beta-local", got)
	}
}

func routeSet(routes gin.RoutesInfo) map[string]bool {
	result := make(map[string]bool, len(routes))
	for _, route := range routes {
		result[route.Method+" "+route.Path] = true
	}
	return result
}
