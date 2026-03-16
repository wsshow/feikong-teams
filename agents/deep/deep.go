package deep

import (
	"context"
	"fkteams/agents/common"
	"fkteams/tools"
	"fmt"

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

	return deep.New(ctx, &deep.Config{
		Name:         "深度探索者",
		Description:  "一个能够深入分析问题并协调多个智能体协作解决复杂任务的智能体。",
		ChatModel:    chatModel,
		SubAgents:    subAgents,
		MaxIteration: common.MaxIterations,
		ModelRetryConfig: &adk.ModelRetryConfig{
			MaxRetries:  common.MaxRetries,
			IsRetryAble: common.IsRetryAble,
		},
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: toolList,
			},
		},
	})
}
