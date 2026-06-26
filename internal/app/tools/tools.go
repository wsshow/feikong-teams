package tools

import (
	"fmt"
	"strings"

	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/resources"
)

func GetToolsByName(name string) ([]runtimeport.Tool, error) {
	return GetToolsByNameWithCleaner(name, nil)
}

// GetToolsByNameWithCleaner 按名称返回工具列表，并按需注册进程级清理函数。
func GetToolsByNameWithCleaner(name string, cleaner *resources.Cleaner) ([]runtimeport.Tool, error) {
	if resolved, ok, err := defaultRegistry.Resolve(name, cleaner); ok || err != nil {
		return resolved, err
	}
	if name, ok := strings.CutPrefix(name, "mcp-"); ok {
		return GetMCPToolsByName(name)
	}
	return nil, fmt.Errorf("tool %s not found", name)
}

// BuiltinToolNames 返回所有内置工具组名称
func BuiltinToolNames() []string {
	return defaultRegistry.Names()
}

// GetAllToolNames 返回所有可用的工具名列表（内置 + MCP）
func GetAllToolNames() []string {
	names := make([]string, 0, len(BuiltinToolNames()))
	names = append(names, BuiltinToolNames()...)
	mcpGroups, err := GetAllMCPToolGroups()
	if err == nil {
		for name := range mcpGroups {
			names = append(names, "mcp-"+name)
		}
	}
	return names
}
