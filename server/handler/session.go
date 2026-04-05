package handler

import (
	"encoding/json"
	"fkteams/common"
	"fkteams/fkevent"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var historyDir = common.SessionsDir()

// SessionInfo 会话信息
type SessionInfo struct {
	SessionID  string    `json:"session_id"`
	Title      string    `json:"title"`
	Status     string    `json:"status"`
	ActiveTask bool      `json:"active_task"` // 是否有内存中的活跃流式任务可订阅
	Size       int64     `json:"size"`
	ModTime    time.Time `json:"mod_time"`
}

// validateSessionID 校验会话 ID 安全性（禁止路径穿越）
func validateSessionID(sessionID string) bool {
	return sessionID != "" &&
		!strings.Contains(sessionID, "..") &&
		!strings.Contains(sessionID, "/") &&
		!strings.Contains(sessionID, "\\")
}

// sessionDirPath 返回会话目录路径
func sessionDirPath(sessionID string) string {
	return filepath.Join(historyDir, filepath.Base(sessionID))
}

// ListSessionsHandler 列出所有历史会话
func ListSessionsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, err := os.Stat(historyDir); os.IsNotExist(err) {
			OK(c, gin.H{"sessions": []SessionInfo{}})
			return
		}

		entries, err := os.ReadDir(historyDir)
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
			sessionDir := filepath.Join(historyDir, sessionID)

			// 读取元数据
			title := sessionID
			status := "active"
			meta, metaErr := fkevent.LoadMetadata(sessionDir)
			if metaErr == nil {
				title = meta.Title
				status = meta.Status
			}

			// 获取 history.json 大小和时间
			histFile := filepath.Join(sessionDir, "history.json")
			var size int64
			var modTime time.Time
			if info, err := os.Stat(histFile); err == nil {
				size = info.Size()
				modTime = info.ModTime()
			}
			// history.json 不存在时使用 metadata 时间
			if modTime.IsZero() && metaErr == nil && !meta.UpdatedAt.IsZero() {
				modTime = meta.UpdatedAt
			}

			files = append(files, SessionInfo{
				SessionID:  sessionID,
				Title:      title,
				Status:     status,
				ActiveTask: globalStreamTasks.get(sessionID) != nil,
				Size:       size,
				ModTime:    modTime,
			})
		}

		OK(c, gin.H{"sessions": files})
	}
}

// CreateSessionHandler 创建新会话（仅创建元数据目录）
func CreateSessionHandler() gin.HandlerFunc {
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

		sessionDir := sessionDirPath(req.SessionID)
		if _, err := os.Stat(sessionDir); err == nil {
			// 会话已存在，直接返回成功
			OK(c, gin.H{"session_id": req.SessionID, "message": "session already exists"})
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
		meta := &fkevent.SessionMetadata{
			ID:        req.SessionID,
			Title:     title,
			Status:    "idle",
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := fkevent.SaveMetadata(sessionDir, meta); err != nil {
			log.Printf("failed to create session %s: %v", req.SessionID, err)
			Fail(c, http.StatusInternalServerError, "failed to create session")
			return
		}

		OK(c, gin.H{"session_id": req.SessionID, "message": "session created"})
	}
}

// GetSessionHandler 加载指定会话的历史记录
func GetSessionHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")
		if !validateSessionID(sessionID) {
			Fail(c, http.StatusBadRequest, "invalid session ID")
			return
		}

		histFile := filepath.Join(sessionDirPath(sessionID), "history.json")
		data, err := os.ReadFile(histFile)
		if err != nil {
			if os.IsNotExist(err) {
				Fail(c, http.StatusNotFound, "session not found")
			} else {
				log.Printf("failed to read history: session=%s, err=%v", sessionID, err)
				Fail(c, http.StatusInternalServerError, "failed to read history")
			}
			return
		}

		var histData fkevent.HistoryData
		if err := json.Unmarshal(data, &histData); err != nil {
			Fail(c, http.StatusInternalServerError, "failed to parse history")
			return
		}

		OK(c, gin.H{
			"session_id": sessionID,
			"messages":   histData.Messages,
		})
	}
}

// DeleteSessionHandler 删除指定的会话目录
func DeleteSessionHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")
		if !validateSessionID(sessionID) {
			Fail(c, http.StatusBadRequest, "invalid session ID")
			return
		}

		sessionDir := sessionDirPath(sessionID)
		if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
			Fail(c, http.StatusNotFound, "session not found")
			return
		}

		// 取消该会话的流式任务并清理缓存
		if task := globalStreamTasks.get(sessionID); task != nil {
			if task.Status == "processing" {
				task.Cancel()
			}
			globalStreamTasks.remove(sessionID)
		}

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

		sessionDir := sessionDirPath(req.SessionID)
		meta, err := fkevent.LoadMetadata(sessionDir)
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
		if err := fkevent.SaveMetadata(sessionDir, meta); err != nil {
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
