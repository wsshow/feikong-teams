package runtime

import (
	"context"
	"testing"

	"fkteams/agentcore"
)

func TestRegisterAndUseRuntime(t *testing.T) {
	original := DefaultName()
	t.Cleanup(func() {
		if err := Use(original); err != nil {
			t.Fatalf("restore runtime: %v", err)
		}
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

func TestUseUnknownRuntimeReturnsError(t *testing.T) {
	if err := Use("missing-runtime"); err == nil {
		t.Fatal("expected error for missing runtime")
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
