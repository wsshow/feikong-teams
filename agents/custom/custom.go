package custom

import (
	"context"
	"fkteams/agents/common"
	"fkteams/tools"
	"log"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
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

	var toolList []tool.BaseTool
	for _, toolName := range cfg.ToolNames {
		baseTools, err := tools.GetToolsByName(toolName)
		if err != nil {
			log.Fatal(err)
		}
		toolList = append(toolList, baseTools...)
	}

	a, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          cfg.Name,
		Description:   cfg.Description,
		Instruction:   instruction,
		Model:         common.NewChatModel(),
		MaxIterations: common.MaxIterations,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: toolList,
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	return a
}
