package custom

import (
	"context"
	"fkteams/agents/common"
	"fkteams/agents/middlewares/tools/warperror"
	"fkteams/tools"
	"fmt"
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

func NewAgent(cfg Config) (adk.Agent, error) {
	ctx := context.Background()

	customPromptTemplate := prompt.FromMessages(schema.FString,
		schema.SystemMessage(cfg.SystemPrompt),
	)
	systemMessages, err := customPromptTemplate.Format(ctx, map[string]any{
		"current_time": time.Now().Format("2006-01-02 15:04:05"),
	})
	if err != nil {
		return nil, fmt.Errorf("format prompt: %w", err)
	}

	var toolList []tool.BaseTool
	for _, toolName := range cfg.ToolNames {
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

	return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          cfg.Name,
		Description:   cfg.Description,
		Instruction:   systemMessages[0].Content,
		Model:         chatModel,
		MaxIterations: common.MaxIterations,
		ModelRetryConfig: &adk.ModelRetryConfig{
			MaxRetries:  common.MaxRetries,
			IsRetryAble: common.IsRetryAble,
		},
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools:               toolList,
				ToolCallMiddlewares: []compose.ToolMiddleware{warperror.New(nil)},
			},
		},
	})
}
