package discussant

import (
	"context"
	"fkteams/agents/common"
	"fkteams/config"
	"fmt"
	"time"

	"github.com/cloudwego/eino/adk"
)

func NewAgent(member config.TeamMember) (adk.Agent, error) {
	ctx := context.Background()
	systemMessages, err := DiscussantPromptTemplate.Format(ctx, map[string]any{
		"current_time": time.Now().Format("2006-01-02 15:04:05"),
	})
	if err != nil {
		return nil, fmt.Errorf("format prompt: %w", err)
	}

	chatModel, err := common.NewChatModelWithConfig(member.ModelName, member.BaseURL, member.APIKey)
	if err != nil {
		return nil, fmt.Errorf("create chat model: %w", err)
	}

	return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          member.Name,
		Description:   member.Desc,
		Instruction:   systemMessages[0].Content,
		Model:         chatModel,
		MaxIterations: common.MaxIterations,
		ModelRetryConfig: &adk.ModelRetryConfig{
			MaxRetries:  common.MaxRetries,
			IsRetryAble: common.IsRetryAble,
		},
	})
}
