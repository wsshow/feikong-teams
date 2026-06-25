// Package turn 提供统一的回合执行内核，封装 Runner 事件循环和 HITL 中断处理。
package turn

import (
	"fkteams/agentcore"
)

type core struct {
	runner       agentcore.Runner
	checkpointID string
}

func newEngine(runner agentcore.Runner, checkpointID string) *core {
	return &core{runner: runner, checkpointID: checkpointID}
}
