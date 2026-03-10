package g

import (
	"fkteams/common"
	"fkteams/memory"
)

// Cleaner 资源清理器，支持注册多个清理函数，在退出前统一执行
var Cleaner = common.NewResourceCleaner()

// MemManager 全局记忆管理器
var MemManager *memory.Manager
