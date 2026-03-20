package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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

// GetFilesHandler 获取指定目录下的文件和文件夹列表
func GetFilesHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		baseDir := os.Getenv("FEIKONG_WORKSPACE_DIR")
		if baseDir == "" {
			Fail(c, http.StatusInternalServerError, "FEIKONG_WORKSPACE_DIR 未配置")
			return
		}

		subPath := c.Query("path")
		fullPath := baseDir
		if subPath != "" {
			cleanPath := filepath.Clean(subPath)
			if strings.Contains(cleanPath, "..") {
				Fail(c, http.StatusBadRequest, "无效的路径")
				return
			}
			fullPath = filepath.Join(baseDir, cleanPath)
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

// UploadFileHandler 处理文件上传（支持多文件），将文件保存到工作目录
func UploadFileHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		baseDir := os.Getenv("FEIKONG_WORKSPACE_DIR")
		if baseDir == "" {
			Fail(c, http.StatusInternalServerError, "FEIKONG_WORKSPACE_DIR 未配置")
			return
		}

		absBase, _ := filepath.Abs(baseDir)

		// 验证目标子目录
		subPath := c.PostForm("path")
		targetDir := baseDir
		if subPath != "" {
			cleanPath := filepath.Clean(subPath)
			if strings.Contains(cleanPath, "..") {
				Fail(c, http.StatusBadRequest, "无效的路径")
				return
			}
			targetDir = filepath.Join(baseDir, cleanPath)
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
		baseDir := os.Getenv("FEIKONG_WORKSPACE_DIR")
		if baseDir == "" {
			Fail(c, http.StatusInternalServerError, "FEIKONG_WORKSPACE_DIR 未配置")
			return
		}

		absBase, _ := filepath.Abs(baseDir)

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
		targetDir := baseDir
		if subPath != "" {
			cleanPath := filepath.Clean(subPath)
			if strings.Contains(cleanPath, "..") {
				Fail(c, http.StatusBadRequest, "无效的路径")
				return
			}
			targetDir = filepath.Join(baseDir, cleanPath)
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
			data, err := os.ReadFile(chunkFile)
			if err != nil {
				Fail(c, http.StatusInternalServerError, fmt.Sprintf("读取分片 %d 失败", i))
				return
			}
			if _, err := outFile.Write(data); err != nil {
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
