package discussant

import (
	"context"
	"fkteams/agents/common"
	"fkteams/config"
	"fkteams/providers"
	"fmt"

	"github.com/cloudwego/eino/adk"
)

func NewAgent(ctx context.Context, member config.TeamMember) (adk.Agent, error) {
	chatModel, err := common.NewChatModelWithConfig(&providers.Config{
		Provider: providers.Type(member.Provider),
		APIKey:   member.APIKey,
		BaseURL:  member.BaseURL,
		Model:    member.ModelName,
	})
	if err != nil {
		return nil, fmt.Errorf("create chat model: %w", err)
	}

	return common.NewAgentBuilder(member.Name, member.Desc).
		WithTemplate(discussantPromptTemplate).
		WithModel(chatModel).
		Build(ctx)
}
