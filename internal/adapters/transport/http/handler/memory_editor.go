package handler

import (
	"errors"
	"fkteams/internal/app/appstate"
	appmemory "fkteams/internal/app/memory"
	"github.com/gin-gonic/gin"
	"net/http"
)

type memoryEditor interface {
	SaveEntry(appmemory.EditRequest) (appmemory.MemoryEntry, error)
	DeleteEntry(string) error
}

// SaveMemoryEntryHandler 保存用户创建或编辑的长期记忆。
func SaveMemoryEntryHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		editor, ok := memoryFromState(state).(memoryEditor)
		if !ok {
			Fail(c, http.StatusServiceUnavailable, "长期记忆服务暂不可用")
			return
		}
		var req appmemory.EditRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, "记忆格式错误")
			return
		}
		entry, err := editor.SaveEntry(req)
		if err != nil {
			memoryEditError(c, err)
			return
		}
		OK(c, entry)
	}
}

// DeleteMemoryEntryHandler 根据稳定 ID 删除长期记忆。
func DeleteMemoryEntryHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		editor, ok := memoryFromState(state).(memoryEditor)
		if !ok {
			Fail(c, http.StatusServiceUnavailable, "长期记忆服务暂不可用")
			return
		}
		if err := editor.DeleteEntry(c.Param("id")); err != nil {
			memoryEditError(c, err)
			return
		}
		OK(c, gin.H{"deleted": 1})
	}
}

func memoryEditError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, appmemory.ErrConflict):
		Fail(c, http.StatusConflict, "记忆已变更，请刷新后再修改")
	case errors.Is(err, appmemory.ErrInvalidEntry):
		Fail(c, http.StatusBadRequest, "记忆字段无效或已达到容量上限")
	case errors.Is(err, appmemory.ErrEntryNotFound):
		Fail(c, http.StatusNotFound, "记忆不存在，请刷新后重试")
	default:
		Fail(c, http.StatusInternalServerError, "保存失败，请稍后重试")
	}
}
