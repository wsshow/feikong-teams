package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
			Fail(c, http.StatusOK, "FEIKONG_WORKSPACE_DIR 未配置")
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
			Fail(c, http.StatusOK, "目录不存在或无法访问")
			return
		}
		if !info.IsDir() {
			Fail(c, http.StatusOK, "路径不是目录")
			return
		}

		entries, err := os.ReadDir(fullPath)
		if err != nil {
			Fail(c, http.StatusOK, "读取目录失败")
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
