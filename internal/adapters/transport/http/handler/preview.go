package handler

import (
	"archive/zip"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	sharestore "fkteams/internal/adapters/storage/file/share"
	"fkteams/internal/app/appdata"
	"fkteams/internal/runtime/log"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// PreviewLink 预览链接信息
type PreviewLink struct {
	ID          string   `json:"id"`
	FilePath    string   `json:"file_path"`
	FilePaths   []string `json:"file_paths,omitempty"`
	HasPassword bool     `json:"has_password"`
	ExpiresAt   int64    `json:"expires_at"`
	CreatedAt   int64    `json:"created_at"`
}

func collectPreviewPaths(baseDir string, requested []string) ([]string, []string, error) {
	cleanPaths := make([]string, 0, len(requested))
	resources := make(map[string]struct{})
	seenRoots := make(map[string]struct{})

	for _, requestedPath := range requested {
		resolved, info, err := resolveWorkspaceEntryNoSymlinks(baseDir, requestedPath)
		if err != nil {
			return nil, nil, err
		}
		if _, exists := seenRoots[resolved.RelPath]; !exists {
			seenRoots[resolved.RelPath] = struct{}{}
			cleanPaths = append(cleanPaths, resolved.RelPath)
		}

		if !info.IsDir() {
			if !info.Mode().IsRegular() {
				return nil, nil, fmt.Errorf("shared path is not a regular file")
			}
			resources[resolved.RelPath] = struct{}{}
			if len(resources) > sharestore.MaxPreviewResourceFiles {
				return nil, nil, fmt.Errorf("share contains too many files")
			}
			continue
		}

		err = filepath.WalkDir(resolved.AbsPath, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if path != resolved.AbsPath && strings.HasPrefix(entry.Name(), ".") {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("symbolic links are not allowed in shared directories")
			}
			if entry.IsDir() {
				return nil
			}
			entryInfo, err := entry.Info()
			if err != nil {
				return err
			}
			if !entryInfo.Mode().IsRegular() {
				return fmt.Errorf("shared directories may only contain regular files")
			}
			relativePath, err := filepath.Rel(resolved.BaseAbs, path)
			if err != nil {
				return err
			}
			resources[relativePath] = struct{}{}
			if len(resources) > sharestore.MaxPreviewResourceFiles {
				return fmt.Errorf("shared directory contains too many files")
			}
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}

	resourcePaths := make([]string, 0, len(resources))
	for resourcePath := range resources {
		resourcePaths = append(resourcePaths, resourcePath)
	}
	sort.Strings(resourcePaths)
	return cleanPaths, resourcePaths, nil
}

// CreatePreviewLinkHandler 创建文件预览链接
// 参数: file_path(单文件路径) 或 file_paths(多文件路径数组), password(可选密码), expires_in(过期时间,秒)

// CreatePreviewLinkHandler 创建当前 HTTP runtime 的文件预览链接。
func (rt *Runtime) CreatePreviewLinkHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		baseDir := appdata.WorkspaceDir()

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

		cleanPaths, resourcePaths, err := collectPreviewPaths(baseDir, paths)
		if err != nil {
			status := http.StatusBadRequest
			if os.IsNotExist(err) {
				status = http.StatusNotFound
			}
			Fail(c, status, err.Error())
			return
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
		entry := sharestore.NewPreviewEntry(cleanPaths, resourcePaths, "", expiresAt, now)

		if req.Password != "" {
			h := hashPassword(req.Password)
			if h == "" {
				Fail(c, http.StatusInternalServerError, "密码处理失败")
				return
			}
			entry.PasswordHash = h
		}

		store := rt.PreviewLinks
		if err := store.Put(linkID, entry); err != nil {
			log.Printf("failed to persist preview link: id=%s, err=%v", linkID, err)
			Fail(c, http.StatusInternalServerError, "failed to save preview link")
			return
		}

		// 响应保持 file_path 兼容
		filePath := cleanPaths[0]
		if len(cleanPaths) > 1 {
			filePath = fmt.Sprintf("%d 个文件", len(cleanPaths))
		}

		OK(c, PreviewLink{
			ID:          linkID,
			FilePath:    filePath,
			FilePaths:   cleanPaths,
			HasPassword: entry.PasswordHash != "",
			ExpiresAt:   sharestore.ExpiresAtUnix(entry.ExpiresAt),
			CreatedAt:   entry.CreatedAt.Unix(),
		})
	}
}

// PreviewFileHandler 通过当前 HTTP runtime 的预览链接访问文件。
func (rt *Runtime) PreviewFileHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		baseDir := appdata.WorkspaceDir()

		linkID := c.Param("linkId")
		if linkID == "" {
			Fail(c, http.StatusBadRequest, "缺少链接 ID")
			return
		}

		store := rt.PreviewLinks
		entry, exists := store.Lookup(linkID)

		if !exists {
			Fail(c, http.StatusNotFound, "链接不存在或已失效")
			return
		}

		if !entry.ExpiresAt.IsZero() && time.Now().After(entry.ExpiresAt) {
			if _, err := store.Delete(linkID); err != nil {
				log.Printf("failed to remove expired preview link: id=%s, err=%v", linkID, err)
			}
			Fail(c, http.StatusGone, "链接已过期")
			return
		}

		if !authorizePreviewPassword(c, linkID, entry.PasswordHash, c.GetHeader("X-Preview-Password")) {
			return
		}

		// 多文件或包含目录 → zip 打包下载
		_, firstInfo, err := resolveWorkspaceEntryNoSymlinks(baseDir, entry.FilePaths[0])
		if err != nil {
			Fail(c, http.StatusNotFound, "shared file is unavailable")
			return
		}
		if len(entry.FilePaths) > 1 || firstInfo.IsDir() {
			resourcePaths, err := validatedPreviewResources(baseDir, entry)
			if err != nil {
				Fail(c, http.StatusConflict, err.Error())
				return
			}
			c.Header("Content-Type", "application/zip")
			c.Header("Content-Disposition", `attachment; filename="share.zip"`)
			c.Status(http.StatusOK)

			zw := zip.NewWriter(c.Writer)
			defer zw.Close()

			for _, resourcePath := range resourcePaths {
				if err := writePreviewFileToZip(zw, baseDir, resourcePath); err != nil {
					log.Printf("failed to add preview resource to zip: path=%s, err=%v", resourcePath, err)
					return
				}
			}
			return
		}

		// 单文件预览
		filePath := entry.FilePaths[0]
		file, info, err := openWorkspaceRegularFileNoSymlinks(baseDir, filePath)
		if err != nil {
			Fail(c, http.StatusNotFound, "文件不存在或已被删除")
			return
		}
		defer file.Close()

		contentType := previewContentType(filePath)

		disposition := "inline"
		if c.Query("download") == "1" {
			disposition = "attachment"
		}
		fileName := filepath.Base(filePath)
		safeFileName := strings.NewReplacer(`"`, `\"`, "\r", "", "\n", "").Replace(fileName)
		c.Header("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disposition, safeFileName))
		c.Header("Content-Type", contentType)
		serveOpenedFileContent(c, file, filepath.Join(baseDir, filePath), info)
	}
}

