package engine

import (
	"context"
	"fkteams/fkevent"
	"fkteams/tools/approval"

	"github.com/cloudwego/eino/adk"
)

// RunConfig 执行配置，收敛所有生命周期关注点。
// 零值字段均有安全默认值。
type RunConfig struct {
	// Messages 输入消息列表
	Messages []adk.Message

	// EventCallback 接收智能体执行期间的事件
	EventCallback func(fkevent.Event) error

	// Recorder 会话历史记录器。设置后 Engine 自动配置摘要持久化回调
	Recorder *fkevent.HistoryRecorder

	// OnStart 执行开始回调（context 装配完成后，事件循环开始前）
	OnStart func(ctx context.Context)

	// OnInterrupt HITL 中断处理。nil 时默认使用 AutoRejectHandler
	OnInterrupt InterruptHandler

	// NonInteractive 标记非交互模式（WebSocket / 通道），不输出终端动画
	NonInteractive bool

	// ApprovalReg 自动审批注册表。nil 时不设置
	ApprovalReg *approval.Registry

	// OnFinish 执行结束回调（含错误）。用于保存历史、更新元数据、提取记忆等
	OnFinish func(ctx context.Context, lastEvent *adk.AgentEvent, err error)
}
