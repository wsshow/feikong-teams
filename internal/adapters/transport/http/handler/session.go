package handler

import (
	"errors"
	"fkteams/internal/adapters/storage/file/history"
	appsession "fkteams/internal/app/session"
	domainsession "fkteams/internal/domain/session"
	"fkteams/internal/runtime/log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

// SessionInfo 会话信息
type SessionInfo struct {
	SessionID    string    `json:"session_id"`
	Title        string    `json:"title"`
	Status       string    `json:"status"`
	Mode         string    `json:"mode,omitempty"`
	CurrentAgent string    `json:"current_agent,omitempty"`
	Favorite     bool      `json:"favorite,omitempty"`
	ActiveTask   bool      `json:"active_task"` // 是否有内存中的活跃流式任务可订阅
	Size         int64     `json:"size"`
	ModTime      time.Time `json:"mod_time"`
}

// validateSessionID 校验会话 ID 安全性（禁止路径穿越）
func validateSessionID(sessionID string) bool {
	return domainsession.ValidID(sessionID)
}

// OpenAssistantHandler 打开或首次创建常驻助手会话。
func (rt *Runtime) OpenAssistantHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		metadata, err := rt.SessionService.OpenAssistant(c.Request.Context())
		if err != nil {
			FailError(c, err)
			return
		}
		OK(c, gin.H{"session_id": metadata.ID, "title": metadata.Title, "current_agent": metadata.CurrentAgent, "mode": metadata.Mode})
	}
}

// ListSessionsHandler 列出所有历史会话

func (rt *Runtime) ListSessionsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		records, err := rt.SessionService.List(c.Request.Context())
		if err != nil {
			log.Printf("failed to list sessions: %v", err)
			FailError(c, err)
			return
		}
		files := make([]SessionInfo, 0, len(records))
		for _, record := range records {
			meta := record.Metadata
			status := string(meta.Status)
			activeTask := rt.sessionHasProcessingStream(meta.ID)
			if activeTask {
				status = string(domainsession.StatusProcessing)
			}
			files = append(files, SessionInfo{
				SessionID:    meta.ID,
				Title:        meta.Title,
				Status:       status,
				Mode:         meta.Mode,
				CurrentAgent: meta.CurrentAgent,
				Favorite:     meta.Favorite,
				ActiveTask:   activeTask,
				Size:         record.Size,
				ModTime:      record.ModTime,
			})
		}

		OK(c, gin.H{"sessions": files})
	}
}

func (rt *Runtime) sessionHasProcessingStream(sessionID string) bool {
	stream := rt.Streams.Get(sessionID)
	return stream != nil && stream.Status() == "processing"
}

// CreateSessionHandler 创建新会话（仅创建元数据目录）

