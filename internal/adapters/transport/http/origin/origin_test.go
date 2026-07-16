package origin

import (
	"crypto/tls"
	"net/http"
	"testing"
)

func TestIsAllowedSameOriginRequiresSchemeHostAndEffectivePort(t *testing.T) {
	tests := []struct {
		name    string
		origin  string
		host    string
		tls     bool
		allowed bool
	}{
		{name: "exact", origin: "http://example.com:23456", host: "example.com:23456", allowed: true},
		{name: "default HTTP port", origin: "http://example.com", host: "example.com:80", allowed: true},
		{name: "default HTTPS port", origin: "https://example.com:443", host: "example.com", tls: true, allowed: true},
		{name: "different scheme", origin: "https://example.com", host: "example.com:443", allowed: false},
		{name: "different port", origin: "http://example.com:5173", host: "example.com:23456", allowed: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := requestWithOrigin(tt.origin, tt.host, tt.tls)
			if got := isAllowed(req, nil); got != tt.allowed {
				t.Fatalf("isAllowed() = %v, want %v", got, tt.allowed)
			}
		})
	}
}

func TestIsAllowedWithoutOriginForNonBrowserClients(t *testing.T) {
	req := requestWithOrigin("", "example.com:23456", false)
	if !isAllowed(req, nil) {
		t.Fatal("requests without Origin should be allowed")
	}
}

func TestIsAllowedRejectsUnlistedLoopbackAndCrossOrigin(t *testing.T) {
	for _, origin := range []string{"http://localhost:5173", "http://127.0.0.1:5173", "https://evil.example"} {
		req := requestWithOrigin(origin, "example.com:23456", false)
		if isAllowed(req, nil) {
			t.Fatalf("expected %q to be rejected", origin)
		}
	}
}

func TestIsAllowedConfiguredOriginIsExact(t *testing.T) {
	allowList := []string{"https://app.example", "http://localhost:5173"}
	if !isAllowed(requestWithOrigin("https://app.example:443", "api.example:23456", false), allowList) {
		t.Fatal("expected configured HTTPS origin to be allowed")
	}
	if !isAllowed(requestWithOrigin("http://localhost:5173", "api.example:23456", false), allowList) {
		t.Fatal("expected configured loopback origin to be allowed")
	}
	for _, origin := range []string{"http://app.example", "https://app.example:444", "http://localhost:5174"} {
		if isAllowed(requestWithOrigin(origin, "api.example:23456", false), allowList) {
			t.Fatalf("expected %q not to match configured origin", origin)
		}
	}
	if isAllowed(requestWithOrigin("https://legacy.example", "api.example:23456", false), []string{"legacy.example"}) {
		t.Fatal("bare allow-list hosts must not bypass scheme and port checks")
	}
}

func TestAllowedOriginUsesWildcardWithoutReflectingCredentialsOrigin(t *testing.T) {
	req := requestWithOrigin("https://app.example", "api.example", false)
	got, ok := allowedOrigin(req, []string{"*"})
	if !ok || got != "*" {
		t.Fatalf("allowedOrigin() = (%q, %v), want wildcard", got, ok)
	}
}

func TestIsAllowedRejectsMalformedOrigins(t *testing.T) {
	for _, origin := range []string{"null", "https://user@example.com", "https://example.com/path", "file:///tmp/test"} {
		if isAllowed(requestWithOrigin(origin, "example.com", false), []string{"*"}) {
			t.Fatalf("expected malformed origin %q to be rejected", origin)
		}
	}
}

func TestTrustedProxyMayProvideExternalScheme(t *testing.T) {
	req := requestWithOrigin("https://app.example", "app.example", false)
	req.URL.Scheme = ""
	req.RemoteAddr = "127.0.0.1:43210"
	req.Header.Set("X-Forwarded-Proto", "https")
	if _, ok := allowedOriginWithProxies(req, nil, []string{"127.0.0.1"}); !ok {
		t.Fatal("trusted proxy scheme should preserve same-origin HTTPS")
	}
	if _, ok := allowedOriginWithProxies(req, nil, []string{"10.0.0.0/8"}); ok {
		t.Fatal("untrusted proxy scheme must be ignored")
	}
}

func requestWithOrigin(origin, host string, useTLS bool) *http.Request {
	req, _ := http.NewRequest(http.MethodGet, "http://"+host+"/ws", nil)
	req.Host = host
	if useTLS {
		req.TLS = &tls.ConnectionState{}
	}
	req.Header.Set("Origin", origin)
	return req
}
