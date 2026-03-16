package custom

import (
	"context"
	"fkteams/agents/common"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/schema"
)

type Model struct {
	Name    string
	APIKey  string
	BaseURL string
}

type Config struct {
	Name         string
	Description  string
	SystemPrompt string
	Model        Model
	ToolNames    []string
}

func NewAgent(cfg Config) (adk.Agent, error) {
	customPromptTemplate := prompt.FromMessages(schema.FString,
		schema.SystemMessage(cfg.SystemPrompt),
	)

	return common.NewAgentBuilder(cfg.Name, cfg.Description).
		WithTemplate(customPromptTemplate).
		WithToolNames(cfg.ToolNames...).
		WithWarperror().
		Build(context.Background())
}
