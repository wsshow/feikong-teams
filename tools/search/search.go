package search

import (
	"context"
	"fkteams/agentcore"
	"fmt"
	"net/http"
	"time"
)

type Config struct {
	// ToolName is the name of the tool
	// Default: see defaultTextSearchToolName
	ToolName string `json:"tool_name"`
	// ToolDesc is the description of the tool
	// Default: see defaultTextSearchToolDesc
	ToolDesc string `json:"tool_desc"`

	// Timeout specifies the maximum duration for a single request.
	// Default: 30 seconds
	Timeout time.Duration

	// HTTPClient specifies the client to send HTTP requests.
	// If HTTPClient is set, Timeout will not be used.
	// Optional. Default &http.client{Timeout: Timeout}
	HTTPClient *http.Client `json:"http_client"`

	// MaxResults limits the number of results returned
	// Default: 10
	MaxResults int `json:"max_results"`

	// Region is the geographical region for results
	// Default: RegionWT, means all regions
	// Reference: https://duckduckgo.com/duckduckgo-help-pages/settings/params
	Region Region `json:"region"`
}

func NewTextSearchTool(ctx context.Context, config *Config) (agentcore.Tool, error) {
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

	return agentcore.NewTool(agentcore.ToolInfo{Name: name, Desc: desc}, cli.TextSearch)
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
