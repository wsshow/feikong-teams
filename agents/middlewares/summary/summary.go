package summary

import (
	"context"
	"fkteams/fkevent"
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/summarization"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const DefaultMaxTokensBeforeSummary = 800 * 1024

// SummaryPersistCallback 摘要持久化回调
type SummaryPersistCallback func(summaryText string)

type summaryPersistCallbackKey struct{}

// WithSummaryPersistCallback 设置摘要持久化回调到 context
func WithSummaryPersistCallback(ctx context.Context, cb SummaryPersistCallback) context.Context {
	return context.WithValue(ctx, summaryPersistCallbackKey{}, cb)
}

type Config struct {
	MaxTokensBeforeSummary int
	Model                  model.BaseChatModel
}

func New(ctx context.Context, cfg *Config) (adk.ChatModelAgentMiddleware, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	if cfg.Model == nil {
		return nil, fmt.Errorf("model is nil")
	}

	maxBefore := DefaultMaxTokensBeforeSummary
	if cfg.MaxTokensBeforeSummary > 0 {
		maxBefore = cfg.MaxTokensBeforeSummary
	}

	return summarization.New(ctx, &summarization.Config{
		Model: cfg.Model,
		Trigger: &summarization.TriggerCondition{
			ContextTokens: maxBefore,
		},
		Callback: func(ctx context.Context, _ adk.ChatModelAgentState, after adk.ChatModelAgentState) error {
			summaryText := latestSummaryText(after.Messages)
			if cb, ok := ctx.Value(summaryPersistCallbackKey{}).(SummaryPersistCallback); ok {
				cb(summaryText)
			}
			_ = fkevent.DispatchEvent(ctx, fkevent.Event{
				Type:       fkevent.EventAction,
				AgentName:  "系统",
				ActionType: fkevent.ActionContextCompress,
				Content:    "对话上下文已压缩，旧消息已被总结摘要替代",
				Detail:     summaryText,
			})
			return nil
		},
	})
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
