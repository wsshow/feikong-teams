package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// PreviewLink 预览链接信息
type PreviewLink struct {
	ID        string `json:"id"`
	FilePath  string `json:"file_path"`
	Password  string `json:"password,omitempty"`
	ExpiresAt int64  `json:"expires_at"`
	CreatedAt int64  `json:"created_at"`
}

// previewLinkStore 预览链接存储
var previewLinkStore = struct {
	sync.RWMutex
	m map[string]*previewLinkEntry
}{m: make(map[string]*previewLinkEntry)}

// previewLinkEntry 存储条目（密码以哈希形式保存）
type previewLinkEntry struct {
	FilePath     string // 文件在 workspace 内的相对路径
	PasswordHash string // 密码 SHA-256 哈希 (空字符串表示无密码)
	ExpiresAt    time.Time
	CreatedAt    time.Time
}

// CreatePreviewLinkHandler 创建文件预览链接
// 参数: file_path(文件相对路径), password(可选密码), expires_in(过期时间,秒,默认86400即1天)
func CreatePreviewLinkHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		baseDir := os.Getenv("FEIKONG_WORKSPACE_DIR")
		if baseDir == "" {
			Fail(c, http.StatusInternalServerError, "FEIKONG_WORKSPACE_DIR 未配置")
			return
		}

		var req struct {
			FilePath  string `json:"file_path" binding:"required"`
			Password  string `json:"password"`
			ExpiresIn int64  `json:"expires_in"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, "参数错误")
			return
		}

		// 校验文件路径
		cleanPath := filepath.Clean(req.FilePath)
		if strings.Contains(cleanPath, "..") {
			Fail(c, http.StatusBadRequest, "无效的文件路径")
			return
		}

		fullPath := filepath.Join(baseDir, cleanPath)
		absBase, _ := filepath.Abs(baseDir)
		absFull, _ := filepath.Abs(fullPath)
		if !strings.HasPrefix(absFull, absBase+string(os.PathSeparator)) {
			Fail(c, http.StatusBadRequest, "无效的文件路径")
			return
		}

		info, err := os.Stat(fullPath)
		if err != nil || info.IsDir() {
			Fail(c, http.StatusNotFound, "文件不存在")
			return
		}

		// 默认过期时间 1 天，最长 30 天
		expiresIn := req.ExpiresIn
		if expiresIn <= 0 {
			expiresIn = 86400
		}
		const maxExpiry = 30 * 24 * 3600
		if expiresIn > maxExpiry {
			expiresIn = maxExpiry
		}

		// 生成链接 ID
		linkID, err := generateLinkID()
		if err != nil {
			Fail(c, http.StatusInternalServerError, "生成链接失败")
			return
		}

		now := time.Now()
		entry := &previewLinkEntry{
			FilePath:  cleanPath,
			ExpiresAt: now.Add(time.Duration(expiresIn) * time.Second),
			CreatedAt: now,
		}

		// 密码哈希
		if req.Password != "" {
			entry.PasswordHash = hashPassword(req.Password)
		}

		previewLinkStore.Lock()
		previewLinkStore.m[linkID] = entry
		previewLinkStore.Unlock()

		OK(c, PreviewLink{
			ID:        linkID,
			FilePath:  cleanPath,
			ExpiresAt: entry.ExpiresAt.Unix(),
			CreatedAt: entry.CreatedAt.Unix(),
		})
	}
}

// PreviewFileHandler 通过预览链接访问文件
// URL: /api/fkteams/preview/:linkId
// Query: password(如果链接设置了密码)
func PreviewFileHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		baseDir := os.Getenv("FEIKONG_WORKSPACE_DIR")
		if baseDir == "" {
			Fail(c, http.StatusInternalServerError, "FEIKONG_WORKSPACE_DIR 未配置")
			return
		}

		linkID := c.Param("linkId")
		if linkID == "" {
			Fail(c, http.StatusBadRequest, "缺少链接 ID")
			return
		}

		// 查找链接
		previewLinkStore.RLock()
		entry, exists := previewLinkStore.m[linkID]
		previewLinkStore.RUnlock()

		if !exists {
			Fail(c, http.StatusNotFound, "链接不存在或已失效")
			return
		}

		// 检查过期
		if time.Now().After(entry.ExpiresAt) {
			// 清理过期链接
			previewLinkStore.Lock()
			delete(previewLinkStore.m, linkID)
			previewLinkStore.Unlock()
			Fail(c, http.StatusGone, "链接已过期")
			return
		}

		// 校验密码
		if entry.PasswordHash != "" {
			password := c.Query("password")
			if password == "" {
				password = c.GetHeader("X-Preview-Password")
			}
			if password == "" {
				c.JSON(http.StatusUnauthorized, Response{
					Code:    1,
					Message: "需要输入访问密码",
					Data:    gin.H{"require_password": true},
				})
				return
			}
			if !verifyPassword(password, entry.PasswordHash) {
				Fail(c, http.StatusForbidden, "密码错误")
				return
			}
		}

		// 读取并返回文件
		fullPath := filepath.Join(baseDir, entry.FilePath)
		file, err := os.Open(fullPath)
		if err != nil {
			Fail(c, http.StatusNotFound, "文件不存在或已被删除")
			return
		}
		defer file.Close()

		info, err := file.Stat()
		if err != nil {
			Fail(c, http.StatusInternalServerError, "获取文件信息失败")
			return
		}

		// 确定 MIME 类型
		contentType := mime.TypeByExtension(filepath.Ext(entry.FilePath))
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		// 设置内联预览（浏览器可直接打开的类型用 inline，否则用 attachment）
		disposition := "inline"
		if !isPreviewable(contentType) {
			disposition = "attachment"
		}
		fileName := filepath.Base(entry.FilePath)
		c.Header("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disposition, fileName))
		c.Header("Content-Type", contentType)
		c.Header("Content-Length", fmt.Sprintf("%d", info.Size()))
		c.Header("Cache-Control", "private, max-age=0")

		c.Status(http.StatusOK)
		io.Copy(c.Writer, file)
	}
}

// DeletePreviewLinkHandler 删除预览链接
func DeletePreviewLinkHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		linkID := c.Param("linkId")
		if linkID == "" {
			Fail(c, http.StatusBadRequest, "缺少链接 ID")
			return
		}

		previewLinkStore.Lock()
		_, exists := previewLinkStore.m[linkID]
		if exists {
			delete(previewLinkStore.m, linkID)
		}
		previewLinkStore.Unlock()

		if !exists {
			Fail(c, http.StatusNotFound, "链接不存在")
			return
		}

		OK(c, nil)
	}
}

// ListPreviewLinksHandler 列出所有预览链接
func ListPreviewLinksHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		previewLinkStore.RLock()
		defer previewLinkStore.RUnlock()

		now := time.Now()
		links := make([]PreviewLink, 0)
		expired := make([]string, 0)

		for id, entry := range previewLinkStore.m {
			if now.After(entry.ExpiresAt) {
				expired = append(expired, id)
				continue
			}
			links = append(links, PreviewLink{
				ID:        id,
				FilePath:  entry.FilePath,
				ExpiresAt: entry.ExpiresAt.Unix(),
				CreatedAt: entry.CreatedAt.Unix(),
			})
		}

		// 异步清理过期链接
		if len(expired) > 0 {
			go func() {
				previewLinkStore.Lock()
				for _, id := range expired {
					delete(previewLinkStore.m, id)
				}
				previewLinkStore.Unlock()
			}()
		}

		OK(c, links)
	}
}

// generateLinkID 生成安全的随机链接 ID
func generateLinkID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// hashPassword 使用 SHA-256 哈希密码
func hashPassword(password string) string {
	h := sha256.Sum256([]byte(password))
	return hex.EncodeToString(h[:])
}

// verifyPassword 校验密码
func verifyPassword(password, hash string) bool {
	h := hashPassword(password)
	return subtle.ConstantTimeCompare([]byte(h), []byte(hash)) == 1
}

// isPreviewable 判断 MIME 类型是否可在浏览器中直接预览
func isPreviewable(contentType string) bool {
	previewTypes := []string{
		"text/", "image/", "audio/", "video/",
		"application/pdf",
		"application/json",
		"application/xml",
		"application/javascript",
	}
	for _, t := range previewTypes {
		if strings.HasPrefix(contentType, t) || contentType == t {
			return true
		}
	}
	return false
}
