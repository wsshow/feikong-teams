package handler

import (
	"encoding/json"
	"fkteams/fkevent"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const historyDir = "./history/chat_history/"

// HistoryFileInfo 历史文件信息
type HistoryFileInfo struct {
	Filename    string    `json:"filename"`
	DisplayName string    `json:"display_name"`
	SessionID   string    `json:"session_id"`
	Size        int64     `json:"size"`
	ModTime     time.Time `json:"mod_time"`
}

// validateFilename 校验文件名安全性
func validateFilename(filename string) bool {
	return filename != "" && !strings.Contains(filename, "..") && !strings.Contains(filename, "/")
}

// ListHistoryFilesHandler 列出所有历史文件
func ListHistoryFilesHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, err := os.Stat(historyDir); os.IsNotExist(err) {
			OK(c, gin.H{"files": []HistoryFileInfo{}})
			return
		}

		entries, err := os.ReadDir(historyDir)
		if err != nil {
			Fail(c, http.StatusInternalServerError, "failed to read directory")
			return
		}

		files := make([]HistoryFileInfo, 0, len(entries))
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			filename := entry.Name()
			files = append(files, HistoryFileInfo{
				Filename:    filename,
				DisplayName: formatDisplayName(filename),
				SessionID:   extractSessionID(filename),
				Size:        info.Size(),
				ModTime:     info.ModTime(),
			})
		}

		OK(c, gin.H{"files": files})
	}
}

// LoadHistoryFileHandler 加载指定的历史文件
func LoadHistoryFileHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		filename := c.Param("filename")
		if !validateFilename(filename) {
			Fail(c, http.StatusBadRequest, "invalid filename")
			return
		}

		filePath := filepath.Join(historyDir, filename)
		data, err := os.ReadFile(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				Fail(c, http.StatusNotFound, "file not found")
			} else {
				Fail(c, http.StatusInternalServerError, "failed to read file")
			}
			return
		}

		var messages []fkevent.AgentMessage
		if err := json.Unmarshal(data, &messages); err != nil {
			Fail(c, http.StatusInternalServerError, "failed to parse file")
			return
		}

		OK(c, gin.H{
			"filename":   filename,
			"session_id": extractSessionID(filename),
			"messages":   messages,
		})
	}
}

// DeleteHistoryFileHandler 删除指定的历史文件
func DeleteHistoryFileHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		filename := c.Param("filename")
		if !validateFilename(filename) {
			Fail(c, http.StatusBadRequest, "invalid filename")
			return
		}

		filePath := filepath.Join(historyDir, filename)
		if err := os.Remove(filePath); err != nil {
			if os.IsNotExist(err) {
				Fail(c, http.StatusNotFound, "file not found")
			} else {
				Fail(c, http.StatusInternalServerError, "failed to delete file")
			}
			return
		}

		log.Printf("deleted history file: %s", filename)
		OK(c, gin.H{"message": "file deleted"})
	}
}

// RenameHistoryFileHandler 重命名历史文件
func RenameHistoryFileHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			OldFilename string `json:"old_filename" binding:"required"`
			NewFilename string `json:"new_filename" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, "invalid request body")
			return
		}
		if !validateFilename(req.OldFilename) || !validateFilename(req.NewFilename) {
			Fail(c, http.StatusBadRequest, "invalid filename")
			return
		}

		oldPath := filepath.Join(historyDir, req.OldFilename)
		newPath := filepath.Join(historyDir, req.NewFilename)

		if _, err := os.Stat(oldPath); os.IsNotExist(err) {
			Fail(c, http.StatusNotFound, "source file not found")
			return
		}
		if _, err := os.Stat(newPath); err == nil {
			Fail(c, http.StatusConflict, "target filename already exists")
			return
		}

		if err := os.Rename(oldPath, newPath); err != nil {
			Fail(c, http.StatusInternalServerError, "failed to rename file")
			return
		}

		log.Printf("renamed history file: %s -> %s", req.OldFilename, req.NewFilename)
		OK(c, gin.H{
			"message":      "file renamed",
			"new_filename": req.NewFilename,
		})
	}
}

// extractSessionID 从文件名中提取 session_id
func extractSessionID(filename string) string {
	prefix := "fkteams_chat_history_"
	if strings.HasPrefix(filename, prefix) {
		sessionID := strings.TrimPrefix(filename, prefix)
		if idx := strings.LastIndex(sessionID, "."); idx > 0 {
			sessionID = sessionID[:idx]
		}
		return sessionID
	}
	return filename
}

// formatDisplayName 格式化显示名称
func formatDisplayName(filename string) string {
	sessionID := extractSessionID(filename)
	if len(sessionID) == 15 && sessionID[8] == '_' {
		if t, err := time.Parse("20060102_150405", sessionID); err == nil {
			return t.Format("2006-01-02 15:04:05")
		}
	}
	return sessionID
}
