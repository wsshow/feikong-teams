//go:build windows
// +build windows

package cli

import (
	"os"
	"os/signal"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32           = windows.NewLazyDLL("kernel32.dll")
	procGetConsoleMode = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode = kernel32.NewProc("SetConsoleMode")
)

const (
	enableProcessedInput = 0x0001
	enableLineInput      = 0x0002
	enableEchoInput      = 0x0004
	enableWindowInput    = 0x0008
	enableMouseInput     = 0x0010
	enableInsertMode     = 0x0020
	enableQuickEditMode  = 0x0040
	enableExtendedFlags  = 0x0080
	enableAutoPosition   = 0x0100
)

// StartKeyboardMonitor 在查询期间监听 Ctrl+C (Windows 实现)
func StartKeyboardMonitor(state *QueryState) func() {
	handle := windows.Handle(os.Stdin.Fd())

	// 1. 保存当前控制台模式
	var oldMode uint32
	r1, _, _ := procGetConsoleMode.Call(uintptr(handle), uintptr(unsafe.Pointer(&oldMode)))
	if r1 == 0 {
		// 获取失败，使用简单信号监听
		return startSimpleSignalMonitor(state)
	}

	// 2. 设置新模式：启用 ENABLE_PROCESSED_INPUT 以处理 Ctrl+C
	newMode := oldMode | enableProcessedInput
	// 移除 raw 模式相关标志
	newMode |= enableLineInput | enableEchoInput

	r2, _, _ := procSetConsoleMode.Call(uintptr(handle), uintptr(newMode))
	if r2 == 0 {
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
		// 恢复原始控制台模式
		procSetConsoleMode.Call(uintptr(handle), uintptr(oldMode))
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
