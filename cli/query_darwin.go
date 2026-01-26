//go:build darwin
// +build darwin

package cli

import (
	"os"
	"os/signal"
	"syscall"
	"unsafe"
)

// StartKeyboardMonitor 在查询期间监听 Ctrl+C (macOS 实现)
// 通过临时恢复终端的 Cooked 模式，让操作系统处理 Ctrl+C 信号
func StartKeyboardMonitor(state *QueryState) func() {
	fd := int(os.Stdin.Fd())

	// 1. 保存当前终端状态 (raw 模式)
	var oldState syscall.Termios
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.TIOCGETA, uintptr(unsafe.Pointer(&oldState))); err != 0 {
		// 如果获取失败，fallback 到简单的信号监听
		return startSimpleSignalMonitor(state)
	}

	// 2. 恢复终端到 cooked 模式（启用信号处理）
	// 创建新状态：启用 ISIG（信号处理）、ICANON（规范模式）、ECHO（回显）
	newState := oldState
	newState.Lflag |= syscall.ISIG | syscall.ICANON | syscall.ECHO
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.TIOCSETA, uintptr(unsafe.Pointer(&newState))); err != 0 {
		return startSimpleSignalMonitor(state)
	}

	// 3. 监听信号
	sigChan := make(chan os.Signal, 1)
	stopChan := make(chan struct{})
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		select {
		case <-sigChan:
			HandleCtrlC(state)
		case <-stopChan:
			return
		}
	}()

	// 4. 返回清理函数
	return func() {
		close(stopChan)
		signal.Stop(sigChan)
		// 恢复原始终端状态 (raw 模式)
		syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.TIOCSETA, uintptr(unsafe.Pointer(&oldState)))
	}
}

// startSimpleSignalMonitor 简单的信号监听（fallback）
func startSimpleSignalMonitor(state *QueryState) func() {
	sigChan := make(chan os.Signal, 1)
	stopChan := make(chan struct{})
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		select {
		case <-sigChan:
			HandleCtrlC(state)
		case <-stopChan:
			return
		}
	}()

	return func() {
		close(stopChan)
		signal.Stop(sigChan)
	}
}
