package handler

import (
	"fkteams/internal/app/appstate"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetMemoryListHandler 获取所有长期记忆条目
func GetMemoryListHandler() gin.HandlerFunc {
	return GetMemoryListHandlerWithState(nil)
}

// GetMemoryListHandlerWithState 获取所有长期记忆条目。
func GetMemoryListHandlerWithState(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		manager := memoryFromState(state)
		if manager == nil {
			OK(c, []any{})
			return
		}
		OK(c, manager.List())
	}
}

// DeleteMemoryHandler 删除指定摘要的记忆条目
func DeleteMemoryHandler() gin.HandlerFunc {
	return DeleteMemoryHandlerWithState(nil)
}

// DeleteMemoryHandlerWithState 删除指定摘要的记忆条目。
func DeleteMemoryHandlerWithState(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		manager := memoryFromState(state)
		if manager == nil {
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

		deleted := manager.Delete(req.Summary)
		if deleted > 0 {
			OK(c, gin.H{"deleted": deleted})
		} else {
			Fail(c, http.StatusNotFound, "未找到匹配的记忆条目")
		}
	}
}

// ClearMemoryHandler 清空所有长期记忆
func ClearMemoryHandler() gin.HandlerFunc {
	return ClearMemoryHandlerWithState(nil)
}

// ClearMemoryHandlerWithState 清空所有长期记忆。
func ClearMemoryHandlerWithState(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		manager := memoryFromState(state)
		if manager == nil {
			Fail(c, http.StatusBadRequest, "长期记忆未启用")
			return
		}
		count := manager.Count()
		manager.Clear()
		OK(c, gin.H{"cleared": count})
	}
}

func memoryFromState(state *appstate.State) appstate.MemoryManager {
	if state == nil {
		return nil
	}
	return state.Memory()
}
