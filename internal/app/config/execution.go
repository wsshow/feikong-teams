package config

import (
	"errors"
	"sync"
)

// ErrExecutionsActive 表示依赖仍被运行中的任务使用，配置更新应在任务结束后重试。
var ErrExecutionsActive = errors.New("configuration cannot be updated while tasks are running")

// ExecutionGate 保护一次应用的配置及派生依赖，避免重载关闭正在使用的 MCP 连接。
// 读取覆盖构建、执行与收尾；更新仅在空闲边界整体发布，不等待长任务。
type ExecutionGate struct{ mu sync.RWMutex }

// Begin 为一次完整执行取得租约，返回可重复调用的释放函数。
func (g *ExecutionGate) Begin() func() {
	if g == nil {
		return func() {}
	}
	g.mu.RLock()
	var once sync.Once
	return func() { once.Do(g.mu.RUnlock) }
}

// BeginUpdate 取得更新租约；调用方必须在配置和派生依赖全部更新后释放。
func (g *ExecutionGate) BeginUpdate() (func(), error) {
	if g == nil {
		return func() {}, nil
	}
	if !g.mu.TryLock() {
		return nil, ErrExecutionsActive
	}
	var once sync.Once
	return func() { once.Do(g.mu.Unlock) }, nil
}
