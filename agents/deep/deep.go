package deep

import (
	"context"
	"fkteams/agents/common"
	"fkteams/agents/middlewares/summary"
	"fkteams/agents/retry"
	rootcommon "fkteams/common"
	"fkteams/fkenv"
	"fkteams/tools"
	"fmt"
	"strconv"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/deep"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
)

func NewAgent(ctx context.Context, subAgents []adk.Agent) (adk.Agent, error) {

	toolNames := []string{"file", "doc", "command", "search", "fetch"}
	var toolList []tool.BaseTool
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

	retryModel := retry.NewRetryChatModel(chatModel, &retry.ModelRetryConfig{
		MaxRetries:  rootcommon.MaxRetries,
		IsRetryAble: rootcommon.IsRetryAble,
	})

	maxTokens := summary.DefaultMaxTokensBeforeSummary
	if v := fkenv.Get(fkenv.MaxTokensBeforeSummary); v != "" {
		if n, _ := strconv.Atoi(v); n > 0 {
			maxTokens = n
		}
	}
	summaryMiddleware, err := summary.New(ctx, &summary.Config{
		Model:                  retryModel,
		SystemPrompt:           summary.PromptOfSummary,
		MaxTokensBeforeSummary: maxTokens,
	})
	if err != nil {
		return nil, fmt.Errorf("init summary middleware: %w", err)
	}

	return deep.New(ctx, &deep.Config{
		Name:         "深度探索者",
		Description:  "一个能够深入分析问题并协调多个智能体协作解决复杂任务的智能体。",
		ChatModel:    retryModel,
		SubAgents:    subAgents,
		MaxIteration: common.MaxIterations(),
		Middlewares:  []adk.AgentMiddleware{summaryMiddleware},
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: toolList,
			},
		},
	})
}
