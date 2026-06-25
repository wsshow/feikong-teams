package mcp

import (
	"context"
	"strings"
	"testing"

	"fkteams/internal/app/config"
	runtimeport "fkteams/internal/ports/runtime"
)

type fakeTool struct {
	name string
}

func (f fakeTool) Info(ctx context.Context) (*runtimeport.ToolInfo, error) {
	return &runtimeport.ToolInfo{Name: f.name}, nil
}

func (f fakeTool) Invoke(ctx context.Context, invocation runtimeport.ToolInvocation) (*runtimeport.ToolResult, error) {
	return &runtimeport.ToolResult{Content: f.name}, nil
}

func TestGetToolsByNameUsesCache(t *testing.T) {
	resetMCPGlobals(t)
	cachedTools = DictToolGroup{
		"demo": {
			Name:  "demo",
			Desc:  "Demo tools",
			Tools: []runtimeport.Tool{fakeTool{name: "demo_tool"}},
		},
	}

	tools, err := GetToolsByName("demo")
	if err != nil {
		t.Fatalf("GetToolsByName() error = %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("tool count = %d, want 1", len(tools))
	}
	info, err := tools[0].Info(context.Background())
	if err != nil {
		t.Fatalf("tool info: %v", err)
	}
	if info.Name != "demo_tool" {
		t.Fatalf("tool name = %q, want demo_tool", info.Name)
	}

	if _, err := GetToolsByName("missing"); err == nil || !strings.Contains(err.Error(), "MCP tool missing not found") {
		t.Fatalf("missing tool error = %v", err)
	}
}

func TestGetAllToolGroupsReturnsCachedMap(t *testing.T) {
	resetMCPGlobals(t)
	cachedTools = DictToolGroup{
		"demo": {Name: "demo"},
	}

	groups, err := GetAllToolGroups()
	if err != nil {
		t.Fatalf("GetAllToolGroups() error = %v", err)
	}
	if len(groups) != 1 || groups["demo"].Name != "demo" {
		t.Fatalf("groups = %#v, want demo", groups)
	}

	ClearCache()
	if cachedTools != nil {
		t.Fatalf("cachedTools = %#v, want nil", cachedTools)
	}
}

func TestGetAllMCPToolsWithNoEnabledServers(t *testing.T) {
	resetMCPGlobals(t)
	t.Setenv("FEIKONG_APP_DIR", t.TempDir())
	if err := config.Save(&config.Config{
		Custom: config.Custom{
			MCPServers: []config.MCPServer{
				{Name: "disabled", Enabled: false, TransportType: "stdio"},
			},
		},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	groups, err := getAllMCPTools()
	if err != nil {
		t.Fatalf("getAllMCPTools() error = %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("groups = %#v, want empty", groups)
	}
}

func TestSetupMCPClientsRejectsUnsupportedTransport(t *testing.T) {
	resetMCPGlobals(t)
	t.Setenv("FEIKONG_APP_DIR", t.TempDir())
	if err := config.Save(&config.Config{
		Custom: config.Custom{
			MCPServers: []config.MCPServer{
				{Name: "bad", Enabled: true, TransportType: "pipe"},
			},
		},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	clients, err := setupMCPClients(context.Background())
	if err == nil {
		t.Fatalf("setupMCPClients() = %#v, want unsupported transport error", clients)
	}
	if !strings.Contains(err.Error(), "unsupported MCP transport type") {
		t.Fatalf("setupMCPClients() error = %v", err)
	}
}

func TestRegisterToolProviderAndClearCache(t *testing.T) {
	resetMCPGlobals(t)
	provider := func(ctx context.Context, rawClient any) ([]runtimeport.Tool, error) {
		return []runtimeport.Tool{fakeTool{name: "provided"}}, nil
	}
	RegisterToolProvider(provider)
	if toolProvider == nil {
		t.Fatal("tool provider was not registered")
	}
	cachedTools = DictToolGroup{
		"cached": {Name: "cached"},
	}
	ClearCache()
	if cachedTools != nil {
		t.Fatal("ClearCache should clear cached tools")
	}
}

func resetMCPGlobals(t *testing.T) {
	t.Helper()

	originalTools := cachedTools
	originalProvider := toolProvider
	cachedTools = nil
	toolProvider = nil
	t.Cleanup(func() {
		cachedTools = originalTools
		toolProvider = originalProvider
	})
}
