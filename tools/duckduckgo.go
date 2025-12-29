package tools

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/cloudwego/eino-ext/components/tool/duckduckgo/v2"
	"github.com/cloudwego/eino/components/tool"
)

func NewDuckDuckGoTool(ctx context.Context) (tool.InvokableTool, error) {
	proxyStr := os.Getenv("FEIKONG_PROXY_URL")
	proxyURL, err := url.Parse(proxyStr)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
	}
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   time.Second * 30,
	}
	tool, err := duckduckgo.NewTextSearchTool(ctx, &duckduckgo.Config{
		ToolName:   "duckduckgo_search",
		ToolDesc:   "search for information by duckduckgo",
		Region:     duckduckgo.RegionWT,
		MaxResults: 10,
		HTTPClient: httpClient,
	})
	return tool, err
}
