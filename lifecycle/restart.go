package lifecycle

import (
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
)

var (
	shutdownFn   func()
	shutdownOnce sync.Once
	shutdownMu   sync.Mutex

	pendingRestart atomic.Bool
)

// SetShutdownFunc 设置关闭回调函数（由 server 层在启动时调用）
func SetShutdownFunc(fn func()) {
	shutdownMu.Lock()
	defer shutdownMu.Unlock()
	shutdownFn = fn
}

// TriggerShutdown 触发优雅关闭
func TriggerShutdown() {
	shutdownMu.Lock()
	fn := shutdownFn
	shutdownMu.Unlock()

	if fn != nil {
		go shutdownOnce.Do(fn)
	}
}

// TriggerRestart 标记待重启并触发关闭
func TriggerRestart() {
	pendingRestart.Store(true)
	TriggerShutdown()
}

// IsShutdownAvailable 返回是否已注册关闭回调
func IsShutdownAvailable() bool {
	shutdownMu.Lock()
	defer shutdownMu.Unlock()
	return shutdownFn != nil
}

// ExecutePendingRestart 在 cleanup 阶段调用，如有待重启则启动新进程。
// PID 1（容器 init 进程）时跳过 self-restart，由容器运行时的 restart policy 接管。
func ExecutePendingRestart() error {
	if !pendingRestart.Load() {
		return nil
	}
	if os.Getpid() == 1 {
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(executable, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Start()
}
