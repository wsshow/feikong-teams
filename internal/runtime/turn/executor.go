package turn

import (
	"context"
	"errors"
	"fmt"

	"fkteams/internal/domain/message"
	"fkteams/internal/domain/session"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/events"
	"fkteams/internal/runtime/hooks"
)

// Executor 执行一次 turn 请求。
type Executor struct{}

func NewExecutor() *Executor {
	return &Executor{}
}

func (e *Executor) Run(ctx context.Context, req Request) (*runtimeport.RunResult, error) {
	if req.Runner == nil {
		return nil, fmt.Errorf("turn runner is required")
	}
	if req.SessionID == "" {
		return nil, fmt.Errorf("turn session ID is required")
	}
	return e.run(ctx, req)
}

// run 执行查询，处理事件和 HITL 中断。
// 根据 Request 自动装配 context（session ID、事件回调、摘要持久化、审批注册表等）。
func (e *Executor) run(ctx context.Context, cfg Request) (result *runtimeport.RunResult, err error) {
	ctx = cfg.prepareContext(ctx, cfg.SessionID)
	defer func() {
		if cfg.OnFinish != nil {
			err = errors.Join(err, cfg.OnFinish(context.WithoutCancel(ctx), result, err))
		}
	}()
	if cfg.RunID == "" {
		cfg.RunID = cfg.SessionID + ":" + session.NewID()
	}

	input, err := cfg.invokeBeforeRun(ctx)
	if err != nil {
		return nil, err
	}
	ctx = cfg.prepareHistoryContext(ctx)

	if cfg.OnStart != nil {
		cfg.OnStart(ctx)
	}

	result, err = cfg.Runner.Run(ctx, input, runtimeport.RunOptions{
		RunID: cfg.RunID, Sink: events.Dispatch(ctx), InterruptHandler: runtimeport.InterruptHandler(cfg.interruptHandler()),
	}.WithDefaults(cfg.SessionID))

	err = errors.Join(err, cfg.invokeAfterRun(ctx, input, result, err))

	return result, err
}

func (cfg Request) prepareContext(ctx context.Context, sessionID string) context.Context {
	ctx = session.WithID(ctx, sessionID)
	ctx = hooks.WithBus(ctx, cfg.hookBus())

	if cfg.EventSink != nil {
		ctx = events.WithCallback(ctx, cfg.EventSink)
	}

	if cfg.NonInteractive {
		ctx = events.WithNonInteractive(ctx)
	}

	for _, hook := range cfg.ContextHooks {
		if hook != nil {
			ctx = hook(ctx)
		}
	}
	return ctx
}

func (cfg Request) prepareHistoryContext(ctx context.Context) context.Context {
	if cfg.Summary != nil {
		countBefore := cfg.Summary.GetMessageCount()
		ctx = runtimeport.WithSummaryPersistCallback(ctx, func(s string) {
			cfg.Summary.SetSummary(s, countBefore)
		})
	}
	return ctx
}

func (cfg Request) invokeBeforeRun(ctx context.Context) (message.TurnInput, error) {
	return cfg.hookBus().InvokeBeforeRun(ctx, cfg.Input)
}

func (cfg Request) invokeAfterRun(ctx context.Context, input message.TurnInput, result *runtimeport.RunResult, runErr error) error {
	return cfg.hookBus().InvokeAfterRun(ctx, input, result, runErr)
}

func (cfg Request) hookBus() *hooks.Bus {
	if cfg.HookBus != nil {
		return cfg.HookBus
	}
	return nil
}

func (cfg Request) interruptHandler() InterruptHandler {
	if cfg.OnInterrupt != nil {
		return cfg.OnInterrupt
	}
	return FixedDecisionHandler(0)
}
