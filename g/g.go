package g

import (
	"fkteams/common"
	"fkteams/memory"
)

var Cleaner = common.NewResourceCleaner()

// MemManager 全局记忆管理器
var MemManager *memory.Manager
