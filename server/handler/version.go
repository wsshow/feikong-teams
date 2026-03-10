package handler

import (
	"fkteams/version"

	"github.com/gin-gonic/gin"
)

// VersionHandler 返回当前版本信息的处理器
func VersionHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		OK(c, version.Get())
	}
}
