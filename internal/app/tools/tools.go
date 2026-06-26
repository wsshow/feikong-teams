package tools

import (
	"context"
	"fmt"
	"strings"

	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/resources"
)

func GetToolsByName(ctx context.Context, name string) ([]runtimeport.Tool, error) {
	return GetToolsByNameWithCleaner(ctx, name, nil)
}

// GetToolsByNameWithCleaner 按名称返回工具列表，并按需注册进程级清理函数。
func GetToolsByNameWithCleaner(ctx context.Context, name string, cleaner *resources.Cleaner) ([]runtimeport.Tool, error) {
	registry, err := RequireRegistry(ctx)
	if err != nil {
		return nil, err
	}
	return registry.GetToolsByNameWithCleaner(ctx, name, cleaner)
}

func (r *ToolGroupRegistry) GetToolsByName(ctx context.Context, name string) ([]runtimeport.Tool, error) {
	return r.GetToolsByNameWithCleaner(ctx, name, nil)
}

func (r *ToolGroupRegistry) GetToolsByNameWithCleaner(ctx context.Context, name string, cleaner *resources.Cleaner) ([]runtimeport.Tool, error) {
	if resolved, ok, err := r.Resolve(name, cleaner); ok || err != nil {
		return resolved, err
	}
	if name, ok := strings.CutPrefix(name, "mcp-"); ok {
		return r.GetMCPToolsByName(ctx, name)
	}
	return nil, fmt.Errorf("tool %s not found", name)
}

// BuiltinToolNames 返回所有内置工具组名称
func BuiltinToolNames(ctx context.Context) []string {
	registry, ok := RegistryFromContext(ctx)
	if !ok {
		return nil
	}
	return registry.Names()
}

// GetAllToolNames 返回所有可用的工具名列表（内置 + MCP）
func GetAllToolNames(ctx context.Context) []string {
	registry, ok := RegistryFromContext(ctx)
	if !ok {
		return nil
	}
	return registry.GetAllToolNames(ctx)
}

func (r *ToolGroupRegistry) GetAllToolNames(ctx context.Context) []string {
	builtinNames := r.Names()
	names := make([]string, 0, len(builtinNames))
	names = append(names, builtinNames...)
	mcpGroups, err := r.GetAllMCPToolGroups(ctx)
	if err == nil {
		for name := range mcpGroups {
			names = append(names, "mcp-"+name)
		}
	}
	return names
}
