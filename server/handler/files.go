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
		// 获取环境变量配置的目录
		baseDir := os.Getenv("FEIKONG_WORKSPACE_DIR")
		if baseDir == "" {
			c.JSON(http.StatusOK, gin.H{
				"code":    -1,
				"message": "FEIKONG_WORKSPACE_DIR 未配置",
				"data":    []FileInfo{},
			})
			return
		}

		// 获取查询参数中的子路径（可选）
		subPath := c.Query("path")

		// 构建完整路径
		fullPath := baseDir
		if subPath != "" {
			// 安全检查：防止路径遍历攻击
			cleanPath := filepath.Clean(subPath)
			if strings.Contains(cleanPath, "..") {
				c.JSON(http.StatusBadRequest, gin.H{
					"code":    -1,
					"message": "无效的路径",
					"data":    []FileInfo{},
				})
				return
			}
			fullPath = filepath.Join(baseDir, cleanPath)
		}

		// 验证路径是否存在
		info, err := os.Stat(fullPath)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"code":    -1,
				"message": "目录不存在或无法访问",
				"data":    []FileInfo{},
			})
			return
		}

		if !info.IsDir() {
			c.JSON(http.StatusOK, gin.H{
				"code":    -1,
				"message": "路径不是目录",
				"data":    []FileInfo{},
			})
			return
		}

		// 读取目录内容
		entries, err := os.ReadDir(fullPath)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"code":    -1,
				"message": "读取目录失败",
				"data":    []FileInfo{},
			})
			return
		}

		// 构建文件列表
		fileList := make([]FileInfo, 0, len(entries))
		for _, entry := range entries {
			// 跳过隐藏文件
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}

			relativePath := entry.Name()
			if subPath != "" {
				relativePath = filepath.Join(subPath, entry.Name())
			}

			// 获取文件详细信息
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

		// 排序：文件夹在前，同类型按修改时间倒序（最近的在前），时间相同按名称排序
		sort.Slice(fileList, func(i, j int) bool {
			// 如果一个是文件夹，一个是文件，文件夹在前
			if fileList[i].IsDir != fileList[j].IsDir {
				return fileList[i].IsDir
			}
			// 同类型先按修改时间倒序排序（最近的在前）
			if fileList[i].ModTime != fileList[j].ModTime {
				return fileList[i].ModTime > fileList[j].ModTime
			}
			// 修改时间相同，按名称排序
			return fileList[i].Name < fileList[j].Name
		})

		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "success",
			"data":    fileList,
		})
	}
}
