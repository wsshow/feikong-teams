package deep

import (
	"context"
	"fkteams/agentcore"
	agentruntime "fkteams/agentcore/runtime"
	"fkteams/agents/common"
	rootcommon "fkteams/common"
	"fkteams/fkenv"
	"fkteams/tools"
	"fmt"
	"strconv"
)

func NewAgent(ctx context.Context, subAgents []agentcore.Agent) (agentcore.Agent, error) {

	toolNames := []string{"file", "doc", "command", "search", "fetch"}
	var toolList []agentcore.Tool
	for _, toolName := range toolNames {
		baseTools, err := tools.GetToolsByName(toolName)
		if err != nil {
			return nil, fmt.Errorf("init tool %s: %w", toolName, err)
		}
		toolList = append(toolList, baseTools...)
	}
	chatModel, err := common.NewChatModel()
	if err != nil {
		return nil, fmt.Errorf("create chat model: %w", err)
	}

	engine := agentruntime.Engine()
	middlewareProvider, ok := engine.(agentcore.AgentPipelineProvider)
	if !ok {
		return nil, fmt.Errorf("runtime does not support deep agent middlewares")
	}
	maxTokens := agentcore.DefaultMaxTokensBeforeSummary
	if v := fkenv.Get(fkenv.MaxTokensBeforeSummary); v != "" {
		if n, _ := strconv.Atoi(v); n > 0 {
			maxTokens = n
		}
	}
	summaryMiddleware, err := middlewareProvider.NewSummaryMiddleware(ctx, &agentcore.SummaryConfig{
		Model:                  chatModel,
		MaxTokensBeforeSummary: maxTokens,
	})
	if err != nil {
		return nil, fmt.Errorf("init summary middleware: %w", err)
	}
	return engine.NewDeepAgent(ctx, &agentcore.DeepAgentConfig{
		Name:             "deep_researcher",
		Description:      "深度研究智能体，负责深入分析问题并协调多个成员解决复杂任务。",
		Model:            chatModel,
		ModelRetryConfig: rootcommon.NewModelRetryConfig(),
		SubAgents:        subAgents,
		Tools:            toolList,
		MaxIterations:    common.MaxIterations(),
		Middlewares: []agentcore.AgentMiddleware{
			middlewareProvider.NewSteeringMiddleware(),
			summaryMiddleware,
		},
	})
}
