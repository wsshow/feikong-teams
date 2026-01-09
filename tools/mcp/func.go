package mcp

import (
	"context"
	"fkteams/config"
	"fmt"
	"log"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

type MCPClient struct {
	Name   string
	Desc   string
	Client *client.Client
}

func setupMCPClients(ctx context.Context) (mcpClients []MCPClient, err error) {

	mcpClients = make([]MCPClient, 0)

	cfg, err := config.Get()
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %v", err)
	}

	mcpServerConfigs := cfg.Custom.MCPServers

	for _, mcpServerConfig := range mcpServerConfigs {

		mcpServerName := mcpServerConfig.Name

		if !mcpServerConfig.Enabled {
			log.Printf("Skipping disabled MCP server: %s", mcpServerName)
			continue
		}

		log.Printf("Connecting to MCP server: %s", mcpServerName)

		var mcpClient *client.Client
		switch mcpServerConfig.TransportType {
		case "http":
			mcpClient, err = client.NewStreamableHttpClient(
				mcpServerConfig.URL,
			)
		case "sse":
			mcpClient, err = client.NewSSEMCPClient(
				mcpServerConfig.URL,
			)
		case "stdio":
			mcpClient, err = client.NewStdioMCPClient(
				mcpServerConfig.Command,
				mcpServerConfig.EnvVars,
				mcpServerConfig.Args...,
			)
		default:
			return nil, fmt.Errorf("unsupported MCP transport type for %s: %s", mcpServerName, mcpServerConfig.TransportType)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to create MCP client for %s: %v", mcpServerName, err)
		}

		err = mcpClient.Start(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to start MCP client for %s: %v", mcpServerName, err)
		}

		initRequest := mcp.InitializeRequest{}
		initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		initRequest.Params.ClientInfo = mcp.Implementation{
			Name:    "fkteams",
			Version: "1.0.0",
		}
		initRequest.Params.Capabilities = mcp.ClientCapabilities{}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		initializeResult, err := mcpClient.Initialize(ctx, initRequest)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize MCP client for %s: %v", mcpServerName, err)
		}

		log.Printf("Initialized MCP client for %s", mcpServerName)

		mcpClient.OnNotification(func(notification mcp.JSONRPCNotification) {
			log.Printf("Received notification from MCP server %s: %+v", mcpServerName, notification)
		})

		desc := mcpServerConfig.Desc
		if len(initializeResult.Instructions) > 0 {
			desc = initializeResult.Instructions
		}

		mcpClients = append(mcpClients, MCPClient{
			Name:   mcpServerConfig.Name,
			Desc:   desc,
			Client: mcpClient,
		})
	}

	return mcpClients, nil
}
