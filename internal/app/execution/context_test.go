package execution

import (
	"context"
	appprocess "fkteams/internal/app/process"
	"testing"

	agents "fkteams/internal/app/agent/catalog"
	"fkteams/internal/app/agent/catalog/toolmeta"
	apptools "fkteams/internal/app/tools"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/hooks"
	modelregistry "fkteams/internal/runtime/model"
)

type runtimeIdentity struct{ runtimeport.Runtime }
type interruptIdentity struct{ runtimeport.InterruptRuntime }
type marker struct{}

func newTestDependencies() Dependencies {
	return Dependencies{
		Processes: &appprocess.Service{},
		Runtime:   &runtimeIdentity{}, Interrupt: &interruptIdentity{},
		ModelRegistry: modelregistry.NewRegistry(),
		ToolRegistry:  apptools.NewToolGroupRegistry(),
		ToolDisplays:  toolmeta.NewRegistry(),
		AgentRegistry: agents.NewRegistry(),
		HookBus:       hooks.NewBus(),
	}
}

func TestDependenciesRebindAllCapabilitiesWithoutCapturingCancellation(t *testing.T) {
	first, second := newTestDependencies(), newTestDependencies()
	parent, cancel := context.WithCancel(context.WithValue(context.Background(), marker{}, "parent"))
	bound := first.Context(parent)
	if bound.Value(marker{}) != "parent" || FromContext(bound) != first {
		t.Fatal("binding lost a parent value or execution capability")
	}
	captured := FromContext(bound)
	cancel()
	if bound.Err() != context.Canceled {
		t.Fatal("binding lost parent cancellation")
	}
	rebound := captured.Context(context.Background())
	if rebound.Err() != nil || rebound.Value(marker{}) != nil || FromContext(rebound) != first {
		t.Fatal("captured dependencies leaked request state or lost capabilities")
	}
	if got := FromContext(second.Context(rebound)); got != second {
		t.Fatalf("application instances were mixed: %+v", got)
	}
	if FromContext(bound) != first {
		t.Fatal("rebinding changed the original application scope")
	}
}

func TestDependenciesHandleEmptyContexts(t *testing.T) {
	if got := FromContext(nil); got != (Dependencies{}) {
		t.Fatalf("nil context = %+v", got)
	}
	if ctx := (Dependencies{}).Context(nil); ctx == nil || FromContext(ctx) != (Dependencies{}) {
		t.Fatal("empty dependencies did not produce a usable context")
	}
}