// DeletePreviewLinkHandler 删除预览链接

// DeletePreviewLinkHandler 删除当前 HTTP runtime 的预览链接。
func (rt *Runtime) DeletePreviewLinkHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		linkID := c.Param("linkId")
		if linkID == "" {
			Fail(c, http.StatusBadRequest, "缺少链接 ID")
			return
		}

		store := rt.PreviewLinks
		exists, err := store.Delete(linkID)
		if !exists {
			Fail(c, http.StatusNotFound, "链接不存在")
			return
		}

		if err != nil {
			log.Printf("failed to persist preview link deletion: id=%s, err=%v", linkID, err)
			Fail(c, http.StatusInternalServerError, "failed to delete preview link")
			return
		}
		OK(c, nil)
	}
}

// PreviewAuthHandler 校验分享密码并签发短期 HttpOnly 预览凭证。
func (rt *Runtime) PreviewAuthHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		linkID := c.Param("linkId")
		store := rt.PreviewLinks
		entry, exists := store.Lookup(linkID)
		if !exists {
			Fail(c, http.StatusNotFound, "preview link not found")
			return
		}
		if !entry.ExpiresAt.IsZero() && time.Now().After(entry.ExpiresAt) {
			if _, err := store.Delete(linkID); err != nil {
				log.Printf("failed to remove expired preview link: id=%s, err=%v", linkID, err)
			}
			Fail(c, http.StatusGone, "preview link expired")
			return
		}

		var req struct {
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, "invalid preview authentication request")
			return
		}
		if !authorizePreviewPassword(c, linkID, entry.PasswordHash, req.Password) {
			return
		}
		OK(c, gin.H{"authenticated": true})
	}
}

