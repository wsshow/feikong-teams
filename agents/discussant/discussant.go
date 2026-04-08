package discussant

import (
	"context"
	"fkteams/agents/common"
	"fkteams/config"
	"fmt"

	"github.com/cloudwego/eino/adk"
)

func NewAgent(ctx context.Context, member config.TeamMember) (adk.Agent, error) {
	cfg := config.Get()
	modelCfg := cfg.ResolveModel(member.Model)
	if modelCfg == nil {
		return nil, fmt.Errorf("模型 %q 未在配置文件中定义", member.Model)
	}

	chatModel, err := common.NewChatModelWithModelConfig(modelCfg)
	if err != nil {
		return nil, fmt.Errorf("create chat model: %w", err)
	}

	return common.NewAgentBuilder(member.Name, member.Desc).
		WithTemplate(discussantPromptTemplate).
		WithModel(chatModel).
		WithSummary().
		Build(ctx)
}
