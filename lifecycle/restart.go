package lifecycle

import (
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"fkteams/log"
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

// TriggerShutdown 触发优雅关闭（延迟 200ms 确保 HTTP 响应送达）
func TriggerShutdown() {
	shutdownMu.Lock()
	fn := shutdownFn
	shutdownMu.Unlock()

	if fn != nil {
		go func() {
			time.Sleep(200 * time.Millisecond)
			shutdownOnce.Do(fn)
		}()
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
// 以下情况跳过 self-restart，由外部进程管理器（systemd/Docker）接管：
//   - PID 1（容器 init 进程）
//   - 设置了 FEIKONG_NO_SELF_RESTART 环境变量（适用于 systemd 等场景）
func ExecutePendingRestart() error {
	if !pendingRestart.Load() {
		return nil
	}
	if os.Getpid() == 1 {
		log.Info("skip self-restart: running as PID 1 (container), relying on restart policy")
		return nil
	}
	if os.Getenv("FEIKONG_NO_SELF_RESTART") != "" {
		log.Info("skip self-restart: FEIKONG_NO_SELF_RESTART is set, relying on external process manager")
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
	log.Info("starting new process for restart")
	return cmd.Start()
}