// PreviewInfoHandler 获取当前 HTTP runtime 的预览链接文件信息。
func (rt *Runtime) PreviewInfoHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		baseDir := appdata.WorkspaceDir()

		linkID := c.Param("linkId")
		if linkID == "" {
			Fail(c, http.StatusBadRequest, "缺少链接 ID")
			return
		}

		store := rt.PreviewLinks
		entry, exists := store.Lookup(linkID)

		if !exists {
			Fail(c, http.StatusNotFound, "链接不存在或已失效")
			return
		}

		if !entry.ExpiresAt.IsZero() && time.Now().After(entry.ExpiresAt) {
			if _, err := store.Delete(linkID); err != nil {
				log.Printf("failed to remove expired preview link: id=%s, err=%v", linkID, err)
			}
			Fail(c, http.StatusGone, "链接已过期")
			return
		}
		_, primaryInfo, err := resolveWorkspaceEntryNoSymlinks(baseDir, entry.FilePaths[0])
		if err != nil {
			Fail(c, http.StatusNotFound, "shared file is unavailable")
			return
		}
		isMulti := len(entry.FilePaths) > 1 || primaryInfo.IsDir()

		// 密码保护链接：返回渲染所需的基本信息，但不泄露完整路径
		if entry.PasswordHash != "" {
			fileName := filepath.Base(entry.FilePaths[0])
			contentType := previewContentType(entry.FilePaths[0])
			OK(c, gin.H{
				"file_name":        fileName,
				"file_count":       len(entry.FilePaths),
				"content_type":     contentType,
				"require_password": true,
				"authorized":       hasValidPreviewGrant(c, linkID, entry.PasswordHash),
				"previewable":      !isMulti && isPreviewable(contentType),
				"expires_at":       sharestore.ExpiresAtUnix(entry.ExpiresAt),
			})
			return
		}

		fileName := filepath.Base(entry.FilePaths[0])
		contentType := previewContentType(entry.FilePaths[0])

		// 获取文件大小
		var fileSize int64
		if !isMulti {
			fileSize = primaryInfo.Size()
		}

		// 构建文件列表信息
		var fileList []gin.H
		for _, fp := range entry.FilePaths {
			_, info, err := resolveWorkspaceEntryNoSymlinks(baseDir, fp)
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
			"authorized":       true,
			"previewable":      !isMulti && isPreviewable(contentType),
			"expires_at":       sharestore.ExpiresAtUnix(entry.ExpiresAt),
		})
	}
}

