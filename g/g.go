package g

import (
	"fkteams/common"
	"fkteams/log"
	"fkteams/memory"
)

// MemoryManager 全局记忆管理器
var MemoryManager *memory.Manager

// ProcessCleaner 进程级资源清理器，注册的清理函数仅在进程退出时执行。
// 用于终止后台子进程、关闭 SSH 连接等进程生命周期绑定的资源。
var ProcessCleaner = common.NewResourceCleaner()

// RunProcessCleanup 执行所有进程级清理函数
func RunProcessCleanup() {
	if err := ProcessCleaner.ExecuteAndClear(); err != nil {
		log.Printf("[cleanup] 进程资源清理出错: %v", err)
	}
}
