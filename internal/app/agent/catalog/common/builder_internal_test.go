package common

import (
	"context"
	"testing"

	apptools "fkteams/internal/app/tools"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/resources"
	"fkteams/internal/testmodel"
)

func TestAgentBuilderBuildDoesNotMutateResolvedTools(t *testing.T) {
	ctx := context.Background()
	const toolGroupName = "builder_tools_test_group"
	if err := apptools.RegisterToolGroup(apptools.ToolGroupRegistration{
		Info: apptools.ToolGroupInfo{
			Name:        toolGroupName,
			DisplayName: "Builder Test",
			Description: "Builder test tools",
			Category:    "Test",
			Builtin:     true,
		},
		Factory: func(*resources.Cleaner) ([]runtimeport.Tool, error) {
			return []runtimeport.Tool{registryTestTool{}}, nil
		},
	}); err != nil {
		t.Fatalf("register test tool group: %v", err)
	}
	builder := NewAgentBuilder("builder_tools_test", "builder tools test agent").
		WithModel(testmodel.New()).
		WithInstruction("test").
		WithToolNames(toolGroupName)

	if _, err := builder.Build(ctx); err != nil {
		t.Fatalf("first build: %v", err)
	}
	if len(builder.tools) != 0 {
		t.Fatalf("expected builder tools to remain empty after first build, got %d", len(builder.tools))
	}

	if _, err := builder.Build(ctx); err != nil {
		t.Fatalf("second build: %v", err)
	}
	if len(builder.tools) != 0 {
		t.Fatalf("expected builder tools to remain empty after second build, got %d", len(builder.tools))
	}
}

func TestRuntimeOptionalCapabilitiesAreNotRequired(t *testing.T) {
	engine := minimalEngine{}
	model := testmodel.New()

	decorated, err := decorateChatModel(context.Background(), engine, model)
	if err != nil {
		t.Fatalf("decorate model: %v", err)
	}
	if decorated != model {
		t.Fatal("model should be returned unchanged when runtime has no decorator")
	}

	if middlewares := defaultToolMiddlewares(engine); len(middlewares) != 0 {
		t.Fatalf("tool middlewares = %d, want 0", len(middlewares))
	}
}

type minimalEngine struct{}

type registryTestTool struct{}

func (registryTestTool) Info(context.Context) (*runtimeport.ToolInfo, error) {
	return &runtimeport.ToolInfo{Name: "builder_test_tool"}, nil
}

func (registryTestTool) Invoke(context.Context, runtimeport.ToolInvocation) (*runtimeport.ToolResult, error) {
	return &runtimeport.ToolResult{}, nil
}

func (minimalEngine) NewChatModelAgent(context.Context, *runtimeport.ChatAgentConfig) (runtimeport.Agent, error) {
	return nil, nil
}

func (minimalEngine) NewLoopAgent(context.Context, *runtimeport.LoopAgentConfig) (runtimeport.Agent, error) {
	return nil, nil
}

func (minimalEngine) NewDeepAgent(context.Context, *runtimeport.DeepAgentConfig) (runtimeport.Agent, error) {
	return nil, nil
}

func (minimalEngine) NewRunner(context.Context, runtimeport.RunnerConfig) (runtimeport.Runner, error) {
	return nil, nil
}

func (minimalEngine) NewAgentTools(context.Context, []runtimeport.Agent, runtimeport.AgentToolConfig) ([]runtimeport.Tool, error) {
	return nil, nil
}
