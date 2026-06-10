package engine

import (
	"context"
	"fkteams/agentcore"
	"fkteams/events"
	"fkteams/hooks"
)

type ContextHook func(context.Context) context.Context

type TurnInput = agentcore.TurnInput

type HistorySink interface {
	GetMessageCount() int
	RecordUserMessage(message agentcore.Message)
	SetSummary(summary string, beforeCount int)
}

// runConfig 执行配置，收敛所有生命周期关注点。
// 零值字段均有安全默认值。
type runConfig struct {
	// Input 本轮运行输入
	Input agentcore.TurnInput

	// RunID 本轮运行 ID；为空时使用 checkpointID
	RunID string

	// EventCallback 接收智能体执行期间的事件
	EventCallback func(events.Event) error

	// Recorder 会话历史接收器。设置后 Engine 自动配置摘要持久化回调
	Recorder HistorySink

	// OnStart 执行开始回调（context 装配完成后，事件循环开始前）
	OnStart func(ctx context.Context)

	// OnInterrupt HITL 中断处理。nil 时默认使用固定拒绝决策
	OnInterrupt InterruptHandler

	// NonInteractive 标记非交互模式（WebSocket / 通道），不输出终端动画
	NonInteractive bool

	// ContextHooks 额外 context 装配逻辑
	ContextHooks []ContextHook

	// HookBus 运行期扩展点总线。nil 时使用 hooks.Global()
	HookBus *hooks.Bus

	// OnFinish 执行结束回调（含错误）。用于保存历史、更新元数据、提取记忆等
	OnFinish func(ctx context.Context, result *agentcore.RunResult, err error)
}
