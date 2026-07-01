package discussant

import (
	"context"
	"fkteams/internal/app/agent/catalog/common"
	"fkteams/internal/app/config"
	"fmt"

	runtimeport "fkteams/internal/ports/runtime"
)

func NewAgent(ctx context.Context, member config.TeamMember) (runtimeport.Agent, error) {
	cfg := config.Get()
	modelCfg := cfg.ResolveModel(member.Model)
	if modelCfg == nil {
		return nil, fmt.Errorf("模型 %q 未在配置文件中定义", member.Model)
	}

	chatModel, err := common.NewChatModelWithModelConfig(ctx, modelCfg)
	if err != nil {
		return nil, fmt.Errorf("create chat model: %w", err)
	}

	return common.BuildAgent(ctx, common.Definition{
		Name:          member.Name,
		Description:   member.Desc,
		Instruction:   discussantPrompt,
		Profile:       common.ProfileWorkspace,
		Model:         chatModel,
		EnableSummary: true,
	})
}
