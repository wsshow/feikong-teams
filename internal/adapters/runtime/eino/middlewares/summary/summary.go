package summary

import (
	"context"
	"fkteams/agentcore"
	"fkteams/events"
	einoruntime "fkteams/internal/adapters/runtime/eino"
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/summarization"
	"github.com/cloudwego/eino/schema"
)

type Config struct {
	MaxTokensBeforeSummary int
	Model                  agentcore.ChatModel
}

func New(ctx context.Context, cfg *Config) (agentcore.AgentMiddleware, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	if cfg.Model == nil {
		return nil, fmt.Errorf("model is nil")
	}

	maxBefore := agentcore.DefaultMaxTokensBeforeSummary
	if cfg.MaxTokensBeforeSummary > 0 {
		maxBefore = cfg.MaxTokensBeforeSummary
	}
	chatModel, err := einoruntime.AdaptChatModelForRunner(cfg.Model)
	if err != nil {
		return nil, err
	}

	handler, err := summarization.New(ctx, &summarization.Config{
		Model: chatModel,
		Trigger: &summarization.TriggerCondition{
			ContextTokens: maxBefore,
		},
		Callback: func(ctx context.Context, _ adk.ChatModelAgentState, after adk.ChatModelAgentState) error {
			return handleSummaryCallback(ctx, after)
		},
	})
	if err != nil {
		return nil, err
	}
	return einoruntime.WrapAgentMiddleware("summary", handler), nil
}

func handleSummaryCallback(ctx context.Context, after adk.ChatModelAgentState) error {
	summaryText := latestSummaryText(after.Messages)
	if cb, ok := agentcore.SummaryPersistCallbackFromContext(ctx); ok {
		cb(summaryText)
	}
	_ = events.DispatchEvent(ctx, events.Event{
		Type:       events.EventAction,
		AgentName:  "系统",
		ActionType: events.ActionContextCompress,
		Content:    "对话上下文已压缩，旧消息已被总结摘要替代",
		Detail:     summaryText,
	})
	return nil
}

func latestSummaryText(messages []*schema.Message) string {
	if len(messages) == 0 {
		return ""
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i] != nil && messages[i].Content != "" {
			return messages[i].Content
		}
	}
	return ""
}
