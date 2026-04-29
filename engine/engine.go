// Package engine 提供统一的执行引擎，封装 Runner 事件循环和 HITL 中断处理。
package engine

import (
	"github.com/cloudwego/eino/adk"
)

// Engine 执行引擎，封装 Runner 事件循环和 HITL 中断处理
type Engine struct {
	runner       *adk.Runner
	checkpointID string
}

// New 创建执行引擎
func New(runner *adk.Runner, checkpointID string) *Engine {
	return &Engine{runner: runner, checkpointID: checkpointID}
}
