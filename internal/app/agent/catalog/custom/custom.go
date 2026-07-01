package custom

import (
	"context"
	"fkteams/internal/app/agent/catalog/common"
	runtimeport "fkteams/internal/ports/runtime"
	modelregistry "fkteams/internal/runtime/model"
	"fmt"
)

type Model struct {
	Provider string
	Name     string
	APIKey   string
	BaseURL  string
}

type Config struct {
	Name         string
	Description  string
	SystemPrompt string
	Model        Model
	ToolNames    []string
	Tools        []runtimeport.Tool
}

func NewAgent(ctx context.Context, cfg Config) (runtimeport.Agent, error) {
	def := common.Definition{
		Name:        cfg.Name,
		Description: cfg.Description,
		Instruction: cfg.SystemPrompt,
		Profile:     common.ProfileFull,
		Tools:       cfg.Tools,
		ToolNames:   cfg.ToolNames,
	}

	if cfg.Model.Name != "" || cfg.Model.BaseURL != "" {
		chatModel, err := common.NewChatModelWithConfig(ctx, &modelregistry.Config{
			Provider: modelregistry.Type(cfg.Model.Provider),
			APIKey:   cfg.Model.APIKey,
			BaseURL:  cfg.Model.BaseURL,
			Model:    cfg.Model.Name,
		})
		if err != nil {
			return nil, fmt.Errorf("create chat model: %w", err)
		}
		def.Model = chatModel
	}

	return common.BuildAgent(ctx, def)
}
