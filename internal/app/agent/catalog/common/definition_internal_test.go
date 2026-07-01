package common

import (
	"context"
	"testing"

	apptools "fkteams/internal/app/tools"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/resources"
	"fkteams/internal/testmodel"
)

func TestDefinitionBuildDoesNotMutateResolvedTools(t *testing.T) {
	runtime := &minimalEngine{}
	ctx := runtimeport.WithRuntime(context.Background(), runtime)
	registry := apptools.NewToolGroupRegistry()
	ctx = apptools.WithRegistry(ctx, registry)
	const toolGroupName = "builder_tools_test_group"
	if err := registry.Register(apptools.ToolGroupRegistration{
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
	def := Definition{
		Name:        "builder_tools_test",
		Description: "builder tools test agent",
		Instruction: "test",
		Profile:     ProfileBare,
		Model:       testmodel.New(),
		ToolNames:   []string{toolGroupName},
	}

	if _, err := BuildAgent(ctx, def); err != nil {
		t.Fatalf("first build: %v", err)
	}
	if len(def.Tools) != 0 {
		t.Fatalf("expected definition tools to remain empty after first build, got %d", len(def.Tools))
	}

	if _, err := BuildAgent(ctx, def); err != nil {
		t.Fatalf("second build: %v", err)
	}
	if len(def.Tools) != 0 {
		t.Fatalf("expected definition tools to remain empty after second build, got %d", len(def.Tools))
	}
}

func TestRuntimeOptionalCapabilitiesAreNotRequired(t *testing.T) {
	model := testmodel.New()

	decorated, err := decorateChatModel(context.Background(), nil, model)
	if err != nil {
		t.Fatalf("decorate model: %v", err)
	}
	if decorated != model {
		t.Fatal("model should be returned unchanged when runtime has no decorator")
	}

	if middlewares := defaultToolMiddlewares(nil, false); len(middlewares) != 0 {
		t.Fatalf("tool middlewares = %d, want 0", len(middlewares))
	}
}

func TestBareProfileBuildsStandaloneAgent(t *testing.T) {
	runtime := &minimalEngine{}
	ctx := runtimeport.WithRuntime(context.Background(), runtime)
	tool, err := runtimeport.InferTool("standalone_echo", "echo", func(context.Context, *struct {
		Text string `json:"text"`
	}) (string, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("create tool: %v", err)
	}

	if _, err := BuildAgent(ctx, Definition{
		Name:        "standalone",
		Description: "standalone agent",
		Instruction: "standalone",
		Profile:     ProfileBare,
		Model:       testmodel.New(),
		Tools:       []runtimeport.Tool{tool},
	}); err != nil {
		t.Fatalf("build standalone: %v", err)
	}
	if runtime.lastConfig == nil {
		t.Fatal("runtime did not receive chat agent config")
	}
	if len(runtime.lastConfig.Tools) != 1 {
		t.Fatalf("tools = %d, want exactly explicit tool", len(runtime.lastConfig.Tools))
	}
	if len(runtime.lastConfig.Middlewares) != 0 {
		t.Fatalf("middlewares = %d, want none for bare profile", len(runtime.lastConfig.Middlewares))
	}
	if len(runtime.lastConfig.ToolMiddlewares) != 0 {
		t.Fatalf("tool middlewares = %d, want none for bare profile", len(runtime.lastConfig.ToolMiddlewares))
	}
}

type minimalEngine struct {
	lastConfig *runtimeport.ChatAgentConfig
}

type registryTestTool struct{}

func (registryTestTool) Info(context.Context) (*runtimeport.ToolInfo, error) {
	return &runtimeport.ToolInfo{Name: "builder_test_tool"}, nil
}

func (registryTestTool) Invoke(context.Context, runtimeport.ToolInvocation) (*runtimeport.ToolResult, error) {
	return &runtimeport.ToolResult{}, nil
}

func (e *minimalEngine) NewChatModelAgent(_ context.Context, cfg *runtimeport.ChatAgentConfig) (runtimeport.Agent, error) {
	copied := *cfg
	copied.Tools = append([]runtimeport.Tool(nil), cfg.Tools...)
	copied.Middlewares = append([]runtimeport.AgentMiddleware(nil), cfg.Middlewares...)
	copied.ToolMiddlewares = append([]runtimeport.ToolMiddleware(nil), cfg.ToolMiddlewares...)
	e.lastConfig = &copied
	return minimalAgent{name: cfg.Name, description: cfg.Description}, nil
}

func (*minimalEngine) NewLoopAgent(context.Context, *runtimeport.LoopAgentConfig) (runtimeport.Agent, error) {
	return nil, nil
}

func (*minimalEngine) NewDeepAgent(context.Context, *runtimeport.DeepAgentConfig) (runtimeport.Agent, error) {
	return nil, nil
}

func (*minimalEngine) NewRunner(context.Context, runtimeport.RunnerConfig) (runtimeport.Runner, error) {
	return nil, nil
}

func (*minimalEngine) NewAgentTools(context.Context, []runtimeport.Agent, runtimeport.AgentToolConfig) ([]runtimeport.Tool, error) {
	return nil, nil
}

type minimalAgent struct {
	name        string
	description string
}

func (a minimalAgent) Name() string {
	return a.name
}

func (a minimalAgent) Description() string {
	return a.description
}
