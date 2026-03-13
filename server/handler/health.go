package handler

import "github.com/gin-gonic/gin"

// HealthHandler 健康检查处理器
func HealthHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		OK(c, gin.H{"status": "ok"})
	}
}