// ListPreviewLinksHandler 列出当前 HTTP runtime 的所有预览链接。
func (rt *Runtime) ListPreviewLinksHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		store := rt.PreviewLinks

		now := time.Now()
		links := make([]PreviewLink, 0)
		expired := make([]string, 0)

		for id, entry := range store.Snapshot() {
			if !entry.ExpiresAt.IsZero() && now.After(entry.ExpiresAt) {
				expired = append(expired, id)
				continue
			}
			filePath := entry.FilePaths[0]
			if len(entry.FilePaths) > 1 {
				filePath = fmt.Sprintf("%d 个文件", len(entry.FilePaths))
			}
			links = append(links, PreviewLink{
				ID:          id,
				FilePath:    filePath,
				FilePaths:   entry.FilePaths,
				HasPassword: entry.PasswordHash != "",
				ExpiresAt:   sharestore.ExpiresAtUnix(entry.ExpiresAt),
				CreatedAt:   entry.CreatedAt.Unix(),
			})
		}

		// 异步清理过期链接
		if len(expired) > 0 {
			rt.Go(func() {
				if err := store.DeleteMany(expired); err != nil {
					log.Printf("failed to persist expired preview link cleanup: err=%v", err)
				}
			})
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

const previewGrantTTL = time.Hour

func previewGrantCookieName(linkID string) string {
	return "fk_preview_" + linkID
}

func generatePreviewGrant(linkID, passwordHash string, expiresAt time.Time) string {
	payload := linkID + "|" + strconv.FormatInt(expiresAt.Unix(), 10)
	key := sha256.Sum256([]byte(passwordHash))
	mac := hmac.New(sha256.New, key[:])
	_, _ = mac.Write([]byte(payload))
	return strconv.FormatInt(expiresAt.Unix(), 10) + "." + hex.EncodeToString(mac.Sum(nil))
}

func validatePreviewGrant(grant, linkID, passwordHash string, now time.Time) bool {
	parts := strings.SplitN(grant, ".", 2)
	if len(parts) != 2 {
		return false
	}
	expiresAt, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || !now.Before(time.Unix(expiresAt, 0)) {
		return false
	}
	expected := generatePreviewGrant(linkID, passwordHash, time.Unix(expiresAt, 0))
	return hmac.Equal([]byte(grant), []byte(expected))
}

func hasValidPreviewGrant(c *gin.Context, linkID, passwordHash string) bool {
	grant, err := c.Cookie(previewGrantCookieName(linkID))
	return err == nil && validatePreviewGrant(grant, linkID, passwordHash, time.Now())
}

func setPreviewGrantCookie(c *gin.Context, linkID, passwordHash string) {
	expiresAt := time.Now().Add(previewGrantTTL)
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     previewGrantCookieName(linkID),
		Value:    generatePreviewGrant(linkID, passwordHash, expiresAt),
		Path:     "/api/fkteams/preview/" + linkID,
		MaxAge:   int(previewGrantTTL / time.Second),
		HttpOnly: true,
		Secure:   requestUsesHTTPS(c.Request),
		SameSite: http.SameSiteStrictMode,
	})
}

func authorizePreviewPassword(c *gin.Context, linkID, passwordHash, password string) bool {
	if passwordHash == "" || hasValidPreviewGrant(c, linkID, passwordHash) {
		return true
	}
	if password == "" {
		c.JSON(http.StatusUnauthorized, Response{
			Code:    1,
			Message: "preview password is required",
			Data:    gin.H{"require_password": true},
		})
		return false
	}
	attemptKey := "preview:" + linkID + ":" + c.ClientIP()
	if allowed, retryAfter := publicShareAttempts.Allow(attemptKey, time.Now()); !allowed {
		rateLimitExceeded(c, retryAfter)
		return false
	}
	if !verifyPassword(password, passwordHash) {
		c.JSON(http.StatusUnauthorized, Response{
			Code:    1,
			Message: "invalid preview password",
			Data:    gin.H{"require_password": true},
		})
		return false
	}
	publicShareAttempts.Reset(attemptKey)
	setPreviewGrantCookie(c, linkID, passwordHash)
	return true
}

// sharestore.ExpiresAtUnix 将过期时间转为 Unix 时间戳，零值（永不过期）返回 0
func validatedPreviewResources(baseDir string, entry *sharestore.PreviewEntry) ([]string, error) {
	resourcePaths := append([]string(nil), entry.ResourcePaths...)
	if entry.ResourcePaths == nil {
		_, expanded, err := collectPreviewPaths(baseDir, entry.FilePaths)
		if err != nil {
			return nil, err
		}
		resourcePaths = expanded
	}
	for _, resourcePath := range resourcePaths {
		if !entry.ResourceWithinSharedRoots(filepath.Clean(resourcePath)) {
			return nil, fmt.Errorf("shared resource is outside the selected paths")
		}
		_, info, err := resolveWorkspaceEntryNoSymlinks(baseDir, resourcePath)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("shared resource is not a regular file")
		}
	}
	return resourcePaths, nil
}

