package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestOKFailAndBasicHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/ok", func(c *gin.Context) { OK(c, gin.H{"value": "done"}) })
	router.GET("/fail", func(c *gin.Context) { Fail(c, http.StatusTeapot, "failed") })
	router.GET("/health", HealthHandler())
	router.GET("/version", VersionHandler())

	tests := []struct {
		path     string
		status   int
		code     int
		message  string
		hasValue bool
	}{
		{path: "/ok", status: http.StatusOK, code: 0, message: "success", hasValue: true},
		{path: "/fail", status: http.StatusTeapot, code: 1, message: "failed"},
		{path: "/health", status: http.StatusOK, code: 0, message: "success", hasValue: true},
		{path: "/version", status: http.StatusOK, code: 0, message: "success", hasValue: true},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, tt.path, nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		if resp.Code != tt.status {
			t.Fatalf("%s status = %d, want %d: %s", tt.path, resp.Code, tt.status, resp.Body.String())
		}
		var got Response
		if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
			t.Fatalf("%s unmarshal response: %v", tt.path, err)
		}
		if got.Code != tt.code || got.Message != tt.message {
			t.Fatalf("%s response = %#v, want code=%d message=%q", tt.path, got, tt.code, tt.message)
		}
		if tt.hasValue && got.Data == nil {
			t.Fatalf("%s expected response data", tt.path)
		}
	}
}
