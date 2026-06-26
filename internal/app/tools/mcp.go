package tools

import (
	"context"
	"fmt"

	runtimeport "fkteams/internal/ports/runtime"
	toolport "fkteams/internal/ports/tools"
)

func (r *ToolGroupRegistry) RegisterMCPProvider(provider toolport.MCPProvider) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mcpProvider = provider
}

func ClearMCPToolCache(ctx context.Context) {
	if registry, ok := RegistryFromContext(ctx); ok {
		registry.ClearMCPToolCache()
	}
}

func (r *ToolGroupRegistry) ClearMCPToolCache() {
	if provider := r.currentMCPProvider(); provider != nil {
		provider.ClearCache()
	}
}

func GetMCPToolsByName(ctx context.Context, name string) ([]runtimeport.Tool, error) {
	registry, err := RequireRegistry(ctx)
	if err != nil {
		return nil, err
	}
	return registry.GetMCPToolsByName(ctx, name)
}

func (r *ToolGroupRegistry) GetMCPToolsByName(ctx context.Context, name string) ([]runtimeport.Tool, error) {
	provider := r.currentMCPProvider()
	if provider == nil {
		return nil, fmt.Errorf("MCP provider is not registered")
	}
	return provider.GetToolsByName(ctx, name)
}

func GetAllMCPToolGroups(ctx context.Context) (toolport.MCPToolGroups, error) {
	registry, err := RequireRegistry(ctx)
	if err != nil {
		return nil, err
	}
	return registry.GetAllMCPToolGroups(ctx)
}

func (r *ToolGroupRegistry) GetAllMCPToolGroups(ctx context.Context) (toolport.MCPToolGroups, error) {
	provider := r.currentMCPProvider()
	if provider == nil {
		return nil, fmt.Errorf("MCP provider is not registered")
	}
	return provider.GetAllToolGroups(ctx)
}

func (r *ToolGroupRegistry) currentMCPProvider() toolport.MCPProvider {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.mcpProvider
}
