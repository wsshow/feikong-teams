package handler

import (
	"fkteams/lifecycle"

	"github.com/gin-gonic/gin"
)

// ShutdownHandler 优雅关闭服务
func ShutdownHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !lifecycle.IsShutdownAvailable() {
			Fail(c, 500, "shutdown not available")
			return
		}
		OK(c, gin.H{"message": "server is shutting down"})
		lifecycle.TriggerShutdown()
	}
}

// RestartHandler 优雅重启服务（关闭后自动启动新进程）
func RestartHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !lifecycle.IsShutdownAvailable() {
			Fail(c, 500, "restart not available")
			return
		}
		OK(c, gin.H{"message": "server is restarting"})
		lifecycle.TriggerRestart()
	}
}
