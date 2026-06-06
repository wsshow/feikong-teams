package engine

import (
	"context"
	"fkteams/agentcore"
	"fkteams/common"
	"fkteams/fkevent"
)

// run 执行查询，处理事件和 HITL 中断。
// 根据 runConfig 自动装配 context（session ID、事件回调、摘要持久化、审批注册表等）。
func (e *core) run(ctx context.Context, cfg runConfig) (*agentcore.RunResult, error) {
	ctx = common.WithSessionID(ctx, e.checkpointID)

	if cfg.EventCallback != nil {
		ctx = fkevent.WithCallback(ctx, cfg.EventCallback)
	}

	if cfg.Recorder != nil {
		countBefore := cfg.Recorder.GetMessageCount()
		if !cfg.Input.Message.IsEmpty() {
			cfg.Recorder.RecordUserMessage(cfg.Input.Message)
		}
		ctx = agentcore.WithSummaryPersistCallback(ctx, func(s string) {
			cfg.Recorder.SetSummary(s, countBefore)
		})
	}

	if cfg.NonInteractive {
		ctx = fkevent.WithNonInteractive(ctx)
	}

	for _, hook := range cfg.ContextHooks {
		if hook != nil {
			ctx = hook(ctx)
		}
	}

	if cfg.OnStart != nil {
		cfg.OnStart(ctx)
	}

	handler := cfg.OnInterrupt
	if handler == nil {
		handler = FixedDecisionHandler(0)
	}

	result, err := e.runLoop(ctx, cfg.Input, handler)

	if cfg.OnFinish != nil {
		cfg.OnFinish(ctx, result, err)
	}

	return result, err
}
