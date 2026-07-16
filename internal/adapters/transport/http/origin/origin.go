// Package origin 提供 HTTP 和 WebSocket 的 Origin 校验。
package origin

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"fkteams/internal/app/config"
)

type canonicalOrigin struct {
	scheme string
	host   string
	port   string
}

// IsAllowed 判断请求 Origin 是否允许访问当前服务。
func IsAllowed(r *http.Request) bool {
	if r == nil {
		return false
	}
	server := config.Get().Server
	_, allowed := allowedOriginWithProxies(r, server.AllowOrigins, server.TrustedProxies)
	return strings.TrimSpace(r.Header.Get("Origin")) == "" || allowed
}

func isAllowed(r *http.Request, allowList []string) bool {
	if r == nil || strings.TrimSpace(r.Header.Get("Origin")) == "" {
		return true
	}
	_, allowed := allowedOriginWithProxies(r, allowList, nil)
	return allowed
}

// AllowedOrigin 返回可用于 Access-Control-Allow-Origin 的值。
func AllowedOrigin(r *http.Request) (string, bool) {
	server := config.Get().Server
	return allowedOriginWithProxies(r, server.AllowOrigins, server.TrustedProxies)
}

func allowedOrigin(r *http.Request, allowList []string) (string, bool) {
	return allowedOriginWithProxies(r, allowList, nil)
}

func allowedOriginWithProxies(r *http.Request, allowList, trustedProxies []string) (string, bool) {
	if r == nil {
		return "", false
	}
	rawOrigin := strings.TrimSpace(r.Header.Get("Origin"))
	if rawOrigin == "" {
		return "", false
	}
	origin, ok := parseOrigin(rawOrigin)
	if !ok {
		return "", false
	}
	if requestOrigin, valid := originFromRequest(r, trustedProxies); valid && origin == requestOrigin {
		return rawOrigin, true
	}

	wildcard := false
	for _, item := range allowList {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if item == "*" {
			wildcard = true
			continue
		}
		allowed, valid := parseOrigin(item)
		if valid && allowed == origin {
			return rawOrigin, true
		}
	}
	if wildcard {
		return "*", true
	}
	return "", false
}

func parseOrigin(raw string) (canonicalOrigin, bool) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" && parsed.Path != "/" {
		return canonicalOrigin{}, false
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return canonicalOrigin{}, false
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host == "" {
		return canonicalOrigin{}, false
	}
	port := parsed.Port()
	if port == "" {
		port = defaultPort(scheme)
	}
	return canonicalOrigin{scheme: scheme, host: host, port: port}, true
}

func originFromRequest(r *http.Request, trustedProxies []string) (canonicalOrigin, bool) {
	if r == nil || strings.TrimSpace(r.Host) == "" {
		return canonicalOrigin{}, false
	}
	scheme := ""
	if r.TLS != nil {
		scheme = "https"
	} else if forwardedScheme, ok := trustedForwardedScheme(r, trustedProxies); ok {
		scheme = forwardedScheme
	} else if r.URL != nil {
		scheme = strings.ToLower(r.URL.Scheme)
	}
	if scheme == "" {
		scheme = "http"
	}
	return parseOrigin(scheme + "://" + r.Host)
}

func trustedForwardedScheme(r *http.Request, trustedProxies []string) (string, bool) {
	if r == nil || len(trustedProxies) == 0 || !remoteIsTrusted(r.RemoteAddr, trustedProxies) {
		return "", false
	}
	values := strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")
	scheme := strings.ToLower(strings.TrimSpace(values[len(values)-1]))
	if scheme == "http" || scheme == "https" {
		return scheme, true
	}
	return "", false
}

func remoteIsTrusted(remoteAddr string, trustedProxies []string) bool {
	host := strings.TrimSpace(remoteAddr)
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return false
	}
	for _, entry := range trustedProxies {
		entry = strings.TrimSpace(entry)
		if trustedIP := net.ParseIP(entry); trustedIP != nil && trustedIP.Equal(ip) {
			return true
		}
		if _, network, err := net.ParseCIDR(entry); err == nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

func defaultPort(scheme string) string {
	if scheme == "https" {
		return "443"
	}
	return "80"
}
