package engine

import (
	"context"
	"fkteams/agentcore"
)

// InterruptHandler 中断处理回调，接收中断上下文列表，返回审批目标映射
type InterruptHandler func(ctx context.Context, interrupts []agentcore.Interrupt) (targets map[string]any, err error)

type InterruptInfoHandler func(info any) (decision any, ok bool)

func FixedDecisionHandler(decision any) InterruptHandler {
	return func(_ context.Context, interrupts []agentcore.Interrupt) (map[string]any, error) {
		targets := make(map[string]any, len(interrupts))
		for _, ic := range interrupts {
			if ic.IsRootCause {
				targets[ic.ID] = decision
			}
		}
		return targets, nil
	}
}

// ChannelHandler 通过 channel 等待审批决定（用于 WebSocket）
func ChannelHandler(ch <-chan any) InterruptHandler {
	return func(ctx context.Context, interrupts []agentcore.Interrupt) (map[string]any, error) {
		var decision any
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

func ChannelTargetHandler(ch <-chan any, targetID string) InterruptHandler {
	return func(ctx context.Context, _ []agentcore.Interrupt) (map[string]any, error) {
		var decision any
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case decision = <-ch:
		}

		targets := make(map[string]any, 1)
		if targetID != "" {
			targets[targetID] = decision
		}
		return targets, nil
	}
}

// CallbackHandler 通过回调函数获取统一决策
func CallbackHandler(promptFunc func() any) InterruptHandler {
	return func(_ context.Context, interrupts []agentcore.Interrupt) (map[string]any, error) {
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

// InfoHandler 根据中断信息逐项生成恢复决策
func InfoHandler(handler InterruptInfoHandler) InterruptHandler {
	return func(_ context.Context, interrupts []agentcore.Interrupt) (map[string]any, error) {
		targets := make(map[string]any, len(interrupts))
		for _, ic := range interrupts {
			if !ic.IsRootCause {
				continue
			}
			if decision, ok := handler(ic.Info); ok {
				targets[ic.ID] = decision
			}
		}
		return targets, nil
	}
}
