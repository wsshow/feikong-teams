package handler

import (
	"github.com/gin-gonic/gin"
)

// ShutdownHandler 优雅关闭服务
func (rt *Runtime) ShutdownHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !rt.Control.Available() {
			Fail(c, 500, "shutdown not available")
			return
		}
		OK(c, gin.H{"message": "server is shutting down"})
		rt.Control.Shutdown()
	}
}

// RestartHandler 优雅重启服务（关闭后自动启动新进程）
func (rt *Runtime) RestartHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !rt.Control.Available() {
			Fail(c, 500, "restart not available")
			return
		}
		OK(c, gin.H{"message": "server is restarting"})
		rt.Control.Restart()
	}
}
