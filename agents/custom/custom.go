package custom

import (
	"context"
	"fkteams/agents/common"
	"log"
	"time"

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
}

func NewAgent(cfg Config) adk.Agent {
	ctx := context.Background()

	customPrompt := cfg.SystemPrompt
	customPromptTemplate := prompt.FromMessages(schema.FString,
		schema.SystemMessage(customPrompt),
	)
	systemMessages, err := customPromptTemplate.Format(ctx, map[string]any{
		"current_time": time.Now().Format("2006-01-02 15:04:05"),
	})
	if err != nil {
		log.Fatal(err)
	}
	instruction := systemMessages[0].Content

	a, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          cfg.Name,
		Description:   cfg.Description,
		Instruction:   instruction,
		Model:         common.NewChatModel(),
		MaxIterations: common.MaxIterations,
	})
	if err != nil {
		log.Fatal(err)
	}
	return a
}
