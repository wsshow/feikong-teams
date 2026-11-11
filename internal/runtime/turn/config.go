package turn

import (
	"context"
	"fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/events"
	"fkteams/internal/runtime/hooks"
)

type ContextHook func(context.Context) context.Context

type TurnInput = message.TurnInput

type SummarySink interface {
	GetMessageCount() int
	SetSummary(summary string, beforeCount int)
}

// Request 执行配置，收敛一次 turn 的所有生命周期关注点。
// 零值字段均有安全默认值。
type Request struct {
	// Runner 执行当前 turn 的 runtime runner。
	Runner runtimeport.Runner

	// SessionID 会话 ID，用于上下文和持久化归属。
	SessionID string

	// Input 本轮运行输入
	Input message.TurnInput

	// RunID 本轮运行 ID；为空时生成唯一标识
	RunID string

	// EventCallback 接收智能体执行期间的事件
	EventSink func(events.Event) error

	// Summary 会话摘要接收器。设置后自动配置摘要持久化回调
	Summary SummarySink

	// OnStart 执行开始回调（context 装配完成后，事件循环开始前）
	OnStart func(ctx context.Context)

	// OnInterrupt HITL 中断处理。nil 时默认使用固定拒绝决策
	OnInterrupt InterruptHandler

	// NonInteractive 标记非交互模式（WebSocket / 通道），不输出终端动画
	NonInteractive bool

	// ContextHooks 额外 context 装配逻辑
	ContextHooks []ContextHook

	// HookBus 运行期扩展点总线。nil 时不执行 hook
	HookBus *hooks.Bus

	// OnFinish 执行结束回调（含错误）。用于保存历史、更新元数据、提取记忆等
	OnFinish func(ctx context.Context, result *runtimeport.RunResult, err error) error
}
