package patch

import (
	"context"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/patchtoolcalls"
)

func New(ctx context.Context) (adk.ChatModelAgentMiddleware, error) {
	chatModelAgentMiddleware, err := patchtoolcalls.New(ctx, nil)
	if err != nil {
		return nil, err
	}
	return chatModelAgentMiddleware, nil
}
