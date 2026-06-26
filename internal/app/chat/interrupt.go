package chat

import (
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/turn"
)

// InterruptInfoHandler 根据中断信息生成恢复决策。
type InterruptInfoHandler func(info any) (decision any, ok bool)

// ChannelInterruptHandler 通过 channel 等待统一的 HITL 决策。
func ChannelInterruptHandler(ch <-chan any) runtimeport.InterruptHandler {
	return runtimeport.InterruptHandler(turn.ChannelHandler(ch))
}

// ChannelTargetInterruptHandler 通过 channel 等待指定中断目标的 HITL 决策。
func ChannelTargetInterruptHandler(ch <-chan any, targetID string) runtimeport.InterruptHandler {
	return runtimeport.InterruptHandler(turn.ChannelTargetHandler(ch, targetID))
}

// InfoInterruptHandler 根据每个 root interrupt 的 Info 字段生成恢复决策。
func InfoInterruptHandler(handler InterruptInfoHandler) runtimeport.InterruptHandler {
	return runtimeport.InterruptHandler(turn.InfoHandler(turn.InterruptInfoHandler(handler)))
}
