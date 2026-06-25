// Package origin 提供 HTTP 和 WebSocket 的 Origin 校验。
package origin

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"fkteams/internal/app/config"
)

// IsAllowed 判断请求 Origin 是否允许访问当前服务。
func IsAllowed(r *http.Request) bool {
	return isAllowed(r, config.Get().Server.AllowOrigins)
}

func isAllowed(r *http.Request, allowList []string) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}

	originURL, err := url.Parse(origin)
	if err != nil || originURL.Scheme == "" || originURL.Host == "" {
		return false
	}

	if sameHost(originURL.Host, r.Host) {
		return true
	}

	if isLoopbackHost(originURL.Hostname()) {
		return true
	}

	return matchesAllowedOrigin(originURL, allowList)
}

// AllowedOrigin 返回可用于 Access-Control-Allow-Origin 的值。
func AllowedOrigin(r *http.Request) (string, bool) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return "", false
	}
	if !IsAllowed(r) {
		return "", false
	}
	return origin, true
}

func matchesAllowedOrigin(originURL *url.URL, allowList []string) bool {
	for _, item := range allowList {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if item == "*" {
			return true
		}
		if allowedURL, err := url.Parse(item); err == nil && allowedURL.Scheme != "" && allowedURL.Host != "" {
			if strings.EqualFold(allowedURL.Scheme, originURL.Scheme) && sameHost(allowedURL.Host, originURL.Host) {
				return true
			}
			continue
		}
		if sameHost(item, originURL.Host) {
			return true
		}
	}
	return false
}

func sameHost(a, b string) bool {
	aHost, aPort := splitHostPort(a)
	bHost, bPort := splitHostPort(b)
	if !strings.EqualFold(aHost, bHost) {
		return false
	}
	return aPort == "" || bPort == "" || aPort == bPort
}

func splitHostPort(hostport string) (host, port string) {
	if hostport == "" {
		return "", ""
	}
	if h, p, err := net.SplitHostPort(hostport); err == nil {
		return strings.Trim(h, "[]"), p
	}
	u, err := url.Parse("//" + hostport)
	if err != nil {
		return strings.Trim(hostport, "[]"), ""
	}
	return strings.Trim(u.Hostname(), "[]"), u.Port()
}

func isLoopbackHost(host string) bool {
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
