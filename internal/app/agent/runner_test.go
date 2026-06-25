package agent

import (
	"context"
	"errors"
	"fkteams/agents/custom"
	"fkteams/agents/toolmeta"
	"fkteams/config"
	domainmessage "fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	runtimeregistry "fkteams/internal/runtime/registry"
	"strings"
	"testing"
)

func TestAgentToolNameNormalizesAndDeduplicates(t *testing.T) {
	used := map[string]bool{}

	tests := []struct {
		name string
		want string
	}{
		{name: "Code Reviewer", want: toolmeta.AgentToolPrefix + "code_reviewer"},
		{name: "123", want: toolmeta.AgentToolPrefix + "member_2"},
		{name: "!!!", want: toolmeta.AgentToolPrefix + "member_3"},
		{name: "Code Reviewer", want: toolmeta.AgentToolPrefix + "code_reviewer_2"},
	}
	for i, tt := range tests {
		if got := agentToolName(tt.name, i, used); got != tt.want {
			t.Fatalf("agentToolName(%q, %d) = %q, want %q", tt.name, i, got, tt.want)
		}
	}
}

func TestBuildAgentToolsUsesRuntimeToolNameMapping(t *testing.T) {
	engine := &runnerTestEngine{}
	restoreRuntime(t, "runner-test-build-tools", engine)

	agents := []runtimeport.Agent{
		runnerTestAgent{name: "成员 A"},
		runnerTestAgent{name: "成员 A"},
	}
	tools, err := buildAgentTools(context.Background(), agents)
	if err != nil {
		t.Fatalf("buildAgentTools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("tools count = %d, want 2", len(tools))
	}

	names := engine.toolNames
	if len(names) != 2 {
		t.Fatalf("tool names = %v, want 2 names", names)
	}
	if names[0] != toolmeta.AgentToolPrefix+"a" || names[1] != toolmeta.AgentToolPrefix+"a_2" {
		t.Fatalf("tool names = %v, want normalized duplicate names", names)
	}
}

func TestCreateAgentRunnerUsesRuntimeRunnerConfig(t *testing.T) {
	engine := &runnerTestEngine{}
	restoreRuntime(t, "runner-test-create-agent", engine)

	agent := runnerTestAgent{name: "agent"}
	got, err := CreateAgentRunner(context.Background(), agent)
	if err != nil {
		t.Fatalf("CreateAgentRunner: %v", err)
	}
	if got != engine.runner {
		t.Fatal("CreateAgentRunner should return runtime runner")
	}
	if engine.runnerCfg.Agent != agent {
		t.Fatalf("runner agent = %#v, want injected agent", engine.runnerCfg.Agent)
	}
	if !engine.runnerCfg.EnableStreaming {
		t.Fatal("runner should enable streaming")
	}
	if engine.runnerCfg.CheckPointStore == nil {
		t.Fatal("runner should configure checkpoint store")
	}
}

func TestCreateAgentRunnerPropagatesRuntimeError(t *testing.T) {
	engine := &runnerTestEngine{runnerErr: errors.New("runner failed")}
	restoreRuntime(t, "runner-test-runner-error", engine)

	if _, err := CreateAgentRunner(context.Background(), runnerTestAgent{name: "agent"}); err == nil || !strings.Contains(err.Error(), "runner failed") {
		t.Fatalf("CreateAgentRunner error = %v, want runtime error", err)
	}
}

func TestResolveCustomModel(t *testing.T) {
	cfg := &config.Config{
		Models: []config.ModelConfig{
			{Name: "default", Provider: "openai", Model: "gpt-default", APIKey: "default-key", BaseURL: "https://default.example"},
			{Name: "fast", Provider: "deepseek", Model: "deepseek-chat", APIKey: "fast-key", BaseURL: "https://fast.example"},
		},
	}

	got := resolveCustomModel(cfg, config.CustomAgent{Model: "fast"})
	if got.Provider != "deepseek" || got.Name != "deepseek-chat" || got.APIKey != "fast-key" || got.BaseURL != "https://fast.example" {
		t.Fatalf("resolved custom model = %#v", got)
	}

	if missing := resolveCustomModel(cfg, config.CustomAgent{Model: "missing"}); missing != (custom.Model{}) {
		t.Fatalf("missing model = %#v, want zero value", missing)
	}
}

func TestCustomModeratorPromptUsesDefaultAndAppendsToolSection(t *testing.T) {
	got := customModeratorPrompt("")
	if !strings.Contains(got, "你是一个公正的主持人") {
		t.Fatalf("prompt = %q, want default prompt", got)
	}
	if !strings.Contains(got, "## 子智能体工具") {
		t.Fatalf("prompt = %q, want sub-agent tool section", got)
	}

	custom := customModeratorPrompt("自定义主持人")
	if !strings.HasPrefix(custom, "自定义主持人") || !strings.Contains(custom, "## 子智能体工具") {
		t.Fatalf("custom prompt = %q", custom)
	}
}

func restoreRuntime(t *testing.T, name string, engine runtimeport.Engine) {
	t.Helper()
	original := runtimeregistry.DefaultName()
	runtimeregistry.Register(name, engine)
	if err := runtimeregistry.Use(name); err != nil {
		t.Fatalf("use runtime: %v", err)
	}
	t.Cleanup(func() {
		if err := runtimeregistry.Use(original); err != nil {
			t.Fatalf("restore runtime: %v", err)
		}
	})
}

type runnerTestAgent struct {
	name string
}

func (a runnerTestAgent) Name() string        { return a.name }
func (a runnerTestAgent) Description() string { return a.name + " description" }

type runnerTestRunner struct{}

func (runnerTestRunner) Run(context.Context, domainmessage.TurnInput, runtimeport.RunOptions) (*runtimeport.RunResult, error) {
	return &runtimeport.RunResult{}, nil
}

type runnerTestTool struct {
	name string
}

func (t runnerTestTool) Info(context.Context) (*runtimeport.ToolInfo, error) {
	return &runtimeport.ToolInfo{Name: t.name}, nil
}

func (t runnerTestTool) Invoke(context.Context, runtimeport.ToolInvocation) (*runtimeport.ToolResult, error) {
	return &runtimeport.ToolResult{}, nil
}

type runnerTestEngine struct {
	runner    runtimeport.Runner
	runnerCfg runtimeport.RunnerConfig
	runnerErr error
	toolNames []string
}

func (e *runnerTestEngine) NewChatModelAgent(context.Context, *runtimeport.ChatAgentConfig) (runtimeport.Agent, error) {
	return nil, nil
}

func (e *runnerTestEngine) NewLoopAgent(context.Context, *runtimeport.LoopAgentConfig) (runtimeport.Agent, error) {
	return nil, nil
}

func (e *runnerTestEngine) NewDeepAgent(context.Context, *runtimeport.DeepAgentConfig) (runtimeport.Agent, error) {
	return nil, nil
}

func (e *runnerTestEngine) NewRunner(_ context.Context, cfg runtimeport.RunnerConfig) (runtimeport.Runner, error) {
	e.runnerCfg = cfg
	if e.runnerErr != nil {
		return nil, e.runnerErr
	}
	if e.runner == nil {
		e.runner = runnerTestRunner{}
	}
	return e.runner, nil
}

func (e *runnerTestEngine) NewAgentTools(_ context.Context, subAgents []runtimeport.Agent, cfg runtimeport.AgentToolConfig) ([]runtimeport.Tool, error) {
	tools := make([]runtimeport.Tool, 0, len(subAgents))
	for i, agent := range subAgents {
		name := cfg.ToolName(agent.Name(), i)
		e.toolNames = append(e.toolNames, name)
		if cfg.RegisterDisplay != nil {
			cfg.RegisterDisplay(name, agent.Name())
		}
		tools = append(tools, runnerTestTool{name: name})
	}
	return tools, nil
}
