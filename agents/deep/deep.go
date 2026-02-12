package deep

import (
	"context"
	"fkteams/agents/common"
	"log"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/deep"
)

func NewAgent(ctx context.Context, subAgents []adk.Agent) (adk.Agent, error) {
	deepAgent, err := deep.New(ctx, &deep.Config{
		Name:         "深度探索者",
		Description:  "一个能够深入分析问题并协调多个智能体协作解决复杂任务的智能体。",
		ChatModel:    common.NewChatModel(),
		SubAgents:    subAgents,
		MaxIteration: common.MaxIterations,
	})
	if err != nil {
		log.Fatal(err)
	}
	return deepAgent, nil
}
