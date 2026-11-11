package lifecycle

import (
	"sync"
	"sync/atomic"
	"time"
)

// Control 是应用实例的关闭与重启入口；回调由组合根注入。
type Control struct {
	shutdown       func()
	restart        func() error
	once           sync.Once
	pendingRestart atomic.Bool
}

// NewControl 创建与单个应用绑定的生命周期控制器。
func NewControl(shutdown func(), restart func() error) *Control {
	return &Control{shutdown: shutdown, restart: restart}
}

// Available 表示当前实例能否响应关闭请求。
func (c *Control) Available() bool { return c != nil && c.shutdown != nil }

// Shutdown 延迟关闭，让传输层有机会送出响应；每个实例至多触发一次。
func (c *Control) Shutdown() {
	if !c.Available() {
		return
	}
	c.once.Do(func() { time.AfterFunc(200*time.Millisecond, c.shutdown) })
}

// Restart 请求关闭，并在应用清理结束后由组合根执行重启。
func (c *Control) Restart() {
	if !c.Available() {
		return
	}
	c.pendingRestart.Store(true)
	c.Shutdown()
}

// ExecutePendingRestart 在应用清理完成后执行一次重启。
func (c *Control) ExecutePendingRestart() error {
	if c != nil && c.pendingRestart.Swap(false) && c.restart != nil {
		return c.restart()
	}
	return nil
}
