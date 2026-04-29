package engine

import (
	"context"
	"fkteams/tools/approval"
	"fkteams/tools/ask"

	"github.com/cloudwego/eino/adk"
)

// InterruptHandler 中断处理回调，接收中断上下文列表，返回审批目标映射
type InterruptHandler func(ctx context.Context, interrupts []*adk.InterruptCtx) (targets map[string]any, err error)

// AutoRejectHandler 自动拒绝所有危险命令
func AutoRejectHandler() InterruptHandler {
	return func(_ context.Context, interrupts []*adk.InterruptCtx) (map[string]any, error) {
		targets := make(map[string]any, len(interrupts))
		for _, ic := range interrupts {
			if ic.IsRootCause {
				targets[ic.ID] = approval.Reject
			}
		}
		return targets, nil
	}
}

// ChannelHandler 通过 channel 等待审批决定（用于 WebSocket）
func ChannelHandler(ch <-chan any) InterruptHandler {
	return func(ctx context.Context, interrupts []*adk.InterruptCtx) (map[string]any, error) {
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

// CompositeCallbackHandler 组合中断处理器，根据中断类型分发到不同回调
func CompositeCallbackHandler(approvalFunc func() int, askFunc func(*ask.AskInfo) *ask.AskResponse) InterruptHandler {
	return func(_ context.Context, interrupts []*adk.InterruptCtx) (map[string]any, error) {
		targets := make(map[string]any, len(interrupts))
		for _, ic := range interrupts {
			if !ic.IsRootCause {
				continue
			}
			if info, ok := ic.Info.(*ask.AskInfo); ok && askFunc != nil {
				resp := askFunc(info)
				targets[ic.ID] = resp
			} else if approvalFunc != nil {
				targets[ic.ID] = approvalFunc()
			}
		}
		return targets, nil
	}
}
