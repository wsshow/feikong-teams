package discussant

import (
	"context"
	"fkteams/agents/common"
	"fkteams/config"
	"fmt"

	"github.com/cloudwego/eino/adk"
)

func NewAgent(member config.TeamMember) (adk.Agent, error) {
	chatModel, err := common.NewChatModelWithConfig(member.ModelName, member.BaseURL, member.APIKey)
	if err != nil {
		return nil, fmt.Errorf("create chat model: %w", err)
	}

	return common.NewAgentBuilder(member.Name, member.Desc).
		WithTemplate(DiscussantPromptTemplate).
		WithModel(chatModel).
		Build(context.Background())
}
