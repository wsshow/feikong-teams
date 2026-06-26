package tools

import (
	"context"
	"testing"

	runtimeport "fkteams/internal/ports/runtime"
	toolport "fkteams/internal/ports/tools"
)

type fakeMCPProvider struct {
	groups toolport.MCPToolGroups
}

func (p fakeMCPProvider) GetToolsByName(_ context.Context, groupName string) ([]runtimeport.Tool, error) {
	return p.groups[groupName].Tools, nil
}

func (p fakeMCPProvider) GetAllToolGroups(context.Context) (toolport.MCPToolGroups, error) {
	return p.groups, nil
}

func (p fakeMCPProvider) ClearCache() {}

func TestMCPProviderRegistrationResolvesDynamicTools(t *testing.T) {
	registry := NewToolGroupRegistry()
	registry.RegisterMCPProvider(fakeMCPProvider{
		groups: toolport.MCPToolGroups{
			"demo": {Name: "demo", Tools: []runtimeport.Tool{registryTestTool{}}},
		},
	})
	ctx := WithRegistry(context.Background(), registry)

	resolved, err := GetToolsByName(ctx, "mcp-demo")
	if err != nil {
		t.Fatalf("GetToolsByName returned error: %v", err)
	}
	if len(resolved) != 1 {
		t.Fatalf("tool count = %d, want 1", len(resolved))
	}
	names := GetAllToolNames(ctx)
	if !containsString(names, "mcp-demo") {
		t.Fatalf("tool names = %#v, want mcp-demo", names)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