func (rt *Runtime) CreateSessionHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			SessionID string `json:"session_id"`
			Title     string `json:"title"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, "invalid request body")
			return
		}

		metadata, created, err := rt.SessionService.Create(c.Request.Context(), appsession.CreateRequest{
			SessionID: req.SessionID,
			Title:     req.Title,
		})
		if err != nil {
			FailError(c, err)
			return
		}
		if !created {
			OK(c, gin.H{
				"session_id":    metadata.ID,
				"current_agent": metadata.CurrentAgent,
				"message":       "session already exists",
			})
			return
		}
		Created(c, gin.H{"session_id": metadata.ID, "message": "session created"})
	}
}

// GetSessionHandler 加载指定会话的历史记录

func (rt *Runtime) GetSessionHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")
		if !validateSessionID(sessionID) {
			Fail(c, http.StatusBadRequest, "invalid session ID")
			return
		}

		stream := rt.Streams.Get(sessionID)
		activeTask := stream != nil && stream.Status() == "processing"
		queue, ok := rt.queueForSessionResponse(c, sessionID)
		if !ok {
			return
		}

		sessionDir := rt.sessionDirPath(sessionID)
		meta, metaErr := rt.SessionService.Get(c.Request.Context(), sessionID)

		transcript, err := eventlog.LoadSessionTranscriptRecords(sessionDir, rt.Documents)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				if !activeTask && metaErr != nil {
					Fail(c, http.StatusNotFound, "session not found")
					return
				}
			} else {
				log.Printf("failed to load history: session=%s, err=%v", sessionID, err)
				Fail(c, http.StatusInternalServerError, "failed to read history")
				return
			}
		}

		currentAgent := ""
		mode := ""
		favorite := false
		if metaErr == nil {
			mode = meta.Mode
			currentAgent = meta.CurrentAgent
			favorite = meta.Favorite
		}

		OK(c, gin.H{
			"session_id":    sessionID,
			"mode":          mode,
			"current_agent": currentAgent,
			"favorite":      favorite,
			"events":        rt.transcriptRecordsToChatEvents(sessionID, transcript),
			"queue":         queue,
			"active_task":   activeTask,
		})
	}
}

// DeleteSessionHandler 删除指定的会话目录

func (rt *Runtime) DeleteSessionHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")
		if !validateSessionID(sessionID) {
			Fail(c, http.StatusBadRequest, "invalid session ID")
			return
		}
		unlockSession := rt.lockSessionOperation(sessionID)
		defer unlockSession()

		if stream := rt.Streams.Get(sessionID); stream != nil && stream.Status() == "processing" {
			Fail(c, http.StatusConflict, "session is active")
			return
		}
		if !rt.Sessions.Remove(sessionID) {
			Fail(c, http.StatusConflict, "session is active")
			return
		}
		if err := rt.SessionService.Delete(c.Request.Context(), sessionID); err != nil {
			FailError(c, err)
			return
		}
		rt.Streams.CancelAndRemove(sessionID)
		log.Printf("deleted session directory: %s", sessionID)
		OK(c, gin.H{"message": "session deleted"})
	}
}

// RenameSessionHandler 更新会话的标题

func (rt *Runtime) RenameSessionHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			SessionID string `json:"session_id" binding:"required"`
			Title     string `json:"title" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, "invalid request body")
			return
		}
		if !validateSessionID(req.SessionID) {
			Fail(c, http.StatusBadRequest, "invalid session ID")
			return
		}

		meta, err := rt.SessionService.Update(c.Request.Context(), appsession.UpdateRequest{SessionID: req.SessionID, Title: &req.Title})
		if err != nil {
			FailError(c, err)
			return
		}

		log.Printf("renamed session %s title to: %s", req.SessionID, req.Title)
		OK(c, gin.H{
			"message":    "session renamed",
			"session_id": req.SessionID,
			"title":      meta.Title,
		})
	}
}

// FavoriteSessionHandler 更新会话收藏状态

func (rt *Runtime) FavoriteSessionHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			SessionID string `json:"session_id" binding:"required"`
			Favorite  bool   `json:"favorite"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, "invalid request body")
			return
		}
		if !validateSessionID(req.SessionID) {
			Fail(c, http.StatusBadRequest, "invalid session ID")
			return
		}

		meta, err := rt.SessionService.Update(c.Request.Context(), appsession.UpdateRequest{SessionID: req.SessionID, Favorite: &req.Favorite})
		if err != nil {
			FailError(c, err)
			return
		}

		OK(c, gin.H{
			"message":    "session favorite updated",
			"session_id": req.SessionID,
			"favorite":   meta.Favorite,
		})
	}
}

// UpdateSessionAgentHandler 更新会话的当前智能体

func (rt *Runtime) UpdateSessionAgentHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			SessionID    string `json:"session_id" binding:"required"`
			CurrentAgent string `json:"current_agent"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, "invalid request body")
			return
		}
		if !validateSessionID(req.SessionID) {
			Fail(c, http.StatusBadRequest, "invalid session ID")
			return
		}

		meta, err := rt.SessionService.Update(c.Request.Context(), appsession.UpdateRequest{SessionID: req.SessionID, CurrentAgent: &req.CurrentAgent})
		if err != nil {
			FailError(c, err)
			return
		}

		OK(c, gin.H{
			"message":       "agent updated",
			"session_id":    req.SessionID,
			"current_agent": meta.CurrentAgent,
		})
	}
}

// UpdateSessionHandler 使用资源路径更新一个或多个会话字段。
func (rt *Runtime) UpdateSessionHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")
		if !validateSessionID(sessionID) {
			Fail(c, http.StatusBadRequest, "invalid session ID")
			return
		}
		var req struct {
			Title        *string `json:"title"`
			Favorite     *bool   `json:"favorite"`
			Mode         *string `json:"mode"`
			CurrentAgent *string `json:"current_agent"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, "invalid request body")
			return
		}
		metadata, err := rt.SessionService.Update(c.Request.Context(), appsession.UpdateRequest{
			SessionID:    sessionID,
			Title:        req.Title,
			Favorite:     req.Favorite,
			Mode:         req.Mode,
			CurrentAgent: req.CurrentAgent,
		})
		if err != nil {
			FailError(c, err)
			return
		}
		OK(c, metadata)
	}
}
