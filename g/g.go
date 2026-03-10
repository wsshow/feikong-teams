package g

import (
	"fkteams/common"
	"fkteams/config"
	"fkteams/log"
	"fkteams/memory"
	"fkteams/utils"
)

// Cleaner 资源清理器，支持注册多个清理函数，在退出前统一执行
var Cleaner = common.NewResourceCleaner()

// MemManager 全局记忆管理器
var MemManager *memory.Manager

// Log 全局日志记录器
var Log = NewLog()

func NewLog() *log.Log {
	utils.NotExistToMkdir("./log")
	cfg, _ := config.Get()
	if cfg.Server.LogLevel == "" {
		cfg.Server.LogLevel = "info"
	}
	return log.New("fkteams", cfg.Server.LogLevel)
}
