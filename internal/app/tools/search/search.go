package search

import (
	"context"
	runtimeport "fkteams/internal/ports/runtime"
	"fmt"
	"net/http"
	"time"
)

type Config struct {
	// ToolName 是工具名称。
	ToolName string `json:"tool_name"`
	// ToolDesc 是工具描述。
	ToolDesc string `json:"tool_desc"`

	// Timeout 是单次请求超时时间，默认 30 秒。
	Timeout time.Duration

	// HTTPClient 是自定义 HTTP 客户端，设置后忽略 Timeout。
	HTTPClient *http.Client `json:"http_client"`

	// MaxResults 限制返回结果数量，默认 10。
	MaxResults int `json:"max_results"`

	// Region 是搜索地区，默认 RegionWT。
	Region Region `json:"region"`
}

func NewTextSearchTool(ctx context.Context, config *Config) (runtimeport.Tool, error) {
	if config == nil {
		config = &Config{}
	}

	name := config.ToolName
	if name == "" {
		name = defaultTextSearchToolName
	}
	desc := config.ToolDesc
	if desc == "" {
		desc = defaultTextSearchToolDesc
	}

	cli, err := buildClient(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create duckduckgo client: %w", err)
	}

	return runtimeport.NewTool(runtimeport.ToolInfo{Name: name, Desc: desc}, cli.TextSearch)
}

func NewSearch(ctx context.Context, config *Config) (Search, error) {
	return buildClient(ctx, config)
}

func buildClient(_ context.Context, config *Config) (Search, error) {
	if config == nil {
		config = &Config{}
	}

	region := config.Region
	if region == "" {
		region = RegionWT
	}

	maxResults := config.MaxResults
	if maxResults <= 0 {
		maxResults = 10
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	var httpCli *http.Client
	if config.HTTPClient != nil {
		httpCli = config.HTTPClient
	} else {
		httpCli = &http.Client{
			Timeout: timeout,
		}
	}

	return &client{
		httpCli:    httpCli,
		maxResults: maxResults,
		region:     region,
	}, nil
}
