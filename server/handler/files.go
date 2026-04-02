package handler

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fkteams/common"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// FileInfo 文件信息响应
type FileInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"mod_time"`
}

// getWorkspaceDir 获取工作目录并返回绝对路径
func getWorkspaceDir() (string, string, error) {
	baseDir := common.WorkspaceDir()
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", "", fmt.Errorf("解析工作目录失败")
	}
	return baseDir, absBase, nil
}

// resolveAndValidatePath 解析相对路径并校验是否在 baseDir 内
// 返回完整路径和清理后的相对路径
func resolveAndValidatePath(baseDir, absBase, subPath string) (string, string, error) {
	if subPath == "" {
		return baseDir, "", nil
	}
	cleanPath := filepath.Clean(subPath)
	if strings.Contains(cleanPath, "..") {
		return "", "", fmt.Errorf("无效的路径")
	}
	fullPath := filepath.Join(baseDir, cleanPath)
	absFull, _ := filepath.Abs(fullPath)
	// 解析符号链接后再校验，防止 symlink 逃逸
	realBase, _ := filepath.EvalSymlinks(absBase)
	realFull, err := filepath.EvalSymlinks(absFull)
	if err != nil {
		// 文件可能不存在，回退到 Abs 校验
		if !strings.HasPrefix(absFull, absBase+string(os.PathSeparator)) {
			return "", "", fmt.Errorf("无效的路径")
		}
		return fullPath, cleanPath, nil
	}
	if !strings.HasPrefix(realFull, realBase+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("无效的路径")
	}
	return fullPath, cleanPath, nil
}

// GetFilesHandler 获取指定目录下的文件和文件夹列表
func GetFilesHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		baseDir, absBase, err := getWorkspaceDir()
		if err != nil {
			Fail(c, http.StatusInternalServerError, err.Error())
			return
		}

		subPath := c.Query("path")
		fullPath, _, err := resolveAndValidatePath(baseDir, absBase, subPath)
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}

		info, err := os.Stat(fullPath)
		if err != nil {
			Fail(c, http.StatusNotFound, "目录不存在或无法访问")
			return
		}
		if !info.IsDir() {
			Fail(c, http.StatusBadRequest, "路径不是目录")
			return
		}

		entries, err := os.ReadDir(fullPath)
		if err != nil {
			log.Printf("failed to read dir: path=%s, err=%v", fullPath, err)
			Fail(c, http.StatusInternalServerError, "读取目录失败")
			return
		}

		fileList := make([]FileInfo, 0, len(entries))
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			relativePath := entry.Name()
			if subPath != "" {
				relativePath = filepath.Join(subPath, entry.Name())
			}
			fileInfo, err := entry.Info()
			if err != nil {
				continue
			}
			fileList = append(fileList, FileInfo{
				Name:    entry.Name(),
				Path:    relativePath,
				IsDir:   entry.IsDir(),
				Size:    fileInfo.Size(),
				ModTime: fileInfo.ModTime().Unix(),
			})
		}

		// 排序：文件夹在前，同类型按修改时间倒序，时间相同按名称排序
		sort.Slice(fileList, func(i, j int) bool {
			if fileList[i].IsDir != fileList[j].IsDir {
				return fileList[i].IsDir
			}
			if fileList[i].ModTime != fileList[j].ModTime {
				return fileList[i].ModTime > fileList[j].ModTime
			}
			return fileList[i].Name < fileList[j].Name
		})

		OK(c, fileList)
	}
}

// SearchFilesHandler 递归搜索文件名
func SearchFilesHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		_, absBase, err := getWorkspaceDir()
		if err != nil {
			Fail(c, http.StatusInternalServerError, err.Error())
			return
		}

		query := strings.TrimSpace(c.Query("q"))
		if query == "" {
			Fail(c, http.StatusBadRequest, "搜索关键词不能为空")
			return
		}

		queryLower := strings.ToLower(query)
		const maxResults = 100

		const maxDepth = 10

		var results []FileInfo
		_ = filepath.WalkDir(absBase, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			// skip hidden files/dirs
			if strings.HasPrefix(d.Name(), ".") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if len(results) >= maxResults {
				return filepath.SkipAll
			}

			rel, err := filepath.Rel(absBase, path)
			if err != nil || rel == "." {
				return nil
			}
			// 限制搜索深度
			if d.IsDir() && strings.Count(rel, string(os.PathSeparator)) >= maxDepth {
				return filepath.SkipDir
			}

			if strings.Contains(strings.ToLower(d.Name()), queryLower) {
				info, err := d.Info()
				if err != nil {
					return nil
				}
				results = append(results, FileInfo{
					Name:    d.Name(),
					Path:    rel,
					IsDir:   d.IsDir(),
					Size:    info.Size(),
					ModTime: info.ModTime().Unix(),
				})
			}
			return nil
		})

		if results == nil {
			results = []FileInfo{}
		}

		// 排序：文件夹在前，同类型按名称排序
		sort.Slice(results, func(i, j int) bool {
			if results[i].IsDir != results[j].IsDir {
				return results[i].IsDir
			}
			return results[i].Name < results[j].Name
		})

		OK(c, results)
	}
}

// UploadFileHandler 处理文件上传（支持多文件），将文件保存到工作目录
func UploadFileHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		baseDir, absBase, err := getWorkspaceDir()
		if err != nil {
			Fail(c, http.StatusInternalServerError, err.Error())
			return
		}

		subPath := c.PostForm("path")
		targetDir, _, err := resolveAndValidatePath(baseDir, absBase, subPath)
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}

		// 确保目标目录存在
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			Fail(c, http.StatusInternalServerError, "创建目录失败")
			return
		}

		// 获取所有上传的文件
		form, err := c.MultipartForm()
		if err != nil {
			Fail(c, http.StatusBadRequest, "解析表单失败")
			return
		}

		files := form.File["file"]
		if len(files) == 0 {
			Fail(c, http.StatusBadRequest, "未找到上传文件")
			return
		}

		results := make([]FileInfo, 0, len(files))

		for _, file := range files {
			// 校验文件名安全性
			fileName := filepath.Base(file.Filename)
			if fileName == "." || fileName == ".." || fileName == "" {
				continue
			}

			savePath := filepath.Join(targetDir, fileName)

			// 确保最终路径在 baseDir 内
			absSave, _ := filepath.Abs(savePath)
			if !strings.HasPrefix(absSave, absBase+string(os.PathSeparator)) {
				continue
			}

			if err := c.SaveUploadedFile(file, savePath); err != nil {
				continue
			}

			info, err := os.Stat(savePath)
			if err != nil {
				continue
			}
			relativePath := fileName
			if subPath != "" {
				relativePath = filepath.Join(subPath, fileName)
			}
			results = append(results, FileInfo{
				Name:    fileName,
				Path:    relativePath,
				IsDir:   false,
				Size:    info.Size(),
				ModTime: info.ModTime().Unix(),
			})
		}

		if len(results) == 0 {
			Fail(c, http.StatusBadRequest, "没有文件上传成功")
			return
		}

		OK(c, results)
	}
}

// chunkUploadMeta 分片上传状态
type chunkUploadMeta struct {
	mu          sync.Mutex
	TotalChunks int
	Received    map[int]bool
	FilePath    string
}

// chunkUploads 全局分片上传状态管理
var chunkUploads = struct {
	sync.Mutex
	m map[string]*chunkUploadMeta
}{m: make(map[string]*chunkUploadMeta)}

// UploadChunkHandler 处理分片上传
// 参数: file(分片内容), uploadId(上传标识), chunkIndex(分片序号,0-based), totalChunks(总分片数), fileName(文件名), path(可选子目录)
func UploadChunkHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		baseDir, absBase, err := getWorkspaceDir()
		if err != nil {
			Fail(c, http.StatusInternalServerError, err.Error())
			return
		}

		uploadID := c.PostForm("uploadId")
		chunkIndexStr := c.PostForm("chunkIndex")
		totalChunksStr := c.PostForm("totalChunks")
		fileName := c.PostForm("fileName")

		if uploadID == "" || chunkIndexStr == "" || totalChunksStr == "" || fileName == "" {
			Fail(c, http.StatusBadRequest, "缺少必要参数")
			return
		}

		chunkIndex, err := strconv.Atoi(chunkIndexStr)
		if err != nil || chunkIndex < 0 {
			Fail(c, http.StatusBadRequest, "无效的分片序号")
			return
		}

		totalChunks, err := strconv.Atoi(totalChunksStr)
		if err != nil || totalChunks <= 0 {
			Fail(c, http.StatusBadRequest, "无效的总分片数")
			return
		}

		if chunkIndex >= totalChunks {
			Fail(c, http.StatusBadRequest, "分片序号超出范围")
			return
		}

		// 校验文件名安全性
		safeName := filepath.Base(fileName)
		if safeName == "." || safeName == ".." || safeName == "" {
			Fail(c, http.StatusBadRequest, "无效的文件名")
			return
		}

		// 验证目标子目录
		subPath := c.PostForm("path")
		targetDir, _, err := resolveAndValidatePath(baseDir, absBase, subPath)
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}

		// 确保最终路径在 baseDir 内
		finalPath := filepath.Join(targetDir, safeName)
		absFinal, _ := filepath.Abs(finalPath)
		if !strings.HasPrefix(absFinal, absBase+string(os.PathSeparator)) {
			Fail(c, http.StatusBadRequest, "无效的保存路径")
			return
		}

		// 分片临时目录
		chunkDir := filepath.Join(os.TempDir(), "fkteams-chunks", sanitizeUploadID(uploadID))
		if err := os.MkdirAll(chunkDir, 0755); err != nil {
			log.Printf("failed to create chunk dir: path=%s, err=%v", chunkDir, err)
			Fail(c, http.StatusInternalServerError, "创建临时目录失败")
			return
		}

		// 保存分片
		file, err := c.FormFile("file")
		if err != nil {
			Fail(c, http.StatusBadRequest, "未找到分片文件")
			return
		}

		chunkPath := filepath.Join(chunkDir, fmt.Sprintf("%d", chunkIndex))
		if err := c.SaveUploadedFile(file, chunkPath); err != nil {
			log.Printf("failed to save chunk: path=%s, err=%v", chunkPath, err)
			Fail(c, http.StatusInternalServerError, "保存分片失败")
			return
		}

		// 更新上传状态
		chunkUploads.Lock()
		meta, exists := chunkUploads.m[uploadID]
		if !exists {
			meta = &chunkUploadMeta{
				TotalChunks: totalChunks,
				Received:    make(map[int]bool),
				FilePath:    finalPath,
			}
			chunkUploads.m[uploadID] = meta
		}
		chunkUploads.Unlock()

		meta.mu.Lock()
		meta.Received[chunkIndex] = true
		allReceived := len(meta.Received) == meta.TotalChunks
		meta.mu.Unlock()

		if !allReceived {
			OK(c, gin.H{
				"uploadId":   uploadID,
				"chunkIndex": chunkIndex,
				"received":   len(meta.Received),
				"total":      totalChunks,
				"completed":  false,
			})
			return
		}

		// 所有分片已接收，合并文件
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			Fail(c, http.StatusInternalServerError, "创建目录失败")
			return
		}

		outFile, err := os.Create(finalPath)
		if err != nil {
			Fail(c, http.StatusInternalServerError, "创建目标文件失败")
			return
		}
		defer outFile.Close()

		for i := 0; i < totalChunks; i++ {
			chunkFile := filepath.Join(chunkDir, fmt.Sprintf("%d", i))
			src, err := os.Open(chunkFile)
			if err != nil {
				Fail(c, http.StatusInternalServerError, fmt.Sprintf("读取分片 %d 失败", i))
				return
			}
			_, writeErr := io.Copy(outFile, src)
			src.Close()
			if writeErr != nil {
				Fail(c, http.StatusInternalServerError, fmt.Sprintf("写入分片 %d 失败", i))
				return
			}
		}

		// 清理临时分片
		os.RemoveAll(chunkDir)
		chunkUploads.Lock()
		delete(chunkUploads.m, uploadID)
		chunkUploads.Unlock()

		info, _ := os.Stat(finalPath)
		relativePath := safeName
		if subPath != "" {
			relativePath = filepath.Join(subPath, safeName)
		}
		OK(c, gin.H{
			"uploadId":  uploadID,
			"completed": true,
			"file": FileInfo{
				Name:    safeName,
				Path:    relativePath,
				IsDir:   false,
				Size:    info.Size(),
				ModTime: info.ModTime().Unix(),
			},
		})
	}
}

// sanitizeUploadID 清理 uploadId 防止路径遍历
func sanitizeUploadID(id string) string {
	h := sha256.Sum256([]byte(id))
	return hex.EncodeToString(h[:16])
}

// DownloadFileHandler 下载工作目录中的文件（目录自动打包为 zip）
// Query: path(文件相对路径)
func DownloadFileHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		baseDir, absBase, err := getWorkspaceDir()
		if err != nil {
			Fail(c, http.StatusInternalServerError, err.Error())
			return
		}

		filePath := c.Query("path")
		if filePath == "" {
			Fail(c, http.StatusBadRequest, "缺少 path 参数")
			return
		}

		fullPath, _, err := resolveAndValidatePath(baseDir, absBase, filePath)
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}

		info, err := os.Stat(fullPath)
		if err != nil {
			Fail(c, http.StatusNotFound, "文件不存在")
			return
		}

		if !info.IsDir() {
			fileName := filepath.Base(fullPath)
			c.FileAttachment(fullPath, fileName)
			return
		}

		// 目录：打包为 zip 流式下载
		dirName := filepath.Base(fullPath)
		c.Header("Content-Type", "application/zip")
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, dirName))
		c.Status(http.StatusOK)

		zw := zip.NewWriter(c.Writer)
		defer zw.Close()

		_ = filepath.WalkDir(fullPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if strings.HasPrefix(d.Name(), ".") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			rel, err := filepath.Rel(fullPath, path)
			if err != nil || rel == "." {
				return nil
			}

			if d.IsDir() {
				_, _ = zw.Create(rel + "/")
				return nil
			}

			fi, err := d.Info()
			if err != nil {
				return nil
			}
			header, err := zip.FileInfoHeader(fi)
			if err != nil {
				return nil
			}
			header.Name = rel
			header.Method = zip.Deflate

			w, err := zw.CreateHeader(header)
			if err != nil {
				return nil
			}
			f, err := os.Open(path)
			if err != nil {
				return nil
			}
			defer f.Close()
			_, _ = io.Copy(w, f)
			return nil
		})
	}
}

// BatchDownloadHandler 批量下载：将多个文件/文件夹打包为单个 zip
// JSON body: {"paths": ["path1", "path2", ...]}
func BatchDownloadHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		baseDir, absBase, err := getWorkspaceDir()
		if err != nil {
			Fail(c, http.StatusInternalServerError, err.Error())
			return
		}

		var req struct {
			Paths []string `json:"paths" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || len(req.Paths) == 0 {
			Fail(c, http.StatusBadRequest, "缺少 paths 参数")
			return
		}

		// 校验所有路径
		type validatedPath struct {
			fullPath string
			relPath  string
		}
		var validated []validatedPath
		for _, p := range req.Paths {
			fullPath, cleanPath, err := resolveAndValidatePath(baseDir, absBase, p)
			if err != nil {
				Fail(c, http.StatusBadRequest, err.Error())
				return
			}
			if _, err := os.Stat(fullPath); err != nil {
				Fail(c, http.StatusNotFound, fmt.Sprintf("文件不存在: %s", cleanPath))
				return
			}
			validated = append(validated, validatedPath{fullPath: fullPath, relPath: cleanPath})
		}

		c.Header("Content-Type", "application/zip")
		c.Header("Content-Disposition", `attachment; filename="download.zip"`)
		c.Status(http.StatusOK)

		zw := zip.NewWriter(c.Writer)
		defer zw.Close()

		for _, vp := range validated {
			info, err := os.Stat(vp.fullPath)
			if err != nil {
				continue
			}

			if !info.IsDir() {
				// 单个文件
				header, err := zip.FileInfoHeader(info)
				if err != nil {
					continue
				}
				header.Name = vp.relPath
				header.Method = zip.Deflate
				w, err := zw.CreateHeader(header)
				if err != nil {
					continue
				}
				f, err := os.Open(vp.fullPath)
				if err != nil {
					continue
				}
				_, _ = io.Copy(w, f)
				f.Close()
				continue
			}

			// 目录递归写入
			_ = filepath.WalkDir(vp.fullPath, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				if strings.HasPrefix(d.Name(), ".") {
					if d.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}

				rel, err := filepath.Rel(filepath.Dir(vp.fullPath), path)
				if err != nil {
					return nil
				}

				if d.IsDir() {
					_, _ = zw.Create(rel + "/")
					return nil
				}

				fi, err := d.Info()
				if err != nil {
					return nil
				}
				header, err := zip.FileInfoHeader(fi)
				if err != nil {
					return nil
				}
				header.Name = rel
				header.Method = zip.Deflate

				w, err := zw.CreateHeader(header)
				if err != nil {
					return nil
				}
				f, err := os.Open(path)
				if err != nil {
					return nil
				}
				defer f.Close()
				_, _ = io.Copy(w, f)
				return nil
			})
		}
	}
}

