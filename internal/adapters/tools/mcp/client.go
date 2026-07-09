package mcp

import (
	"context"
	"fkteams/internal/runtime/log"
	"fmt"
	"sort"
	"time"

	"fkteams/internal/app/config"

	"github.com/mark3labs/mcp-go/client"
	mcpsdk "github.com/mark3labs/mcp-go/mcp"
)

type MCPClient struct {
	Name   string
	Desc   string
	Client *client.Client
}

func setupMCPClients(ctx context.Context) ([]MCPClient, error) {
	cfg := config.Get()
	clients := make([]MCPClient, 0, len(cfg.Tools.MCPServers))

	for _, server := range cfg.Tools.MCPServers {
		if !server.Enabled {
			log.Printf("Skipping disabled MCP server: %s", serverDisplayName(server))
			continue
		}

		log.Printf("Connecting to MCP server: %s", serverDisplayName(server))
		mcpClient, err := newClient(server)
		if err != nil {
			return nil, err
		}
		if err := mcpClient.Start(ctx); err != nil {
			return nil, fmt.Errorf("failed to start MCP client for %s: %v", serverDisplayName(server), err)
		}

		initCtx, cancel := context.WithTimeout(ctx, serverTimeout(server))
		initializeResult, err := mcpClient.Initialize(initCtx, initializeRequest())
		cancel()
		if err != nil {
			return nil, fmt.Errorf("failed to initialize MCP client for %s: %v", serverDisplayName(server), err)
		}

		log.Printf("Initialized MCP client for %s", serverDisplayName(server))
		mcpClient.OnNotification(func(notification mcpsdk.JSONRPCNotification) {
			log.Printf("Received notification from MCP server %s: %+v", serverDisplayName(server), notification)
		})

		desc := server.Description
		if desc == "" {
			desc = server.Name
		}
		if initializeResult.Instructions != "" {
			desc = initializeResult.Instructions
		}
		clients = append(clients, MCPClient{
			Name:   serverID(server),
			Desc:   desc,
			Client: mcpClient,
		})
	}

	return clients, nil
}

func newClient(server config.MCPServer) (*client.Client, error) {
	switch server.Transport {
	case "http":
		return client.NewStreamableHttpClient(server.URL)
	case "sse":
		return client.NewSSEMCPClient(server.URL)
	case "stdio":
		return client.NewStdioMCPClient(server.Command, envMapToList(server.Env), server.Args...)
	default:
		return nil, fmt.Errorf("unsupported MCP transport type for %s: %s", serverDisplayName(server), server.Transport)
	}
}

func serverID(server config.MCPServer) string {
	if server.ID != "" {
		return server.ID
	}
	return server.Name
}

func serverDisplayName(server config.MCPServer) string {
	if server.Name != "" {
		return server.Name
	}
	return serverID(server)
}

func serverTimeout(server config.MCPServer) time.Duration {
	if server.Timeout == "" {
		return 10 * time.Second
	}
	timeout, err := time.ParseDuration(server.Timeout)
	if err != nil || timeout <= 0 {
		return 10 * time.Second
	}
	return timeout
}

func envMapToList(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, fmt.Sprintf("%s=%s", key, env[key]))
	}
	return result
}

func initializeRequest() mcpsdk.InitializeRequest {
	req := mcpsdk.InitializeRequest{}
	req.Params.ProtocolVersion = mcpsdk.LATEST_PROTOCOL_VERSION
	req.Params.ClientInfo = mcpsdk.Implementation{
		Name:    "fkteams",
		Version: "1.0.0",
	}
	req.Params.Capabilities = mcpsdk.ClientCapabilities{}
	return req
}
