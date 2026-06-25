package patch

import (
	"context"
	"fkteams/agentcore"
	einoruntime "fkteams/internal/adapters/runtime/eino"

	"github.com/cloudwego/eino/adk/middlewares/patchtoolcalls"
)

func New(ctx context.Context) (agentcore.AgentMiddleware, error) {
	chatModelAgentMiddleware, err := patchtoolcalls.New(ctx, nil)
	if err != nil {
		return nil, err
	}
	return einoruntime.WrapAgentMiddleware("patch_tool_calls", chatModelAgentMiddleware), nil
}