func writePreviewFileToZip(zw *zip.Writer, baseDir, relativePath string) error {
	file, info, err := openWorkspaceRegularFileNoSymlinks(baseDir, relativePath)
	if err != nil {
		return err
	}
	defer file.Close()

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = filepath.ToSlash(relativePath)
	header.Method = zip.Deflate
	writer, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, file)
	return err
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

// PreviewRenderHandler 渲染当前 HTTP runtime 的预览文件。
func (rt *Runtime) PreviewRenderHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		baseDir := appdata.WorkspaceDir()

		linkID := c.Param("linkId")
		if linkID == "" {
			Fail(c, http.StatusBadRequest, "缺少链接 ID")
			return
		}

		store := rt.PreviewLinks
		entry, exists := store.Lookup(linkID)

		if !exists {
			Fail(c, http.StatusNotFound, "链接不存在或已失效")
			return
		}

		if !entry.ExpiresAt.IsZero() && time.Now().After(entry.ExpiresAt) {
			if _, err := store.Delete(linkID); err != nil {
				log.Printf("failed to remove expired preview link: id=%s, err=%v", linkID, err)
			}
			Fail(c, http.StatusGone, "链接已过期")
			return
		}

		if !authorizePreviewPassword(c, linkID, entry.PasswordHash, c.GetHeader("X-Preview-Password")) {
			return
		}

		mainFile := entry.FilePaths[0]
		mainDir := filepath.Dir(mainFile)

		// 获取请求的相对路径
		relativePath := strings.TrimPrefix(c.Param("filepath"), "/")

		// 如果无路径或路径为 "/"，重定向到主文件
		if relativePath == "" {
			baseName := url.PathEscape(filepath.Base(mainFile))
			redirectURL := fmt.Sprintf("/api/fkteams/preview/%s/render/%s", linkID, baseName)
			c.Redirect(http.StatusFound, redirectURL)
			return
		}

		// 组合完整相对路径：主文件目录 + 请求路径
		targetRel := filepath.Join(mainDir, relativePath)
		cleanTarget := filepath.Clean(targetRel)

		// 安全校验：必须在主文件目录内
		cleanMainDir := filepath.Clean(mainDir)
		if cleanMainDir == "." {
			// 主文件在 workspace 根目录：仅允许访问主文件本身
			if cleanTarget != filepath.Clean(mainFile) {
				Fail(c, http.StatusForbidden, "禁止访问该路径")
				return
			}
		} else if !strings.HasPrefix(cleanTarget, cleanMainDir+string(filepath.Separator)) && cleanTarget != cleanMainDir {
			Fail(c, http.StatusForbidden, "禁止访问该路径")
			return
		}

		// 仅允许访问创建分享链接时记录的资源，不隐式授权同目录文件。
		if !entry.AllowsResource(cleanTarget) {
			Fail(c, http.StatusForbidden, "禁止访问该路径")
			return
		}

		file, info, err := openWorkspaceRegularFileNoSymlinks(baseDir, cleanTarget)
		if err != nil {
			Fail(c, http.StatusNotFound, "文件不存在")
			return
		}
		defer file.Close()

		fullPath := filepath.Join(baseDir, cleanTarget)
		c.Header("Content-Type", previewContentType(cleanTarget))
		c.Header("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, strings.NewReplacer(`"`, `\"`, "\r", "", "\n", "").Replace(info.Name())))
		serveOpenedFileContent(c, file, fullPath, info)
	}
}

func previewContentType(filePath string) string {
	contentType := mime.TypeByExtension(filepath.Ext(filePath))
	if contentType != "" {
		return contentType
	}
	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".md", ".markdown":
		return "text/markdown; charset=utf-8"
	case ".txt", ".log", ".env", ".gitignore":
		return "text/plain; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".yaml", ".yml", ".toml":
		return "text/plain; charset=utf-8"
	case ".js", ".ts", ".tsx", ".jsx", ".css", ".html", ".xml", ".svg", ".go", ".py", ".sh", ".rs", ".java", ".c", ".cpp", ".h":
		return "text/plain; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}
