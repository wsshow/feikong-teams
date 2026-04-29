package engine

import (
	"context"
	"fkteams/agents/middlewares/summary"
	"fkteams/common"
	"fkteams/fkevent"
	"fkteams/tools/approval"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// Run 执行查询，处理事件和 HITL 中断。
// 根据 RunConfig 自动装配 context（session ID、事件回调、摘要持久化、审批注册表等）。
func (e *Engine) Run(ctx context.Context, cfg RunConfig) (*adk.AgentEvent, error) {
	ctx = common.WithSessionID(ctx, e.checkpointID)

	if cfg.EventCallback != nil {
		ctx = fkevent.WithCallback(ctx, cfg.EventCallback)
	}

	if cfg.Recorder != nil {
		countBefore := cfg.Recorder.GetMessageCount()
		if userText := lastUserMessageText(cfg.Messages); userText != "" {
			cfg.Recorder.RecordUserInput(userText)
		}
		ctx = summary.WithSummaryPersistCallback(ctx, func(s string) {
			cfg.Recorder.SetSummary(s, countBefore)
		})
	}

	if cfg.NonInteractive {
		ctx = fkevent.WithNonInteractive(ctx)
	}

	if cfg.ApprovalReg != nil {
		ctx = approval.WithRegistry(ctx, cfg.ApprovalReg)
	}

	if cfg.OnStart != nil {
		cfg.OnStart(ctx)
	}

	handler := cfg.OnInterrupt
	if handler == nil {
		handler = AutoRejectHandler()
	}

	lastEvent, err := e.runLoop(ctx, cfg.Messages, handler)

	if cfg.OnFinish != nil {
		cfg.OnFinish(ctx, lastEvent, err)
	}

	return lastEvent, err
}

func lastUserMessageText(messages []adk.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role != schema.User {
			continue
		}
		if msg.Content != "" {
			return msg.Content
		}
		for _, part := range msg.UserInputMultiContent {
			if part.Type == schema.ChatMessagePartTypeText {
				return part.Text
			}
		}
		for _, part := range msg.MultiContent {
			if part.Type == schema.ChatMessagePartTypeText {
				return part.Text
			}
		}
		return ""
	}
	return ""
}
