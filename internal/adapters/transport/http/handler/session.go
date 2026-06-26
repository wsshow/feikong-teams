package handler

import (
	"errors"
	"fkteams/internal/adapters/storage/file/history"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// SessionInfo 会话信息
type SessionInfo struct {
	SessionID    string    `json:"session_id"`
	Title        string    `json:"title"`
	Status       string    `json:"status"`
	CurrentAgent string    `json:"current_agent,omitempty"`
	Favorite     bool      `json:"favorite,omitempty"`
	ActiveTask   bool      `json:"active_task"` // 是否有内存中的活跃流式任务可订阅
	Size         int64     `json:"size"`
	ModTime      time.Time `json:"mod_time"`
}

// validateSessionID 校验会话 ID 安全性（禁止路径穿越）
func validateSessionID(sessionID string) bool {
	return sessionID != "" &&
		!strings.Contains(sessionID, "..") &&
		!strings.Contains(sessionID, "/") &&
		!strings.Contains(sessionID, "\\")
}

// ListSessionsHandler 列出所有历史会话
func ListSessionsHandler() gin.HandlerFunc {
	return NewRuntime().ListSessionsHandler()
}

func (rt *Runtime) ListSessionsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, err := os.Stat(rt.HistoryDir); os.IsNotExist(err) {
			OK(c, gin.H{"sessions": []SessionInfo{}})
			return
		}

		entries, err := os.ReadDir(rt.HistoryDir)
		if err != nil {
			log.Printf("failed to read history dir: %v", err)
			Fail(c, http.StatusInternalServerError, "failed to read directory")
			return
		}

		files := make([]SessionInfo, 0, len(entries))
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			sessionID := entry.Name()
			sessionDir := filepath.Join(rt.HistoryDir, sessionID)

			// 读取元数据
			title := sessionID
			status := "active"
			meta, metaErr := eventlog.LoadMetadata(sessionDir)
			if metaErr == nil {
				title = meta.Title
				status = meta.Status
			}

			// 获取历史事件日志大小和时间
			histFile := filepath.Join(sessionDir, eventlog.HistoryFileName)
			var size int64
			var modTime time.Time
			if info, err := os.Stat(histFile); err == nil {
				size = info.Size()
				modTime = info.ModTime()
			}
			// 历史事件日志不存在时使用 metadata 时间
			if modTime.IsZero() && metaErr == nil && !meta.UpdatedAt.IsZero() {
				modTime = meta.UpdatedAt
			}

			currentAgent := ""
			favorite := false
			if metaErr == nil {
				currentAgent = meta.CurrentAgent
				favorite = meta.Favorite
			}

			files = append(files, SessionInfo{
				SessionID:    sessionID,
				Title:        title,
				Status:       status,
				CurrentAgent: currentAgent,
				Favorite:     favorite,
				ActiveTask:   rt.Streams.Get(sessionID) != nil,
				Size:         size,
				ModTime:      modTime,
			})
		}

		OK(c, gin.H{"sessions": files})
	}
}

// CreateSessionHandler 创建新会话（仅创建元数据目录）
func CreateSessionHandler() gin.HandlerFunc {
	return NewRuntime().CreateSessionHandler()
}

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

		// 如果前端未提供 session_id，由后端生成 UUID
		if req.SessionID == "" {
			req.SessionID = uuid.New().String()
		}
		if !validateSessionID(req.SessionID) {
			Fail(c, http.StatusBadRequest, "invalid session ID")
			return
		}

		sessionDir := rt.sessionDirPath(req.SessionID)
		if _, err := os.Stat(sessionDir); err == nil {
			// 会话已存在，返回已有的元数据
			currentAgent := ""
			if meta, metaErr := eventlog.LoadMetadata(sessionDir); metaErr == nil {
				currentAgent = meta.CurrentAgent
			}
			OK(c, gin.H{
				"session_id":    req.SessionID,
				"current_agent": currentAgent,
				"message":       "session already exists",
			})
			return
		}

		title := req.Title
		if title == "" {
			title = "未命名会话"
		}
		// 截断标题
		runes := []rune(title)
		if len(runes) > 50 {
			title = string(runes[:50]) + "..."
		}

		now := time.Now()
		meta := &eventlog.SessionMetadata{
			ID:        req.SessionID,
			Title:     title,
			Status:    "idle",
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := eventlog.SaveMetadata(sessionDir, meta); err != nil {
			log.Printf("failed to create session %s: %v", req.SessionID, err)
			Fail(c, http.StatusInternalServerError, "failed to create session")
			return
		}

		OK(c, gin.H{"session_id": req.SessionID, "message": "session created"})
	}
}

// GetSessionHandler 加载指定会话的历史记录
func GetSessionHandler() gin.HandlerFunc {
	return NewRuntime().GetSessionHandler()
}

