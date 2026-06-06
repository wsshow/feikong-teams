package eino

import (
	"context"
	"fkteams/agentcore"
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/deep"
	"github.com/cloudwego/eino/compose"
)

func NewDeepAgent(ctx context.Context, cfg *agentcore.DeepAgentConfig) (agentcore.Agent, error) {
	if cfg == nil {
		return nil, fmt.Errorf("deep agent config is nil")
	}
	chatModel, err := AdaptChatModelForRunner(cfg.Model)
	if err != nil {
		return nil, fmt.Errorf("adapt chat model: %w", err)
	}
	runnerTools, err := AdaptToolsForRunner(ctx, cfg.Tools)
	if err != nil {
		return nil, fmt.Errorf("adapt tools: %w", err)
	}
	runnerSubAgents, err := AdaptAgentsForRunner(cfg.SubAgents)
	if err != nil {
		return nil, fmt.Errorf("adapt sub agents: %w", err)
	}
	runnerHandlers, err := AdaptAgentMiddlewaresForRunner(cfg.Middlewares)
	if err != nil {
		return nil, fmt.Errorf("adapt middleware: %w", err)
	}

	agent, err := deep.New(ctx, &deep.Config{
		Name:             cfg.Name,
		Description:      cfg.Description,
		ChatModel:        chatModel,
		ModelRetryConfig: AdaptModelRetryConfigForRunner(cfg.ModelRetryConfig),
		SubAgents:        runnerSubAgents,
		MaxIteration:     cfg.MaxIterations,
		Handlers:         runnerHandlers,
		ToolsConfig: adk.ToolsConfig{
			EmitInternalEvents: cfg.EmitInternalEvents,
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: runnerTools,
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return WrapNamedAgent(cfg.Name, cfg.Description, agent), nil
}