// DeleteFileHandler 删除工作目录中的文件或目录
// JSON body: {"path": "相对路径", "force": false}
// force 为 true 时可删除非空目录
func DeleteFileHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		baseDir, absBase, err := getWorkspaceDir()
		if err != nil {
			Fail(c, http.StatusInternalServerError, err.Error())
			return
		}

		var req struct {
			Path  string `json:"path" binding:"required"`
			Force bool   `json:"force"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, "缺少 path 参数")
			return
		}

		fullPath, _, err := resolveAndValidatePath(baseDir, absBase, req.Path)
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}

		// 不允许删除根工作目录
		absFull, _ := filepath.Abs(fullPath)
		if absFull == absBase {
			Fail(c, http.StatusBadRequest, "无效的文件路径")
			return
		}

		info, err := os.Stat(fullPath)
		if err != nil {
			Fail(c, http.StatusNotFound, "文件或目录不存在")
			return
		}

		if info.IsDir() {
			entries, err := os.ReadDir(fullPath)
			if err != nil {
				log.Printf("failed to read dir for delete: path=%s, err=%v", fullPath, err)
				Fail(c, http.StatusInternalServerError, "读取目录失败")
				return
			}
			if len(entries) > 0 && !req.Force {
				Fail(c, http.StatusBadRequest, "目录非空，请设置 force:true 确认删除")
				return
			}
			if err := os.RemoveAll(fullPath); err != nil {
				log.Printf("failed to delete dir: path=%s, err=%v", fullPath, err)
				Fail(c, http.StatusInternalServerError, fmt.Sprintf("删除目录失败: %v", err))
				return
			}
		} else {
			if err := os.Remove(fullPath); err != nil {
				log.Printf("failed to delete file: path=%s, err=%v", fullPath, err)
				Fail(c, http.StatusInternalServerError, fmt.Sprintf("删除文件失败: %v", err))
				return
			}
		}

		OK(c, nil)
	}
}
