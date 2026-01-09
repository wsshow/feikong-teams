package mcp

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/tool"
)

func getAllMCPTools() (dtg DictToolGroup, err error) {

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

var cachedTools DictToolGroup

func GetToolsByName(toolName string) ([]tool.BaseTool, error) {
	if cachedTools == nil {
		var err error
		cachedTools, err = getAllMCPTools()
		if err != nil {
			return nil, err
		}
	}
	if tg, exists := cachedTools[toolName]; exists {
		return tg.Tools, nil
	}
	return nil, fmt.Errorf("MCP tool %s not found", toolName)
}
