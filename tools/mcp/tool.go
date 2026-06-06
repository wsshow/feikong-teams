package mcp

import (
	"context"
	"fkteams/agentcore"
	"fmt"
)

type ToolProvider func(context.Context, any) ([]agentcore.Tool, error)

var toolProvider ToolProvider

func RegisterToolProvider(provider ToolProvider) {
	toolProvider = provider
}

func getAllMCPTools() (dtg DictToolGroup, err error) {

	ctx := context.Background()
	mcpClients, err := setupMCPClients(ctx)
	if err != nil {
		return nil, err
	}

	dtg = make(DictToolGroup, len(mcpClients))
	for _, mcpClient := range mcpClients {
		if toolProvider == nil {
			return nil, fmt.Errorf("MCP tool provider is not registered")
		}
		tools, err := toolProvider(ctx, mcpClient.Client)
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

func GetToolsByName(toolName string) ([]agentcore.Tool, error) {
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

// GetAllToolGroups 返回所有 MCP 工具组
func GetAllToolGroups() (DictToolGroup, error) {
	if cachedTools == nil {
		var err error
		cachedTools, err = getAllMCPTools()
		if err != nil {
			return nil, err
		}
	}
	return cachedTools, nil
}

// ClearCache 清除 MCP 工具缓存，下次调用时重新连接
func ClearCache() {
	cachedTools = nil
}
