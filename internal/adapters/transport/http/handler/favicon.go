package handler

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	defaultFaviconSize = 16
	maxFaviconSize     = 64
)

const defaultFaviconUpstream = "https://www.google.com/s2/favicons"

// FaviconProxy 代理并缓存单个 HTTP runtime 的来源图标请求。
type FaviconProxy struct {
	sync.Mutex
	items    map[string]faviconCacheEntry
	client   *http.Client
	upstream string
}

// FaviconProxyOptions 描述 favicon 代理的可替换依赖。
type FaviconProxyOptions struct {
	Client   *http.Client
	Upstream string
}

// NewFaviconProxy 创建独立的 favicon 代理。
func NewFaviconProxy(options FaviconProxyOptions) *FaviconProxy {
	client := options.Client
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	upstream := options.Upstream
	if upstream == "" {
		upstream = defaultFaviconUpstream
	}
	return &FaviconProxy{
		items:    make(map[string]faviconCacheEntry),
		client:   client,
		upstream: upstream,
	}
}

type faviconCacheEntry struct {
	data        []byte
	contentType string
	expiresAt   time.Time
}

// FaviconHandler 代理来源站点图标，避免浏览器侧大量外部 favicon 404 噪音。
func FaviconHandler() gin.HandlerFunc {
	return NewRuntime().FaviconHandler()
}

// FaviconHandler 代理当前 HTTP runtime 的来源站点图标。
func (rt *Runtime) FaviconHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		domain := normalizeFaviconDomain(c.Query("domain"))
		if domain == "" {
			c.Data(http.StatusOK, "image/svg+xml; charset=utf-8", fallbackFaviconSVG("", faviconSize(c.Query("size"))))
			return
		}
		size := faviconSize(c.Query("size"))
		key := fmt.Sprintf("%s:%d", domain, size)
		proxy := rt.Favicons

		if entry, ok := proxy.get(key); ok {
			writeFavicon(c, entry)
			return
		}

		data, contentType, err := proxy.fetch(c.Request.Context(), domain, size)
		if err != nil {
			data = fallbackFaviconSVG(domain, size)
			contentType = "image/svg+xml; charset=utf-8"
		}
		entry := faviconCacheEntry{
			data:        data,
			contentType: contentType,
			expiresAt:   time.Now().Add(24 * time.Hour),
		}
		proxy.set(key, entry)
		writeFavicon(c, entry)
	}
}

func (p *FaviconProxy) get(key string) (faviconCacheEntry, bool) {
	p.Lock()
	defer p.Unlock()
	entry, ok := p.items[key]
	if !ok || time.Now().After(entry.expiresAt) {
		delete(p.items, key)
		return faviconCacheEntry{}, false
	}
	return entry, true
}

func (p *FaviconProxy) set(key string, entry faviconCacheEntry) {
	p.Lock()
	p.items[key] = entry
	p.Unlock()
}

func writeFavicon(c *gin.Context, entry faviconCacheEntry) {
	c.Header("Cache-Control", "public, max-age=86400")
	c.Data(http.StatusOK, entry.contentType, entry.data)
}

func (p *FaviconProxy) fetch(ctx context.Context, domain string, size int) ([]byte, string, error) {
	u, err := url.Parse(p.upstream)
	if err != nil {
		return nil, "", err
	}
	q := u.Query()
	q.Set("domain", domain)
	q.Set("sz", strconv.Itoa(size))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "fkteams")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("favicon upstream status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 {
		return nil, "", fmt.Errorf("empty favicon")
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	if !strings.HasPrefix(contentType, "image/") {
		return nil, "", fmt.Errorf("unexpected favicon content type %s", contentType)
	}
	return data, contentType, nil
}

func normalizeFaviconDomain(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	if strings.Contains(input, "://") {
		if u, err := url.Parse(input); err == nil {
			input = u.Hostname()
		}
	} else if strings.ContainsAny(input, "/?#") {
		if u, err := url.Parse("https://" + input); err == nil {
			input = u.Hostname()
		}
	}
	input = strings.Trim(strings.ToLower(input), ".")
	if input == "" || strings.ContainsAny(input, " /\\\t\r\n") {
		return ""
	}
	return input
}

func faviconSize(raw string) int {
	size, err := strconv.Atoi(raw)
	if err != nil || size <= 0 {
		return defaultFaviconSize
	}
	if size > maxFaviconSize {
		return maxFaviconSize
	}
	return size
}

func fallbackFaviconSVG(domain string, size int) []byte {
	label := "#"
	trimmed := strings.TrimPrefix(domain, "www.")
	if trimmed != "" {
		first := []rune(strings.ToUpper(trimmed))[0]
		if (first >= 'A' && first <= 'Z') || (first >= '0' && first <= '9') {
			label = string(first)
		}
	}
	safeLabel := html.EscapeString(label)
	safeSize := faviconSize(strconv.Itoa(size))
	return []byte(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d"><rect width="%d" height="%d" rx="%d" fill="#f6efe4"/><text x="50%%" y="54%%" dominant-baseline="middle" text-anchor="middle" font-family="Arial,sans-serif" font-size="%d" font-weight="700" fill="#6f6356">%s</text></svg>`, safeSize, safeSize, safeSize, safeSize, safeSize, safeSize, safeSize/4, safeSize*11/16, safeLabel))
}
