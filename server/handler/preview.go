package handler

import (
	"archive/zip"
	"crypto/rand"
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
	"golang.org/x/crypto/bcrypt"
)

// PreviewLink 预览链接信息
type PreviewLink struct {
	ID        string   `json:"id"`
	FilePath  string   `json:"file_path"`
	FilePaths []string `json:"file_paths,omitempty"`
	Password  string   `json:"password,omitempty"`
	ExpiresAt int64    `json:"expires_at"`
	CreatedAt int64    `json:"created_at"`
}

// previewLinkStore 预览链接存储
var previewLinkStore = struct {
	sync.RWMutex
	m map[string]*previewLinkEntry
}{m: make(map[string]*previewLinkEntry)}

// previewLinkEntry 存储条目
type previewLinkEntry struct {
	FilePaths    []string // 文件在 workspace 内的相对路径列表
	PasswordHash string   // bcrypt 哈希 (空字符串表示无密码)
	ExpiresAt    time.Time
	CreatedAt    time.Time
}

// CreatePreviewLinkHandler 创建文件预览链接
// 参数: file_path(单文件路径) 或 file_paths(多文件路径数组), password(可选密码), expires_in(过期时间,秒)
func CreatePreviewLinkHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		baseDir := os.Getenv("FEIKONG_WORKSPACE_DIR")
		if baseDir == "" {
			Fail(c, http.StatusInternalServerError, "FEIKONG_WORKSPACE_DIR 未配置")
			return
		}

		var req struct {
			FilePath  string   `json:"file_path"`
			FilePaths []string `json:"file_paths"`
			Password  string   `json:"password"`
			ExpiresIn int64    `json:"expires_in"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, "参数错误")
			return
		}

		// 合并 file_path 和 file_paths
		paths := req.FilePaths
		if req.FilePath != "" && len(paths) == 0 {
			paths = []string{req.FilePath}
		}
		if len(paths) == 0 {
			Fail(c, http.StatusBadRequest, "缺少文件路径")
			return
		}

		absBase, _ := filepath.Abs(baseDir)

		// 校验所有路径
		var cleanPaths []string
		for _, p := range paths {
			cleanPath := filepath.Clean(p)
			if strings.Contains(cleanPath, "..") {
				Fail(c, http.StatusBadRequest, "无效的文件路径")
				return
			}
			fullPath := filepath.Join(baseDir, cleanPath)
			absFull, _ := filepath.Abs(fullPath)
			if !strings.HasPrefix(absFull, absBase+string(os.PathSeparator)) {
				Fail(c, http.StatusBadRequest, "无效的文件路径")
				return
			}
			if _, err := os.Stat(fullPath); err != nil {
				Fail(c, http.StatusNotFound, fmt.Sprintf("文件不存在: %s", cleanPath))
				return
			}
			cleanPaths = append(cleanPaths, cleanPath)
		}

		// 默认过期时间 1 天，最长 30 天；-1 表示永不过期
		expiresIn := req.ExpiresIn
		if expiresIn == 0 {
			expiresIn = 86400
		}
		const maxExpiry = 30 * 24 * 3600
		if expiresIn > 0 && expiresIn > maxExpiry {
			expiresIn = maxExpiry
		}

		linkID, err := generateLinkID()
		if err != nil {
			Fail(c, http.StatusInternalServerError, "生成链接失败")
			return
		}

		now := time.Now()
		var expiresAt time.Time
		if expiresIn >= 0 {
			expiresAt = now.Add(time.Duration(expiresIn) * time.Second)
		}
		entry := &previewLinkEntry{
			FilePaths: cleanPaths,
			ExpiresAt: expiresAt,
			CreatedAt: now,
		}

		if req.Password != "" {
			h := hashPassword(req.Password)
			if h == "" {
				Fail(c, http.StatusInternalServerError, "密码处理失败")
				return
			}
			entry.PasswordHash = h
		}

		previewLinkStore.Lock()
		previewLinkStore.m[linkID] = entry
		previewLinkStore.Unlock()

		// 响应保持 file_path 兼容
		filePath := cleanPaths[0]
		if len(cleanPaths) > 1 {
			filePath = fmt.Sprintf("%d 个文件", len(cleanPaths))
		}

		OK(c, PreviewLink{
			ID:        linkID,
			FilePath:  filePath,
			FilePaths: cleanPaths,
			ExpiresAt: expiresAtUnix(entry.ExpiresAt),
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

		previewLinkStore.RLock()
		entry, exists := previewLinkStore.m[linkID]
		previewLinkStore.RUnlock()

		if !exists {
			Fail(c, http.StatusNotFound, "链接不存在或已失效")
			return
		}

		if !entry.ExpiresAt.IsZero() && time.Now().After(entry.ExpiresAt) {
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

		// 多文件或包含目录 → zip 打包下载
		if len(entry.FilePaths) > 1 || isDir(filepath.Join(baseDir, entry.FilePaths[0])) {
			c.Header("Content-Type", "application/zip")
			c.Header("Content-Disposition", `attachment; filename="share.zip"`)
			c.Status(http.StatusOK)

			zw := zip.NewWriter(c.Writer)
			defer zw.Close()

			for _, fp := range entry.FilePaths {
				fullPath := filepath.Join(baseDir, fp)
				info, err := os.Stat(fullPath)
				if err != nil {
					continue
				}
				if !info.IsDir() {
					writeFileToZip(zw, fullPath, fp)
					continue
				}
				// 目录递归写入
				_ = filepath.WalkDir(fullPath, func(path string, d os.DirEntry, err error) error {
					if err != nil || strings.HasPrefix(d.Name(), ".") {
						if d != nil && d.IsDir() {
							return filepath.SkipDir
						}
						return nil
					}
					rel, err := filepath.Rel(filepath.Dir(fullPath), path)
					if err != nil {
						return nil
					}
					if d.IsDir() {
						_, _ = zw.Create(rel + "/")
						return nil
					}
					writeFileToZip(zw, path, rel)
					return nil
				})
			}
			return
		}

		// 单文件预览
		filePath := entry.FilePaths[0]
		fullPath := filepath.Join(baseDir, filePath)
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

		contentType := mime.TypeByExtension(filepath.Ext(filePath))
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		disposition := "inline"
		if !isPreviewable(contentType) {
			disposition = "attachment"
		}
		fileName := filepath.Base(filePath)
		safeFileName := strings.NewReplacer(`"`, `\"`, "\r", "", "\n", "").Replace(fileName)
		c.Header("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disposition, safeFileName))
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

// PreviewInfoHandler 获取预览链接的文件信息（不需要密码）
// 返回文件名、大小、类型、是否需要密码、是否可预览等
func PreviewInfoHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		baseDir := os.Getenv("FEIKONG_WORKSPACE_DIR")

		linkID := c.Param("linkId")
		if linkID == "" {
			Fail(c, http.StatusBadRequest, "缺少链接 ID")
			return
		}

		previewLinkStore.RLock()
		entry, exists := previewLinkStore.m[linkID]
		previewLinkStore.RUnlock()

		if !exists {
			Fail(c, http.StatusNotFound, "链接不存在或已失效")
			return
		}

		if !entry.ExpiresAt.IsZero() && time.Now().After(entry.ExpiresAt) {
			previewLinkStore.Lock()
			delete(previewLinkStore.m, linkID)
			previewLinkStore.Unlock()
			Fail(c, http.StatusGone, "链接已过期")
			return
		}

		fileName := filepath.Base(entry.FilePaths[0])
		contentType := mime.TypeByExtension(filepath.Ext(entry.FilePaths[0]))
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		isMulti := len(entry.FilePaths) > 1 || isDir(filepath.Join(baseDir, entry.FilePaths[0]))

		// 获取文件大小
		var fileSize int64
		if !isMulti {
			if info, err := os.Stat(filepath.Join(baseDir, entry.FilePaths[0])); err == nil {
				fileSize = info.Size()
			}
		}

		// 构建文件列表信息
		var fileList []gin.H
		for _, fp := range entry.FilePaths {
			fullPath := filepath.Join(baseDir, fp)
			info, err := os.Stat(fullPath)
			fInfo := gin.H{
				"path":   fp,
				"name":   filepath.Base(fp),
				"is_dir": err == nil && info.IsDir(),
			}
			if err == nil {
				fInfo["size"] = info.Size()
			}
			fileList = append(fileList, fInfo)
		}

		OK(c, gin.H{
			"file_name":        fileName,
			"file_size":        fileSize,
			"file_count":       len(entry.FilePaths),
			"files":            fileList,
			"content_type":     contentType,
			"require_password": entry.PasswordHash != "",
			"previewable":      !isMulti && isPreviewable(contentType),
			"expires_at":       expiresAtUnix(entry.ExpiresAt),
		})
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
			if !entry.ExpiresAt.IsZero() && now.After(entry.ExpiresAt) {
				expired = append(expired, id)
				continue
			}
			filePath := entry.FilePaths[0]
			if len(entry.FilePaths) > 1 {
				filePath = fmt.Sprintf("%d 个文件", len(entry.FilePaths))
			}
			links = append(links, PreviewLink{
				ID:        id,
				FilePath:  filePath,
				FilePaths: entry.FilePaths,
				ExpiresAt: expiresAtUnix(entry.ExpiresAt),
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

// hashPassword 使用 bcrypt 哈希密码
func hashPassword(password string) string {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return ""
	}
	return string(h)
}

// verifyPassword 校验密码
func verifyPassword(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// expiresAtUnix 将过期时间转为 Unix 时间戳，零值（永不过期）返回 0
func expiresAtUnix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func writeFileToZip(zw *zip.Writer, fullPath, relPath string) {
	fi, err := os.Stat(fullPath)
	if err != nil {
		return
	}
	header, err := zip.FileInfoHeader(fi)
	if err != nil {
		return
	}
	header.Name = relPath
	header.Method = zip.Deflate
	w, err := zw.CreateHeader(header)
	if err != nil {
		return
	}
	f, err := os.Open(fullPath)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = io.Copy(w, f)
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
