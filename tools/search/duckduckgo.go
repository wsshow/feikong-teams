package search

import (
	"context"
	"fkteams/fkenv"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/cloudwego/eino/components/tool"
)

func NewDuckDuckGoTool(ctx context.Context) (tool.InvokableTool, error) {
	// 1. 获取代理配置
	proxyStr := fkenv.Get(fkenv.ProxyURL)

	var proxyFunc func(*http.Request) (*url.URL, error)

	if proxyStr != "" {
		// 如果自定义变量存在，解析并固定使用它
		proxyURL, err := url.Parse(proxyStr)
		if err != nil {
			return nil, fmt.Errorf("invalid FEIKONG_PROXY_URL: %w", err)
		}
		proxyFunc = http.ProxyURL(proxyURL)
	} else {
		// 2. 如果不存在，则回退到系统环境变量 (HTTP_PROXY, HTTPS_PROXY, NO_PROXY)
		proxyFunc = http.ProxyFromEnvironment
	}

	transport := &http.Transport{
		Proxy:                 proxyFunc,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   time.Second * 30,
	}
	tool, err := NewTextSearchTool(ctx, &Config{
		ToolName:   "search",
		ToolDesc:   "search for information by duckduckgo",
		Region:     RegionWT,
		MaxResults: 10,
		HTTPClient: httpClient,
	})
	return tool, err
}
