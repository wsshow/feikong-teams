// Package engine 提供统一的执行引擎，封装 Runner 事件循环和 HITL 中断处理。
package engine

import (
	"context"
	"fkteams/fkevent"
	"fkteams/tools/command"
	"fmt"

	"github.com/cloudwego/eino/adk"
)

// InterruptHandler 中断处理回调，接收中断上下文列表，返回审批目标映射
type InterruptHandler func(ctx context.Context, interrupts []*adk.InterruptCtx) (targets map[string]any, err error)

// Engine 执行引擎，封装 Runner 事件循环和 HITL 中断处理
type Engine struct {
	runner       *adk.Runner
	checkpointID string
}

// New 创建执行引擎
func New(runner *adk.Runner, checkpointID string) *Engine {
	return &Engine{runner: runner, checkpointID: checkpointID}
}

// RunOption 运行选项
type RunOption func(*runConfig)

type runConfig struct {
	onInterrupt InterruptHandler
}

// WithInterruptHandler 设置中断处理回调。未设置时，遇到中断将直接结束执行。
func WithInterruptHandler(h InterruptHandler) RunOption {
	return func(c *runConfig) { c.onInterrupt = h }
}

// Run 执行查询，处理事件和 HITL 中断。
// ctx 应已通过 fkevent.WithCallback 绑定事件回调。
func (e *Engine) Run(ctx context.Context, messages []adk.Message, opts ...RunOption) (*adk.AgentEvent, error) {
	cfg := &runConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	iter := e.runner.Run(ctx, messages, adk.WithCheckPointID(e.checkpointID))
	for {
		lastEvent, err := drainEvents(ctx, iter)
		if err != nil {
			return lastEvent, err
		}

		if lastEvent != nil && lastEvent.Action != nil && lastEvent.Action.Interrupted != nil {
			interrupts := lastEvent.Action.Interrupted.InterruptContexts
			if len(interrupts) > 0 && cfg.onInterrupt != nil {
				targets, handlerErr := cfg.onInterrupt(ctx, interrupts)
				if handlerErr != nil {
					return lastEvent, handlerErr
				}
				resumeIter, resumeErr := e.runner.ResumeWithParams(ctx, e.checkpointID, &adk.ResumeParams{
					Targets: targets,
				})
				if resumeErr != nil {
					return lastEvent, fmt.Errorf("resume failed: %w", resumeErr)
				}
				iter = resumeIter
				continue
			}
		}
		return lastEvent, nil
	}
}

// drainEvents 遍历迭代器中所有事件，逐个调用 ProcessAgentEvent
func drainEvents(ctx context.Context, iter *adk.AsyncIterator[*adk.AgentEvent]) (*adk.AgentEvent, error) {
	var lastEvent *adk.AgentEvent
	for {
		select {
		case <-ctx.Done():
			return lastEvent, ctx.Err()
		default:
		}

		event, ok := iter.Next()
		if !ok {
			return lastEvent, nil
		}
		lastEvent = event
		if err := fkevent.ProcessAgentEvent(ctx, event); err != nil {
			return lastEvent, err
		}
	}
}

// AutoRejectHandler 自动拒绝所有危险命令
func AutoRejectHandler() InterruptHandler {
	return func(_ context.Context, interrupts []*adk.InterruptCtx) (map[string]any, error) {
		targets := make(map[string]any, len(interrupts))
		for _, ic := range interrupts {
			if ic.IsRootCause {
				targets[ic.ID] = command.DecisionReject
			}
		}
		return targets, nil
	}
}

// ChannelHandler 通过 channel 等待审批决定（用于 WebSocket）
func ChannelHandler(ch <-chan int) InterruptHandler {
	return func(ctx context.Context, interrupts []*adk.InterruptCtx) (map[string]any, error) {
		var decision int
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case decision = <-ch:
		}

		targets := make(map[string]any, len(interrupts))
		for _, ic := range interrupts {
			if ic.IsRootCause {
				targets[ic.ID] = decision
			}
		}
		return targets, nil
	}
}

// CallbackHandler 通过回调函数获取审批决定（用于 CLI 交互式审批）
func CallbackHandler(promptFunc func() int) InterruptHandler {
	return func(_ context.Context, interrupts []*adk.InterruptCtx) (map[string]any, error) {
		decision := promptFunc()
		targets := make(map[string]any, len(interrupts))
		for _, ic := range interrupts {
			if ic.IsRootCause {
				targets[ic.ID] = decision
			}
		}
		return targets, nil
	}
}
