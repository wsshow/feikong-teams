package engine

import (
	"context"
	"fmt"

	"fkteams/agentcore"
	einoruntime "fkteams/agentcore/eino"
	"fkteams/agentcore/eino/middlewares/agentsmd"
	"fkteams/agentcore/eino/middlewares/autocontinue"
	"fkteams/agentcore/eino/middlewares/dispatch"
	"fkteams/agentcore/eino/middlewares/inject"
	"fkteams/agentcore/eino/middlewares/skills"
	"fkteams/agentcore/eino/middlewares/steering"
	"fkteams/agentcore/eino/middlewares/summary"
	"fkteams/agentcore/eino/middlewares/tools/destructiveguard"
	hooktools "fkteams/agentcore/eino/middlewares/tools/hooks"
	"fkteams/agentcore/eino/middlewares/tools/patch"
	"fkteams/agentcore/eino/middlewares/tools/trimresult"
	"fkteams/agentcore/eino/middlewares/tools/warperror"

	einoMCP "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/mark3labs/mcp-go/client"
)

type Engine struct{}

func NewEngine() *Engine {
	return &Engine{}
}

func (e *Engine) RuntimeInfo() agentcore.RuntimeInfo {
	return agentcore.RuntimeInfo{
		Name:        "eino",
		Description: "CloudWeGo Eino ADK runtime adapter",
		Capabilities: []string{
			"chat_agent",
			"loop_agent",
			"deep_agent",
			"agent_tools",
			"streaming_runner",
			"agent_middlewares",
			"tool_middlewares",
			"mcp_tools",
		},
	}
}

func (e *Engine) CheckHealth(ctx context.Context) agentcore.RuntimeHealth {
	return agentcore.RuntimeHealth{
		Name:  e.RuntimeInfo().Name,
		Ready: ctx.Err() == nil,
	}
}

func (e *Engine) NewChatModelAgent(ctx context.Context, cfg *agentcore.ChatAgentConfig) (agentcore.Agent, error) {
	return einoruntime.NewChatModelAgent(ctx, cfg)
}

func (e *Engine) NewLoopAgent(ctx context.Context, cfg *agentcore.LoopAgentConfig) (agentcore.Agent, error) {
	return einoruntime.NewLoopAgent(ctx, cfg)
}

func (e *Engine) NewDeepAgent(ctx context.Context, cfg *agentcore.DeepAgentConfig) (agentcore.Agent, error) {
	return einoruntime.NewDeepAgent(ctx, cfg)
}

func (e *Engine) NewRunner(ctx context.Context, cfg agentcore.RunnerConfig) (agentcore.Runner, error) {
	return einoruntime.NewRunnerFromConfig(ctx, cfg)
}

func (e *Engine) NewAgentTools(ctx context.Context, subAgents []agentcore.Agent, cfg agentcore.AgentToolConfig) ([]agentcore.Tool, error) {
	return einoruntime.NewAgentTools(ctx, subAgents, cfg)
}

func (e *Engine) DecorateChatModel(ctx context.Context, chatModel agentcore.ChatModel) (agentcore.ChatModel, error) {
	return inject.NewForModel(chatModel)
}

func (e *Engine) DefaultAgentMiddlewares(ctx context.Context) ([]agentcore.AgentMiddleware, error) {
	result := make([]agentcore.AgentMiddleware, 0, 5)
	patchMiddleware, err := e.newPatchMiddleware(ctx)
	if err != nil {
		return nil, err
	}
	result = append(result, patchMiddleware)
	result = append(result, e.newToolErrorMiddleware())
	acMiddleware, err := e.newAutoContinueMiddleware()
	if err != nil {
		return nil, err
	}
	result = append(result, acMiddleware)
	result = append(result, e.newTrimResultMiddleware())
	result = append(result, e.NewSteeringMiddleware())
	return result, nil
}

func (e *Engine) DefaultToolMiddlewares() []agentcore.ToolMiddleware {
	return []agentcore.ToolMiddleware{
		e.newHookToolMiddleware(),
		e.newDestructiveGuardMiddleware(),
	}
}

func (e *Engine) newPatchMiddleware(ctx context.Context) (agentcore.AgentMiddleware, error) {
	return patch.New(ctx)
}

func (e *Engine) newToolErrorMiddleware() agentcore.AgentMiddleware {
	return warperror.NewHandler(nil)
}

func (e *Engine) newAutoContinueMiddleware() (agentcore.AgentMiddleware, error) {
	return autocontinue.NewHandler()
}

func (e *Engine) newTrimResultMiddleware() agentcore.AgentMiddleware {
	return trimresult.New(nil)
}

func (e *Engine) NewSteeringMiddleware() agentcore.AgentMiddleware {
	return steering.New()
}

func (e *Engine) NewSummaryMiddleware(ctx context.Context, cfg *agentcore.SummaryConfig) (agentcore.AgentMiddleware, error) {
	if cfg == nil {
		return summary.New(ctx, nil)
	}
	return summary.New(ctx, &summary.Config{
		Model:                  cfg.Model,
		MaxTokensBeforeSummary: cfg.MaxTokensBeforeSummary,
	})
}

func (e *Engine) NewSkillsMiddleware(ctx context.Context) (agentcore.AgentMiddleware, error) {
	return skills.New(ctx)
}

func (e *Engine) NewDispatchMiddleware(ctx context.Context, cfg *agentcore.DispatchConfig) (agentcore.AgentMiddleware, error) {
	if cfg == nil {
		return dispatch.New(ctx, &dispatch.Config{})
	}
	return dispatch.New(ctx, &dispatch.Config{
		Model:          cfg.Model,
		ToolNames:      cfg.ToolNames,
		Tools:          cfg.Tools,
		MaxConcurrency: cfg.MaxConcurrency,
		TaskTimeout:    cfg.TaskTimeout,
	})
}

func (e *Engine) NewAgentsMDMiddleware(ctx context.Context) (agentcore.AgentMiddleware, error) {
	return agentsmd.New(ctx)
}

func (e *Engine) newDestructiveGuardMiddleware() agentcore.ToolMiddleware {
	return destructiveguard.New()
}

func (e *Engine) newHookToolMiddleware() agentcore.ToolMiddleware {
	return hooktools.New()
}

func (e *Engine) MCPTools(ctx context.Context, rawClient any) ([]agentcore.Tool, error) {
	cli, ok := rawClient.(*client.Client)
	if !ok {
		return nil, fmt.Errorf("unsupported MCP client: %T", rawClient)
	}
	tools, err := einoMCP.GetTools(ctx, &einoMCP.Config{Cli: cli})
	if err != nil {
		return nil, err
	}
	result := make([]agentcore.Tool, 0, len(tools))
	for _, t := range tools {
		result = append(result, einoruntime.WrapTool(t))
	}
	return result, nil
}
