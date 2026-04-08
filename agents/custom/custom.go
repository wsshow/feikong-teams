package custom

import (
	"context"
	"fkteams/agents/common"
	"fkteams/providers"
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/schema"
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
}

func NewAgent(ctx context.Context, cfg Config) (adk.Agent, error) {
	customPromptTemplate := prompt.FromMessages(schema.FString,
		schema.SystemMessage(cfg.SystemPrompt),
	)

	builder := common.NewAgentBuilder(cfg.Name, cfg.Description).
		WithTemplate(customPromptTemplate).
		WithToolNames(cfg.ToolNames...).
		WithSummary().
		WithSkills()

	if cfg.Model.Name != "" || cfg.Model.BaseURL != "" {
		chatModel, err := common.NewChatModelWithConfig(&providers.Config{
			Provider: providers.Type(cfg.Model.Provider),
			APIKey:   cfg.Model.APIKey,
			BaseURL:  cfg.Model.BaseURL,
			Model:    cfg.Model.Name,
		})
		if err != nil {
			return nil, fmt.Errorf("create chat model: %w", err)
		}
		builder = builder.WithModel(chatModel)
	}

	return builder.Build(ctx)
}
