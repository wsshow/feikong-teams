package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestFaviconHandlerProxiesAndCachesUpstreamIcon(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Query().Get("domain") != "example.com" || r.URL.Query().Get("sz") != "16" {
			t.Fatalf("unexpected upstream query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png"))
	}))
	defer upstream.Close()
	withFaviconTestState(t, upstream.URL)

	router := gin.New()
	router.GET("/favicon", FaviconHandler())

	for i := 0; i < 2; i++ {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/favicon?domain=example.com&size=16", nil)
		router.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", resp.Code, resp.Body.String())
		}
		if got := resp.Header().Get("Content-Type"); got != "image/png" {
			t.Fatalf("content type = %q, want image/png", got)
		}
		if resp.Body.String() != "png" {
			t.Fatalf("body = %q, want png", resp.Body.String())
		}
	}
	if requests != 1 {
		t.Fatalf("upstream requests = %d, want 1", requests)
	}
}

func TestFaviconHandlerReturnsFallbackSVGOnUpstream404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	withFaviconTestState(t, upstream.URL)

	router := gin.New()
	router.GET("/favicon", FaviconHandler())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/favicon?domain=missing.example&size=32", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	if got := resp.Header().Get("Content-Type"); got != "image/svg+xml; charset=utf-8" {
		t.Fatalf("content type = %q, want svg", got)
	}
	if body := resp.Body.String(); !strings.HasPrefix(body, "<svg") {
		t.Fatalf("fallback body = %q, want svg", body)
	}
}

func withFaviconTestState(t *testing.T, upstream string) {
	t.Helper()
	oldUpstream := faviconUpstream
	oldClient := faviconHTTPClient
	faviconUpstream = upstream
	faviconHTTPClient = &http.Client{Timeout: time.Second}
	faviconCache.Lock()
	oldCache := faviconCache.items
	faviconCache.items = make(map[string]faviconCacheEntry)
	faviconCache.Unlock()
	t.Cleanup(func() {
		faviconUpstream = oldUpstream
		faviconHTTPClient = oldClient
		faviconCache.Lock()
		faviconCache.items = oldCache
		faviconCache.Unlock()
	})
}
