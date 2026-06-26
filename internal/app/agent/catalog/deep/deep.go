package deep

import (
	"context"
	"fkteams/internal/app/agent/catalog/common"
	"fkteams/internal/app/tools"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/env"
	retry "fkteams/internal/runtime/retry"
	"fkteams/internal/runtime/toolpolicy"
	"fmt"
	"strconv"
)

func NewAgent(ctx context.Context, subAgents []runtimeport.Agent) (runtimeport.Agent, error) {

	toolList, err := tools.GetBuiltinCapabilityTools()
	if err != nil {
		return nil, err
	}
	toolNames := []string{"file", "doc", "command", "search", "fetch"}
	for _, toolName := range toolNames {
		baseTools, err := tools.GetToolsByName(toolName)
		if err != nil {
			return nil, fmt.Errorf("init tool %s: %w", toolName, err)
		}
		if err := toolpolicy.MarkPolicyRequired(baseTools); err != nil {
			return nil, fmt.Errorf("mark tool policy %s: %w", toolName, err)
		}
		toolList = append(toolList, baseTools...)
	}
	if err := toolpolicy.ClassifyTools(toolList); err != nil {
		return nil, fmt.Errorf("classify tools: %w", err)
	}
	chatModel, err := common.NewChatModel()
	if err != nil {
		return nil, fmt.Errorf("create chat model: %w", err)
	}

	engine, err := runtimeport.RequireEngine(ctx)
	if err != nil {
		return nil, err
	}
	middlewareProvider, ok := engine.(runtimeport.AgentPipelineProvider)
	if !ok {
		return nil, fmt.Errorf("runtime does not support deep agent middlewares")
	}
	maxTokens := runtimeport.DefaultMaxTokensBeforeSummary
	if v := env.Get(env.MaxTokensBeforeSummary); v != "" {
		if n, _ := strconv.Atoi(v); n > 0 {
			maxTokens = n
		}
	}
	summaryMiddleware, err := middlewareProvider.NewSummaryMiddleware(ctx, &runtimeport.SummaryConfig{
		Model:                  chatModel,
		MaxTokensBeforeSummary: maxTokens,
	})
	if err != nil {
		return nil, fmt.Errorf("init summary middleware: %w", err)
	}
	agentsMDMiddleware, err := middlewareProvider.NewAgentsMDMiddleware(ctx)
	if err != nil {
		return nil, fmt.Errorf("init agents.md middleware: %w", err)
	}
	return engine.NewDeepAgent(ctx, &runtimeport.DeepAgentConfig{
		Name:             "deep_researcher",
		Description:      "深度研究智能体，负责深入分析问题并协调多个成员解决复杂任务。",
		Model:            chatModel,
		ModelRetryConfig: retry.NewModelRetryConfig(),
		SubAgents:        subAgents,
		Tools:            toolList,
		MaxIterations:    retry.MaxIterations(),
		Middlewares: []runtimeport.AgentMiddleware{
			middlewareProvider.NewSteeringMiddleware(),
			summaryMiddleware,
			agentsMDMiddleware,
		},
	})
}
