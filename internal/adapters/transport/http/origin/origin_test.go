package origin

import (
	"net/http"
	"testing"
)

func TestIsAllowedSameOrigin(t *testing.T) {
	req := requestWithOrigin("http://example.com:23456", "example.com:23456")
	if !isAllowed(req, nil) {
		t.Fatal("expected same origin to be allowed")
	}
}

func TestIsAllowedLoopbackOrigin(t *testing.T) {
	req := requestWithOrigin("http://localhost:5173", "example.com:23456")
	if !isAllowed(req, nil) {
		t.Fatal("expected localhost origin to be allowed")
	}
}

func TestIsAllowedRejectsUnlistedCrossOrigin(t *testing.T) {
	req := requestWithOrigin("https://evil.example", "example.com:23456")
	if isAllowed(req, nil) {
		t.Fatal("expected unlisted cross origin to be rejected")
	}
}

func TestIsAllowedConfiguredOrigin(t *testing.T) {
	req := requestWithOrigin("https://app.example", "api.example:23456")
	if !isAllowed(req, []string{"https://app.example"}) {
		t.Fatal("expected configured origin to be allowed")
	}
}

func requestWithOrigin(origin, host string) *http.Request {
	req, _ := http.NewRequest(http.MethodGet, "http://"+host+"/ws", nil)
	req.Host = host
	req.Header.Set("Origin", origin)
	return req
}
