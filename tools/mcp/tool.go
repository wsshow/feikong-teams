package mcp

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/components/tool/mcp"
)

func GetTools() (dtg DictToolGroup, err error) {

	ctx := context.Background()
	mcpClients, err := setupMCPClients(ctx)
	if err != nil {
		return nil, err
	}

	dtg = make(DictToolGroup, len(mcpClients))
	for _, mcpClient := range mcpClients {
		cli := mcpClient.Client
		tools, err := mcp.GetTools(ctx, &mcp.Config{Cli: cli})
		if err != nil {
			return nil, fmt.Errorf("failed to get tools from MCP server %s: %v", mcpClient.Name, err)
		}
		dtg[mcpClient.Name] = ToolGroup{
			Name:  mcpClient.Name,
			Desc:  mcpClient.Desc,
			Tools: tools,
		}
	}

	return dtg, nil
}
