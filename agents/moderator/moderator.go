package moderator

import (
	"context"
	"fkteams/agents/common"
	"fmt"
	"time"

	"github.com/cloudwego/eino/adk"
)

func NewAgent() (adk.Agent, error) {
	ctx := context.Background()
	systemMessages, err := ModeratorPromptTemplate.Format(ctx, map[string]any{
		"current_time": time.Now().Format("2006-01-02 15:04:05"),
	})
	if err != nil {
		return nil, fmt.Errorf("format prompt: %w", err)
	}

	chatModel, err := common.NewChatModel()
	if err != nil {
		return nil, fmt.Errorf("create chat model: %w", err)
	}

	return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          "小议",
		Description:   "会议主持人，负责引导讨论并确保各成员积极参与。",
		Instruction:   systemMessages[0].Content,
		Model:         chatModel,
		MaxIterations: common.MaxIterations,
		ModelRetryConfig: &adk.ModelRetryConfig{
			MaxRetries:  common.MaxRetries,
			IsRetryAble: common.IsRetryAble,
		},
	})
}
