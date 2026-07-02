package tools

import (
	"context"
	"slices"
	"testing"

	runtimeport "fkteams/internal/ports/runtime"
)

type registryTestTool struct{}

func (registryTestTool) Info(context.Context) (*runtimeport.ToolInfo, error) {
	return &runtimeport.ToolInfo{Name: "registry_test_tool"}, nil
}

func (registryTestTool) Invoke(context.Context, runtimeport.ToolInvocation) (*runtimeport.ToolResult, error) {
	return &runtimeport.ToolResult{}, nil
}

func TestBuiltinToolRegistryMatchesCatalog(t *testing.T) {
	registry := newTestRegistry(t)
	ctx := WithRegistry(context.Background(), registry)
	registered := BuiltinToolNames(ctx)
	catalog := BuiltinToolInfos(ctx)
	if len(registered) != len(catalog) {
		t.Fatalf("registered tool groups = %d, catalog = %d", len(registered), len(catalog))
	}
	for _, info := range catalog {
		if !slices.Contains(registered, info.Name) {
			t.Fatalf("builtin registry missing catalog group %s", info.Name)
		}
	}
}

func newTestRegistry(t *testing.T) *ToolGroupRegistry {
	t.Helper()
	registry := NewToolGroupRegistry()
	err := registry.Register(ToolGroupRegistration{
		Info: ToolGroupInfo{
			Name:        "demo",
			DisplayName: "Demo",
			Description: "Demo tools",
			Category:    "Test",
			Builtin:     true,
		},
		Factory: func(ToolResolveContext) ([]runtimeport.Tool, error) {
			return []runtimeport.Tool{registryTestTool{}}, nil
		},
	})
	if err != nil {
		t.Fatalf("register demo: %v", err)
	}
	return registry
}

func TestToolGroupRegistryRejectsDuplicateAndResolves(t *testing.T) {
	registry := NewToolGroupRegistry()
	reg := ToolGroupRegistration{
		Info: ToolGroupInfo{
			Name:        "demo",
			DisplayName: "Demo",
			Description: "Demo tools",
			Category:    "Test",
			Builtin:     true,
		},
		Factory: func(ToolResolveContext) ([]runtimeport.Tool, error) {
			return []runtimeport.Tool{registryTestTool{}}, nil
		},
	}
	if err := registry.Register(reg); err != nil {
		t.Fatalf("register demo: %v", err)
	}
	if err := registry.Register(reg); err == nil {
		t.Fatal("expected duplicate registration error")
	}
	resolved, ok, err := registry.Resolve(context.Background(), "demo", nil)
	if err != nil {
		t.Fatalf("resolve demo: %v", err)
	}
	if !ok || len(resolved) != 1 {
		t.Fatalf("resolve demo = ok:%v tools:%d, want one tool", ok, len(resolved))
	}
}

func TestToolGroupRegistryResolveUsesContextPatch(t *testing.T) {
	registry := NewToolGroupRegistry(ToolResolveContext{Config: "base"})
	err := registry.Register(ToolGroupRegistration{
		Info: ToolGroupInfo{
			Name:        "demo",
			DisplayName: "Demo",
			Description: "Demo tools",
			Category:    "Test",
			Builtin:     true,
		},
		Factory: func(ctx ToolResolveContext) ([]runtimeport.Tool, error) {
			if ctx.Config != "agent" {
				t.Fatalf("config = %#v, want agent override", ctx.Config)
			}
			return []runtimeport.Tool{registryTestTool{}}, nil
		},
	})
	if err != nil {
		t.Fatalf("register demo: %v", err)
	}

	ctx := WithResolveContextPatch(context.Background(), ToolResolveContext{Config: "agent"})
	resolved, ok, err := registry.Resolve(ctx, "demo", nil)
	if err != nil {
		t.Fatalf("resolve demo: %v", err)
	}
	if !ok || len(resolved) != 1 {
		t.Fatalf("resolve demo = ok:%v tools:%d, want one tool", ok, len(resolved))
	}
}
