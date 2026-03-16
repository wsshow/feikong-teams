package handler

import (
	"fkteams/g"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetMemoryListHandler 获取所有长期记忆条目
func GetMemoryListHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if g.MemoryManager == nil {
			OK(c, []any{})
			return
		}
		OK(c, g.MemoryManager.List())
	}
}

// DeleteMemoryHandler 删除指定摘要的记忆条目
func DeleteMemoryHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if g.MemoryManager == nil {
			Fail(c, http.StatusBadRequest, "长期记忆未启用")
			return
		}

		var req struct {
			Summary string `json:"summary" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, "参数错误: summary 不能为空")
			return
		}

		deleted := g.MemoryManager.Delete(req.Summary)
		if deleted > 0 {
			OK(c, gin.H{"deleted": deleted})
		} else {
			Fail(c, http.StatusNotFound, "未找到匹配的记忆条目")
		}
	}
}

// ClearMemoryHandler 清空所有长期记忆
func ClearMemoryHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if g.MemoryManager == nil {
			Fail(c, http.StatusBadRequest, "长期记忆未启用")
			return
		}
		count := g.MemoryManager.Count()
		g.MemoryManager.Clear()
		OK(c, gin.H{"cleared": count})
	}
}
