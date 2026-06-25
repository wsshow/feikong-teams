package tools

import (
	"context"
	"slices"
	"testing"

	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/resources"
)

type registryTestTool struct{}

func (registryTestTool) Info(context.Context) (*runtimeport.ToolInfo, error) {
	return &runtimeport.ToolInfo{Name: "registry_test_tool"}, nil
}

func (registryTestTool) Invoke(context.Context, runtimeport.ToolInvocation) (*runtimeport.ToolResult, error) {
	return &runtimeport.ToolResult{}, nil
}

func TestBuiltinToolRegistryMatchesCatalog(t *testing.T) {
	registered := BuiltinToolNames()
	if len(registered) != len(builtinToolCatalog) {
		t.Fatalf("registered tool groups = %d, catalog = %d", len(registered), len(builtinToolCatalog))
	}
	for _, info := range builtinToolCatalog {
		if !slices.Contains(registered, info.Name) {
			t.Fatalf("builtin registry missing catalog group %s", info.Name)
		}
	}
}

func TestToolGroupRegistryRejectsDuplicateAndResolves(t *testing.T) {
	registry := NewToolGroupRegistry()
	reg := ToolGroupRegistration{
		Name: "demo",
		Factory: func(cleaner *resources.Cleaner) ([]runtimeport.Tool, error) {
			return []runtimeport.Tool{registryTestTool{}}, nil
		},
	}
	if err := registry.Register(reg); err != nil {
		t.Fatalf("register demo: %v", err)
	}
	if err := registry.Register(reg); err == nil {
		t.Fatal("expected duplicate registration error")
	}
	resolved, ok, err := registry.Resolve("demo", nil)
	if err != nil {
		t.Fatalf("resolve demo: %v", err)
	}
	if !ok || len(resolved) != 1 {
		t.Fatalf("resolve demo = ok:%v tools:%d, want one tool", ok, len(resolved))
	}
}
