package runtime

import (
	"context"
	"sort"
	"testing"

	"fkteams/agentcore"
)

func TestRegisterAndUseRuntime(t *testing.T) {
	original := DefaultName()
	t.Cleanup(func() {
		registry.Lock()
		registry.defaultName = original
		registry.Unlock()
	})

	engine := testEngine{}
	Register("test-runtime", engine)
	if err := Use("test-runtime"); err != nil {
		t.Fatalf("use runtime: %v", err)
	}
	if DefaultName() != "test-runtime" {
		t.Fatalf("default runtime = %q, want test-runtime", DefaultName())
	}
	if got := Engine(); got != engine {
		t.Fatal("Engine did not return registered runtime")
	}
}

func TestEngineByNameRequiresExplicitRegistration(t *testing.T) {
	if _, err := EngineByName("missing-runtime"); err == nil {
		t.Fatal("expected missing runtime error")
	}
}

func TestUseUnknownRuntimeReturnsError(t *testing.T) {
	if err := Use("missing-runtime"); err == nil {
		t.Fatal("expected error for missing runtime")
	}
}

func TestRegisteredNamesAreSorted(t *testing.T) {
	Register("z-runtime", testEngine{})
	Register("a-runtime", testEngine{})

	names := RegisteredNames()
	if !sort.StringsAreSorted(names) {
		t.Fatalf("registered names are not sorted: %v", names)
	}
}

type testEngine struct{}

func (testEngine) NewChatModelAgent(context.Context, *agentcore.ChatAgentConfig) (agentcore.Agent, error) {
	return nil, nil
}

func (testEngine) NewLoopAgent(context.Context, *agentcore.LoopAgentConfig) (agentcore.Agent, error) {
	return nil, nil
}

func (testEngine) NewDeepAgent(context.Context, *agentcore.DeepAgentConfig) (agentcore.Agent, error) {
	return nil, nil
}

func (testEngine) NewRunner(context.Context, agentcore.RunnerConfig) (agentcore.Runner, error) {
	return nil, nil
}

func (testEngine) NewAgentTools(context.Context, []agentcore.Agent, agentcore.AgentToolConfig) ([]agentcore.Tool, error) {
	return nil, nil
}
