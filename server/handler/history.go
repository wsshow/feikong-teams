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

// HistoryFileInfo 历史文件信息
type HistoryFileInfo struct {
	Filename    string    `json:"filename"`
	DisplayName string    `json:"display_name"`
	SessionID   string    `json:"session_id"`
	Size        int64     `json:"size"`
	ModTime     time.Time `json:"mod_time"`
}

// ListHistoryFilesHandler 列出所有历史文件
func ListHistoryFilesHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		dir := "./history/chat_history/"

		// 检查目录是否存在
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			c.JSON(http.StatusOK, resp.Success(gin.H{
				"files": []HistoryFileInfo{},
			}))
			return
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			c.JSON(http.StatusInternalServerError, resp.Failure().WithDesc("读取目录失败"))
			return
		}

		files := make([]HistoryFileInfo, 0)
		for _, entry := range entries {
			if !entry.IsDir() {
				info, err := entry.Info()
				if err != nil {
					continue
				}

				filename := entry.Name()
				// 提取 session_id (格式: fkteams_chat_history_SESSION_ID)
				sessionID := extractSessionID(filename)

				files = append(files, HistoryFileInfo{
					Filename:    filename,
					DisplayName: formatDisplayName(filename),
					SessionID:   sessionID,
					Size:        info.Size(),
					ModTime:     info.ModTime(),
				})
			}
		}

		c.JSON(http.StatusOK, resp.Success(gin.H{
			"files": files,
		}))
	}
}

// LoadHistoryFileHandler 加载指定的历史文件
func LoadHistoryFileHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		filename := c.Param("filename")
		if filename == "" {
			c.JSON(http.StatusBadRequest, resp.Failure().WithDesc("文件名不能为空"))
			return
		}

		// 安全检查：防止路径遍历攻击
		if strings.Contains(filename, "..") || strings.Contains(filename, "/") {
			c.JSON(http.StatusBadRequest, resp.Failure().WithDesc("无效的文件名"))
			return
		}

		filePath := filepath.Join("./history/chat_history/", filename)

		// 检查文件是否存在
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, resp.Failure().WithDesc("文件不存在"))
			return
		}

		// 读取并解析历史文件
		var messages []fkevent.AgentMessage
		data, err := os.ReadFile(filePath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, resp.Failure().WithDesc("读取文件失败"))
			return
		}

		if err := json.Unmarshal(data, &messages); err != nil {
			c.JSON(http.StatusInternalServerError, resp.Failure().WithDesc("解析文件失败"))
			return
		}

		c.JSON(http.StatusOK, resp.Success(gin.H{
			"filename":   filename,
			"session_id": extractSessionID(filename),
			"messages":   messages,
		}))
	}
}

// DeleteHistoryFileHandler 删除指定的历史文件
func DeleteHistoryFileHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		filename := c.Param("filename")
		if filename == "" {
			c.JSON(http.StatusBadRequest, resp.Failure().WithDesc("文件名不能为空"))
			return
		}

		// 安全检查：防止路径遍历攻击
		if strings.Contains(filename, "..") || strings.Contains(filename, "/") {
			c.JSON(http.StatusBadRequest, resp.Failure().WithDesc("无效的文件名"))
			return
		}

		filePath := filepath.Join("./history/chat_history/", filename)

		// 删除文件
		if err := os.Remove(filePath); err != nil {
			if os.IsNotExist(err) {
				c.JSON(http.StatusNotFound, resp.Failure().WithDesc("文件不存在"))
			} else {
				c.JSON(http.StatusInternalServerError, resp.Failure().WithDesc("删除文件失败"))
			}
			return
		}

		log.Printf("已删除历史文件: %s", filename)
		c.JSON(http.StatusOK, resp.Success(gin.H{
			"message": "文件已删除",
		}))
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
			c.JSON(http.StatusBadRequest, resp.Failure().WithDesc("请求参数错误"))
			return
		}

		// 安全检查：防止路径遍历攻击
		if strings.Contains(req.OldFilename, "..") || strings.Contains(req.OldFilename, "/") ||
			strings.Contains(req.NewFilename, "..") || strings.Contains(req.NewFilename, "/") {
			c.JSON(http.StatusBadRequest, resp.Failure().WithDesc("无效的文件名"))
			return
		}

		oldPath := filepath.Join("./history/chat_history/", req.OldFilename)
		newPath := filepath.Join("./history/chat_history/", req.NewFilename)

		// 检查旧文件是否存在
		if _, err := os.Stat(oldPath); os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, resp.Failure().WithDesc("原文件不存在"))
			return
		}

		// 检查新文件名是否已存在
		if _, err := os.Stat(newPath); err == nil {
			c.JSON(http.StatusConflict, resp.Failure().WithDesc("目标文件名已存在"))
			return
		}

		// 重命名文件
		if err := os.Rename(oldPath, newPath); err != nil {
			c.JSON(http.StatusInternalServerError, resp.Failure().WithDesc("重命名文件失败"))
			return
		}

		log.Printf("已重命名历史文件: %s -> %s", req.OldFilename, req.NewFilename)
		c.JSON(http.StatusOK, resp.Success(gin.H{
			"message":      "文件已重命名",
			"new_filename": req.NewFilename,
		}))
	}
}

// extractSessionID 从文件名中提取 session_id
func extractSessionID(filename string) string {
	// 格式: fkteams_chat_history_SESSION_ID
	prefix := "fkteams_chat_history_"
	if strings.HasPrefix(filename, prefix) {
		sessionID := strings.TrimPrefix(filename, prefix)
		// 移除可能的扩展名
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

	// 如果是时间戳格式（例如: 20260116_153045），格式化为更友好的显示
	if len(sessionID) == 15 && sessionID[8] == '_' {
		// 尝试解析为时间
		if t, err := time.Parse("20060102_150405", sessionID); err == nil {
			return t.Format("2006-01-02 15:04:05")
		}
	}

	return sessionID
}