func (rt *Runtime) GetSessionHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")
		if !validateSessionID(sessionID) {
			Fail(c, http.StatusBadRequest, "invalid session ID")
			return
		}

		stream := rt.Streams.Get(sessionID)
		activeTask := stream != nil && stream.Status() == "processing"

		sessionDir := rt.sessionDirPath(sessionID)
		meta, metaErr := eventlog.LoadMetadata(sessionDir)

		histFile := filepath.Join(sessionDir, eventlog.HistoryFileName)
		recorder := eventlog.NewHistoryRecorder()
		if err := recorder.LoadFromFile(histFile); err != nil {
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
		favorite := false
		if metaErr == nil {
			currentAgent = meta.CurrentAgent
			favorite = meta.Favorite
		}

		OK(c, gin.H{
			"session_id":    sessionID,
			"current_agent": currentAgent,
			"favorite":      favorite,
			"messages":      recorder.GetMessages(),
			"active_task":   activeTask,
		})
	}
}

// DeleteSessionHandler 删除指定的会话目录
func DeleteSessionHandler() gin.HandlerFunc {
	return NewRuntime().DeleteSessionHandler()
}

func (rt *Runtime) DeleteSessionHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")
		if !validateSessionID(sessionID) {
			Fail(c, http.StatusBadRequest, "invalid session ID")
			return
		}

		sessionDir := rt.sessionDirPath(sessionID)
		if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
			Fail(c, http.StatusNotFound, "session not found")
			return
		}

		// 取消该会话的活跃任务并清理缓存
		rt.Streams.CancelAndRemove(sessionID)

		// 清理内存中的会话历史记录
		rt.Sessions.Remove(sessionID)

		if err := os.RemoveAll(sessionDir); err != nil {
			log.Printf("failed to delete session %s: %v", sessionID, err)
			Fail(c, http.StatusInternalServerError, "failed to delete session")
			return
		}

		log.Printf("deleted session directory: %s", sessionID)
		OK(c, gin.H{"message": "session deleted"})
	}
}

// RenameSessionHandler 更新会话的标题
func RenameSessionHandler() gin.HandlerFunc {
	return NewRuntime().RenameSessionHandler()
}

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

		sessionDir := rt.sessionDirPath(req.SessionID)
		meta, err := eventlog.LoadMetadata(sessionDir)
		if err != nil {
			if os.IsNotExist(err) {
				Fail(c, http.StatusNotFound, "session not found")
			} else {
				log.Printf("failed to load metadata: session=%s, err=%v", req.SessionID, err)
				Fail(c, http.StatusInternalServerError, "failed to read metadata")
			}
			return
		}

		meta.Title = req.Title
		meta.UpdatedAt = time.Now()
		if err := eventlog.SaveMetadata(sessionDir, meta); err != nil {
			log.Printf("failed to save metadata: session=%s, err=%v", req.SessionID, err)
			Fail(c, http.StatusInternalServerError, "failed to save metadata")
			return
		}

		log.Printf("renamed session %s title to: %s", req.SessionID, req.Title)
		OK(c, gin.H{
			"message":    "session renamed",
			"session_id": req.SessionID,
			"title":      req.Title,
		})
	}
}

// FavoriteSessionHandler 更新会话收藏状态
func FavoriteSessionHandler() gin.HandlerFunc {
	return NewRuntime().FavoriteSessionHandler()
}

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

		sessionDir := rt.sessionDirPath(req.SessionID)
		meta, err := eventlog.LoadMetadata(sessionDir)
		if err != nil {
			if os.IsNotExist(err) {
				Fail(c, http.StatusNotFound, "session not found")
			} else {
				log.Printf("failed to load metadata: session=%s, err=%v", req.SessionID, err)
				Fail(c, http.StatusInternalServerError, "failed to read metadata")
			}
			return
		}

		meta.Favorite = req.Favorite
		meta.UpdatedAt = time.Now()
		if err := eventlog.SaveMetadata(sessionDir, meta); err != nil {
			log.Printf("failed to save metadata: session=%s, err=%v", req.SessionID, err)
			Fail(c, http.StatusInternalServerError, "failed to save metadata")
			return
		}

		OK(c, gin.H{
			"message":    "session favorite updated",
			"session_id": req.SessionID,
			"favorite":   req.Favorite,
		})
	}
}

// UpdateSessionAgentHandler 更新会话的当前智能体
func UpdateSessionAgentHandler() gin.HandlerFunc {
	return NewRuntime().UpdateSessionAgentHandler()
}

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

		sessionDir := rt.sessionDirPath(req.SessionID)
		meta, err := eventlog.LoadMetadata(sessionDir)
		if err != nil {
			if os.IsNotExist(err) {
				Fail(c, http.StatusNotFound, "session not found")
			} else {
				log.Printf("failed to load metadata: session=%s, err=%v", req.SessionID, err)
				Fail(c, http.StatusInternalServerError, "failed to read metadata")
			}
			return
		}

		meta.CurrentAgent = req.CurrentAgent
		meta.UpdatedAt = time.Now()
		if err := eventlog.SaveMetadata(sessionDir, meta); err != nil {
			log.Printf("failed to save metadata: session=%s, err=%v", req.SessionID, err)
			Fail(c, http.StatusInternalServerError, "failed to save metadata")
			return
		}

		OK(c, gin.H{
			"message":       "agent updated",
			"session_id":    req.SessionID,
			"current_agent": req.CurrentAgent,
		})
	}
}
