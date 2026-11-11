package bootstrap

import (
	"context"
	"testing"

	modelproviders "fkteams/internal/adapters/model/providers"
	agents "fkteams/internal/app/agent/catalog"
	"fkteams/internal/app/agent/catalog/toolmeta"
	apptools "fkteams/internal/app/tools"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/hooks"
	modelregistry "fkteams/internal/runtime/model"
)

type contextMarker struct{}

func TestExecutionDependenciesContextBindsDefaults(t *testing.T) {
	dependencies, err := NewExecutionDependencies()
	if err != nil {
		t.Fatalf("NewExecutionDependencies() error = %v", err)
	}

	parent := context.WithValue(context.Background(), contextMarker{}, "preserved")
	ctx := dependencies.Context(parent)
	if got := ctx.Value(contextMarker{}); got != "preserved" {
		t.Fatalf("parent context value = %v, want preserved", got)
	}

	runtimeAdapter, ok := runtimeport.RuntimeFromContext(ctx)
	assertSameDependency(t, "runtime", runtimeAdapter, ok, dependencies.Runtime)
	interrupt, ok := runtimeport.InterruptRuntimeFromContext(ctx)
	assertSameDependency(t, "interrupt runtime", interrupt, ok, dependencies.Interrupt)
	models, ok := modelregistry.RegistryFromContext(ctx)
	assertSameDependency(t, "model registry", models, ok, dependencies.ModelRegistry)
	modelProviders, ok := modelproviders.RegistryFromContext(ctx)
	assertSameDependency(t, "model provider registry", modelProviders, ok, dependencies.ModelProviderRegistry)
	tools, ok := apptools.RegistryFromContext(ctx)
	assertSameDependency(t, "tool registry", tools, ok, dependencies.ToolRegistry)
	toolDisplays, ok := toolmeta.RegistryFromContext(ctx)
	assertSameDependency(t, "tool display registry", toolDisplays, ok, dependencies.ToolDisplays)
	agentRegistry, ok := agents.RegistryFromContext(ctx)
	assertSameDependency(t, "agent registry", agentRegistry, ok, dependencies.AgentRegistry)
	if hookBus := hooks.FromContext(ctx); hookBus != dependencies.HookBus {
		t.Fatalf("hook bus = %p, want %p", hookBus, dependencies.HookBus)
	}
}

func TestExecutionDependenciesContextHandlesNil(t *testing.T) {
	var dependencies *ExecutionDependencies
	if ctx := dependencies.Context(context.Background()); ctx == nil {
		t.Fatal("Context() returned nil")
	}
}

func assertSameDependency[T comparable](t *testing.T, name string, got T, ok bool, want T) {
	t.Helper()
	if !ok {
		t.Fatalf("%s is not bound", name)
	}
	if got != want {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}
