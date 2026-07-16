package handler

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"fkteams/internal/app/appdata"
	"fkteams/internal/runtime/atomicfile"
	"fkteams/internal/runtime/log"
	"fkteams/internal/runtime/pathguard"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// FileInfo 文件信息响应
type FileInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"mod_time"`
}

const (
	editableFileMaxBytes              = 2 * 1024 * 1024
	maxUploadFiles                    = 32
	maxUploadedFileBytes              = 100 << 20
	maxUploadIDBytes                  = 128
	maxUploadChunks                   = 1024
	maxUploadChunkBytes               = 64 << 20
	maxChunkedFileBytes               = 4 << 30
	chunkUploadTTL                    = time.Hour
	maxActiveChunkUploads             = 32
	maxConcurrentChunkRequests        = 8
	maxChunkUploadTempBytes     int64 = maxChunkedFileBytes
	maxArchiveEntries                 = 10_000
	maxArchiveUncompressedBytes int64 = 2 << 30
	maxBatchDownloadPaths             = 128
	maxConcurrentArchives             = 4
	maxDirectoryEntries               = 10_000
	maxSearchEntries                  = 50_000
)

var errChunkUploadCapacity = errors.New("chunk upload capacity exceeded")
var errArchiveLimit = errors.New("archive resource limit exceeded")
var archiveSlots = make(chan struct{}, maxConcurrentArchives)
var errSearchLimit = errors.New("workspace search limit exceeded")

const untrustedContentSecurityPolicy = "sandbox; default-src 'none'; img-src 'self' data: blob:; media-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; font-src 'self' data:; script-src 'none'; connect-src 'none'; object-src 'none'; frame-src 'none'; worker-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'self'"

// getWorkspaceDir 获取工作目录并返回绝对路径
func getWorkspaceDir() (string, error) {
	baseDir := appdata.WorkspaceDir()
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return "", fmt.Errorf("创建工作目录失败")
	}
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("解析工作目录失败")
	}
	return absBase, nil
}

// resolveWorkspaceEntryNoSymlinks 解析已存在的工作区条目，并拒绝路径中任意符号链接。
// 分享和浏览器预览会在后续请求中重复调用，避免创建链接后的路径替换扩大访问范围。
func resolveWorkspaceEntryNoSymlinks(baseDir, subPath string) (pathguard.ResolvedPath, os.FileInfo, error) {
	resolved, err := pathguard.ResolveWorkspace(baseDir, subPath)
	if err != nil {
		return pathguard.ResolvedPath{}, nil, err
	}
	if err := rejectSymlinkComponents(resolved.BaseAbs, resolved.RelPath); err != nil {
		return pathguard.ResolvedPath{}, nil, err
	}
	info, err := os.Lstat(resolved.AbsPath)
	if err != nil {
		return pathguard.ResolvedPath{}, nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return pathguard.ResolvedPath{}, nil, fmt.Errorf("symbolic links are not allowed")
	}
	return resolved, info, nil
}

func rejectSymlinkComponents(baseDir, relativePath string) error {
	current := filepath.Clean(baseDir)
	if relativePath == "" {
		return nil
	}
	for _, component := range strings.Split(filepath.Clean(relativePath), string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic links are not allowed")
		}
	}
	return nil
}

func openWorkspaceRegularFileNoSymlinks(baseDir, relativePath string) (*os.File, os.FileInfo, error) {
	resolved, info, err := resolveWorkspaceEntryNoSymlinks(baseDir, relativePath)
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("path is not a regular file")
	}

	root, err := os.OpenRoot(resolved.BaseAbs)
	if err != nil {
		return nil, nil, err
	}
	defer root.Close()

	file, err := root.Open(workspaceFSPath(resolved.RelPath))
	if err != nil {
		return nil, nil, err
	}
	openedInfo, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	if !openedInfo.Mode().IsRegular() {
		file.Close()
		return nil, nil, fmt.Errorf("path is not a regular file")
	}

	// 根目录句柄保证打开过程不会逃逸工作区；打开后再校验组件与文件身份。
	if err := rejectRootSymlinkComponents(root, resolved.RelPath); err != nil {
		file.Close()
		return nil, nil, err
	}
	currentInfo, err := root.Lstat(workspaceFSPath(resolved.RelPath))
	if err != nil || !os.SameFile(openedInfo, currentInfo) {
		file.Close()
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("file changed while opening")
	}
	return file, openedInfo, nil
}

func openWorkspaceDirectoryNoSymlinks(baseDir, relativePath string) (*os.File, os.FileInfo, error) {
	resolved, info, err := resolveWorkspaceEntryNoSymlinks(baseDir, relativePath)
	if err != nil {
		return nil, nil, err
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("path is not a directory")
	}

	root, err := os.OpenRoot(resolved.BaseAbs)
	if err != nil {
		return nil, nil, err
	}
	defer root.Close()
	file, err := root.Open(workspaceFSPath(resolved.RelPath))
	if err != nil {
		return nil, nil, err
	}
	openedInfo, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	if !openedInfo.IsDir() {
		file.Close()
		return nil, nil, fmt.Errorf("path is not a directory")
	}
	if err := rejectRootSymlinkComponents(root, resolved.RelPath); err != nil {
		file.Close()
		return nil, nil, err
	}
	currentInfo, err := root.Lstat(workspaceFSPath(resolved.RelPath))
	if err != nil || !os.SameFile(openedInfo, currentInfo) {
		file.Close()
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("directory changed while opening")
	}
	return file, openedInfo, nil
}

func rejectRootSymlinkComponents(root *os.Root, relativePath string) error {
	current := ""
	for _, component := range strings.Split(filepath.Clean(relativePath), string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err := root.Lstat(workspaceFSPath(current))
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic links are not allowed")
		}
	}
	return nil
}

func ensureWorkspaceDirectoryNoSymlinks(baseDir, relativePath string) error {
	root, err := os.OpenRoot(baseDir)
	if err != nil {
		return err
	}
	defer root.Close()
	path := workspaceFSPath(relativePath)
	if err := root.MkdirAll(path, 0755); err != nil {
		return err
	}
	if err := rejectRootSymlinkComponents(root, relativePath); err != nil {
		return err
	}
	info, err := root.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace path is not a regular directory")
	}
	return nil
}

func safeUploadedFileName(name string) (string, error) {
	normalized := strings.ReplaceAll(name, `\`, "/")
	fileName := filepath.Base(filepath.FromSlash(normalized))
	if fileName == "." || fileName == ".." || fileName == "" || strings.ContainsRune(fileName, 0) {
		return "", fmt.Errorf("invalid uploaded file name")
	}
	return fileName, nil
}

func saveUploadedFileInRoot(root *os.Root, fileHeader *multipart.FileHeader, relativePath string, maxBytes int64) (int64, error) {
	source, err := fileHeader.Open()
	if err != nil {
		return 0, fmt.Errorf("open uploaded file: %w", err)
	}
	written, writeErr := atomicfile.WriteReaderInRoot(root, relativePath, source, maxBytes, 0644)
	closeErr := source.Close()
	if writeErr != nil {
		return written, writeErr
	}
	if closeErr != nil {
		return written, fmt.Errorf("close uploaded file: %w", closeErr)
	}
	return written, nil
}

func saveUploadedFileAtomic(fileHeader *multipart.FileHeader, destination string, maxBytes int64) (int64, error) {
	source, err := fileHeader.Open()
	if err != nil {
		return 0, fmt.Errorf("open uploaded file: %w", err)
	}
	defer source.Close()

	temporary, err := os.CreateTemp(filepath.Dir(destination), ".upload-*")
	if err != nil {
		return 0, fmt.Errorf("create upload temp file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	written, copyErr := io.Copy(temporary, io.LimitReader(source, maxBytes+1))
	if copyErr != nil {
		_ = temporary.Close()
		return 0, fmt.Errorf("copy uploaded file: %w", copyErr)
	}
	if written > maxBytes {
		_ = temporary.Close()
		return 0, fmt.Errorf("uploaded file is too large")
	}
	if err := temporary.Chmod(0644); err != nil {
		_ = temporary.Close()
		return 0, fmt.Errorf("set uploaded file permissions: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return 0, fmt.Errorf("close uploaded file: %w", err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return 0, fmt.Errorf("replace uploaded file: %w", err)
	}
	return written, nil
}

// GetFilesHandler 获取指定目录下的文件和文件夹列表
func GetFilesHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		baseDir, err := getWorkspaceDir()
		if err != nil {
			Fail(c, http.StatusInternalServerError, err.Error())
			return
		}

		subPath := c.Query("path")
		resolved, _, err := resolveWorkspaceEntryNoSymlinks(baseDir, subPath)
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}

		directory, _, err := openWorkspaceDirectoryNoSymlinks(baseDir, resolved.RelPath)
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}
		entries, readErr := directory.ReadDir(maxDirectoryEntries + 1)
		closeErr := directory.Close()
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			log.Printf("failed to read workspace directory: path=%s, err=%v", resolved.RelPath, readErr)
			Fail(c, http.StatusInternalServerError, "读取目录失败")
			return
		}
		if closeErr != nil {
			log.Printf("failed to close workspace directory: path=%s, err=%v", resolved.RelPath, closeErr)
		}
		if len(entries) > maxDirectoryEntries {
			Fail(c, http.StatusRequestEntityTooLarge, "directory contains too many entries")
			return
		}

		fileList := make([]FileInfo, 0, len(entries))
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			relativePath := entry.Name()
			if resolved.RelPath != "" {
				relativePath = filepath.Join(resolved.RelPath, entry.Name())
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

// SearchFilesHandler 递归搜索文件名和相对路径
func SearchFilesHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		baseDir, err := getWorkspaceDir()
		if err != nil {
			Fail(c, http.StatusInternalServerError, err.Error())
			return
		}

		query := strings.TrimSpace(c.Query("q"))
		if query == "" {
			Fail(c, http.StatusBadRequest, "搜索关键词不能为空")
			return
		}

		root, err := os.OpenRoot(baseDir)
		if err != nil {
			Fail(c, http.StatusInternalServerError, "failed to open workspace")
			return
		}
		results, err := searchWorkspaceFiles(c.Request.Context(), root.FS(), query)
		root.Close()
		if errors.Is(err, errSearchLimit) {
			Fail(c, http.StatusRequestEntityTooLarge, err.Error())
			return
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		if err != nil {
			log.Printf("failed to search workspace: err=%v", err)
			Fail(c, http.StatusInternalServerError, "failed to search workspace")
			return
		}

		if results == nil {
			results = []FileInfo{}
		}

		// 排序：文件夹在前，同类型按名称排序
		sort.Slice(results, func(i, j int) bool {
			if results[i].IsDir != results[j].IsDir {
				return results[i].IsDir
			}
			return results[i].Path < results[j].Path
		})

		OK(c, results)
	}
}

func searchWorkspaceFiles(ctx context.Context, filesystem fs.FS, query string) ([]FileInfo, error) {
	const maxResults = 100
	const maxDepth = 10

	queryLower := strings.ToLower(filepath.ToSlash(query))
	results := make([]FileInfo, 0)
	visited := 0
	err := fs.WalkDir(filesystem, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == "." {
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		visited++
		if visited > maxSearchEntries {
			return errSearchLimit
		}
		if len(results) >= maxResults {
			return fs.SkipAll
		}

		relativePath := strings.TrimPrefix(filepath.ToSlash(path), "./")
		if entry.IsDir() && strings.Count(relativePath, "/") >= maxDepth {
			return fs.SkipDir
		}
		if !strings.Contains(strings.ToLower(entry.Name()), queryLower) && !strings.Contains(strings.ToLower(relativePath), queryLower) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		results = append(results, FileInfo{
			Name:    entry.Name(),
			Path:    relativePath,
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().Unix(),
		})
		return nil
	})
	return results, err
}

// GetFileContentHandler 读取工作目录中的文本文件内容。
// Query: path(文件相对路径)
func GetFileContentHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		baseDir, err := getWorkspaceDir()
		if err != nil {
			Fail(c, http.StatusInternalServerError, err.Error())
			return
		}

		filePath := c.Query("path")
		if filePath == "" {
			Fail(c, http.StatusBadRequest, "缺少 path 参数")
			return
		}

		resolved, _, err := resolveWorkspaceEntryNoSymlinks(baseDir, filePath)
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}

		file, info, err := openWorkspaceRegularFileNoSymlinks(baseDir, resolved.RelPath)
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}
		defer file.Close()
		if info.Size() > editableFileMaxBytes {
			Fail(c, http.StatusBadRequest, "文件过大，无法编辑")
			return
		}

		data, err := io.ReadAll(io.LimitReader(file, editableFileMaxBytes+1))
		if err != nil {
			log.Printf("failed to read file content: path=%s, err=%v", resolved.RelPath, err)
			Fail(c, http.StatusInternalServerError, "读取文件失败")
			return
		}
		if len(data) > editableFileMaxBytes {
			Fail(c, http.StatusBadRequest, "文件过大，无法编辑")
			return
		}
		if !utf8.Valid(data) {
			Fail(c, http.StatusBadRequest, "文件不是 UTF-8 文本")
			return
		}

		OK(c, gin.H{
			"path":     resolved.RelPath,
			"name":     filepath.Base(resolved.RelPath),
			"content":  string(data),
			"size":     info.Size(),
			"mod_time": info.ModTime().Unix(),
		})
	}
}

// SaveFileContentHandler 保存工作目录中的文本文件内容。
// JSON body: {"path": "相对路径", "content": "文件内容"}
func SaveFileContentHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		baseDir, err := getWorkspaceDir()
		if err != nil {
			Fail(c, http.StatusInternalServerError, err.Error())
			return
		}

		var req struct {
			Path    string `json:"path" binding:"required"`
			Content string `json:"content"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, "缺少 path 参数")
			return
		}
		if len([]byte(req.Content)) > editableFileMaxBytes {
			Fail(c, http.StatusBadRequest, "内容过大，无法保存")
			return
		}

		resolved, _, err := resolveWorkspaceEntryNoSymlinks(baseDir, req.Path)
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}

		opened, info, err := openWorkspaceRegularFileNoSymlinks(baseDir, resolved.RelPath)
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}
		if err := opened.Close(); err != nil {
			Fail(c, http.StatusInternalServerError, "failed to close file before saving")
			return
		}
		if !utf8.ValidString(req.Content) {
			Fail(c, http.StatusBadRequest, "内容不是 UTF-8 文本")
			return
		}

		root, err := os.OpenRoot(baseDir)
		if err != nil {
			Fail(c, http.StatusInternalServerError, "failed to open workspace")
			return
		}
		defer root.Close()
		if err := rejectRootSymlinkComponents(root, resolved.RelPath); err != nil {
			Fail(c, http.StatusConflict, "file changed before saving")
			return
		}
		currentInfo, err := root.Lstat(resolved.RelPath)
		if err != nil || !os.SameFile(info, currentInfo) || !currentInfo.Mode().IsRegular() {
			Fail(c, http.StatusConflict, "file changed before saving")
			return
		}
		if err := atomicfile.WriteFileInRoot(root, resolved.RelPath, []byte(req.Content), info.Mode().Perm()); err != nil {
			log.Printf("failed to save file content: path=%s, err=%v", resolved.RelPath, err)
			Fail(c, http.StatusInternalServerError, "保存文件失败")
			return
		}

		nextInfo, err := root.Lstat(resolved.RelPath)
		if err != nil {
			Fail(c, http.StatusInternalServerError, "读取文件状态失败")
			return
		}
		OK(c, gin.H{
			"path":     resolved.RelPath,
			"name":     filepath.Base(resolved.RelPath),
			"size":     nextInfo.Size(),
			"mod_time": nextInfo.ModTime().Unix(),
		})
	}
}

// UploadFileHandler 处理文件上传（支持多文件），将文件保存到工作目录
func UploadFileHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if c.Request.MultipartForm != nil {
				_ = c.Request.MultipartForm.RemoveAll()
			}
		}()

		baseDir, err := getWorkspaceDir()
		if err != nil {
			Fail(c, http.StatusInternalServerError, err.Error())
			return
		}

		subPath := c.PostForm("path")
		resolvedTarget, err := pathguard.ResolveWorkspace(baseDir, subPath)
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}

		targetRelPath := resolvedTarget.RelPath
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
		if len(files) > maxUploadFiles {
			Fail(c, http.StatusBadRequest, "too many uploaded files")
			return
		}

		type pendingUpload struct {
			header       *multipart.FileHeader
			fileName     string
			relativePath string
		}
		pending := make([]pendingUpload, 0, len(files))
		seenNames := make(map[string]struct{}, len(files))
		for _, file := range files {
			fileName, err := safeUploadedFileName(file.Filename)
			if err != nil {
				Fail(c, http.StatusBadRequest, "invalid uploaded file name")
				return
			}
			if file.Size < 0 || file.Size > maxUploadedFileBytes {
				Fail(c, http.StatusRequestEntityTooLarge, "uploaded file is too large")
				return
			}
			nameKey := strings.ToLower(fileName)
			if _, exists := seenNames[nameKey]; exists {
				Fail(c, http.StatusBadRequest, "duplicate uploaded file name")
				return
			}
			seenNames[nameKey] = struct{}{}

			relativePath := fileName
			if targetRelPath != "" {
				relativePath = filepath.Join(targetRelPath, fileName)
			}
			pending = append(pending, pendingUpload{header: file, fileName: fileName, relativePath: relativePath})
		}

		if err := ensureWorkspaceDirectoryNoSymlinks(baseDir, targetRelPath); err != nil {
			Fail(c, http.StatusBadRequest, "invalid upload directory")
			return
		}
		root, err := os.OpenRoot(baseDir)
		if err != nil {
			Fail(c, http.StatusInternalServerError, "failed to open workspace")
			return
		}
		defer root.Close()
		if err := rejectRootSymlinkComponents(root, targetRelPath); err != nil {
			Fail(c, http.StatusBadRequest, "invalid upload directory")
			return
		}
		for _, upload := range pending {
			info, err := root.Lstat(upload.relativePath)
			if err == nil && !info.Mode().IsRegular() {
				Fail(c, http.StatusBadRequest, "upload target is not a regular file")
				return
			}
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				Fail(c, http.StatusInternalServerError, "failed to inspect upload target")
				return
			}
		}
		results := make([]FileInfo, 0, len(pending))
		for _, upload := range pending {
			if err := rejectRootSymlinkComponents(root, targetRelPath); err != nil {
				Fail(c, http.StatusBadRequest, "invalid upload directory")
				return
			}
			size, err := saveUploadedFileInRoot(root, upload.header, upload.relativePath, maxUploadedFileBytes)
			if err != nil {
				Fail(c, http.StatusInternalServerError, "failed to save uploaded file")
				return
			}
			info, err := root.Lstat(upload.relativePath)
			if err != nil {
				Fail(c, http.StatusInternalServerError, "failed to inspect uploaded file")
				return
			}
			results = append(results, FileInfo{
				Name:    upload.fileName,
				Path:    upload.relativePath,
				IsDir:   false,
				Size:    size,
				ModTime: info.ModTime().Unix(),
			})
		}

		OK(c, results)
	}
}

// chunkUploadMeta 分片上传状态
type chunkUploadMeta struct {
	mu           sync.Mutex
	TotalChunks  int
	Received     map[int]int64
	FilePath     string
	RelativePath string
	ChunkDir     string
	TotalBytes   int64
	UpdatedAt    time.Time
	Completed    bool
	Expired      bool
	released     bool
}

// ChunkUploadStore 保存单个 HTTP runtime 的分片上传状态。
type ChunkUploadStore struct {
	sync.Mutex
	m            map[string]*chunkUploadMeta
	rootDir      string
	active       int
	activeBytes  atomic.Int64
	maxActive    int
	maxBytes     int64
	requestSlots chan struct{}
	closed       bool
}

type chunkUploadStoreOptions struct {
	rootDir       string
	maxActive     int
	maxBytes      int64
	maxConcurrent int
}

// NewChunkUploadStore 创建独立的分片上传状态存储。
func NewChunkUploadStore(options ...chunkUploadStoreOptions) *ChunkUploadStore {
	opt := chunkUploadStoreOptions{
		rootDir:       filepath.Join(os.TempDir(), "fkteams-chunks", uuid.NewString()),
		maxActive:     maxActiveChunkUploads,
		maxBytes:      maxChunkUploadTempBytes,
		maxConcurrent: maxConcurrentChunkRequests,
	}
	if len(options) > 0 {
		if options[0].rootDir != "" {
			opt.rootDir = options[0].rootDir
		}
		if options[0].maxActive > 0 {
			opt.maxActive = options[0].maxActive
		}
		if options[0].maxBytes > 0 {
			opt.maxBytes = options[0].maxBytes
		}
		if options[0].maxConcurrent > 0 {
			opt.maxConcurrent = options[0].maxConcurrent
		}
	}
	return &ChunkUploadStore{
		m:            make(map[string]*chunkUploadMeta),
		rootDir:      opt.rootDir,
		maxActive:    opt.maxActive,
		maxBytes:     opt.maxBytes,
		requestSlots: make(chan struct{}, opt.maxConcurrent),
	}
}

func (s *ChunkUploadStore) beginRequest() bool {
	if s == nil {
		return false
	}
	s.Lock()
	defer s.Unlock()
	if s.closed {
		return false
	}
	select {
	case s.requestSlots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *ChunkUploadStore) endRequest() {
	if s == nil {
		return
	}
	select {
	case <-s.requestSlots:
	default:
	}
}

func (s *ChunkUploadStore) getOrCreate(uploadID string, totalChunks int, filePath, relativePath string) (*chunkUploadMeta, error) {
	key := sanitizeUploadID(uploadID)
	s.Lock()
	defer s.Unlock()
	if s.closed {
		return nil, fmt.Errorf("chunk upload store is closed")
	}
	if existing := s.m[key]; existing != nil {
		if existing.TotalChunks != totalChunks || existing.FilePath != filePath || existing.RelativePath != relativePath {
			return nil, fmt.Errorf("upload metadata does not match the existing upload")
		}
		return existing, nil
	}
	if s.maxActive > 0 && s.active >= s.maxActive {
		return nil, errChunkUploadCapacity
	}
	meta := &chunkUploadMeta{
		TotalChunks:  totalChunks,
		Received:     make(map[int]int64),
		FilePath:     filePath,
		RelativePath: relativePath,
		ChunkDir:     filepath.Join(s.rootDir, key),
		UpdatedAt:    time.Now(),
	}
	s.m[key] = meta
	s.active++
	return meta, nil
}

func (s *ChunkUploadStore) removeExpired(now time.Time) {
	s.Lock()
	items := make(map[string]*chunkUploadMeta, len(s.m))
	for key, meta := range s.m {
		items[key] = meta
	}
	s.Unlock()

	for key, meta := range items {
		meta.mu.Lock()
		expired := now.Sub(meta.UpdatedAt) >= chunkUploadTTL
		if expired {
			if err := os.RemoveAll(meta.ChunkDir); err != nil {
				log.Printf("failed to remove expired chunk upload: path=%s, err=%v", meta.ChunkDir, err)
				meta.mu.Unlock()
				continue
			}
			s.Lock()
			if s.m[key] == meta && now.Sub(meta.UpdatedAt) >= chunkUploadTTL {
				delete(s.m, key)
				meta.Expired = true
				s.releaseLocked(meta)
			}
			s.Unlock()
		}
		meta.mu.Unlock()
	}
}

func (s *ChunkUploadStore) reserveBytes(delta int64) bool {
	if delta == 0 {
		return true
	}
	if delta < 0 {
		s.activeBytes.Add(delta)
		return true
	}
	for {
		current := s.activeBytes.Load()
		if s.maxBytes > 0 && current+delta > s.maxBytes {
			return false
		}
		if s.activeBytes.CompareAndSwap(current, current+delta) {
			return true
		}
	}
}

func (s *ChunkUploadStore) finish(meta *chunkUploadMeta) {
	s.Lock()
	s.releaseLocked(meta)
	s.Unlock()
}

func (s *ChunkUploadStore) releaseLocked(meta *chunkUploadMeta) {
	if meta.released {
		return
	}
	meta.released = true
	if s.active > 0 {
		s.active--
	}
	if meta.TotalBytes != 0 {
		s.activeBytes.Add(-meta.TotalBytes)
	}
}

func (s *ChunkUploadStore) Close() {
	if s == nil {
		return
	}
	s.Lock()
	s.m = make(map[string]*chunkUploadMeta)
	s.active = 0
	s.closed = true
	s.activeBytes.Store(0)
	rootDir := s.rootDir
	s.Unlock()
	if err := os.RemoveAll(rootDir); err != nil {
		log.Printf("failed to remove chunk upload store: path=%s, err=%v", rootDir, err)
	}
}

// UploadChunkHandler 处理分片上传
// 参数: file(分片内容), uploadId(上传标识), chunkIndex(分片序号,0-based), totalChunks(总分片数), fileName(文件名), path(可选子目录)

// UploadChunkHandler 处理当前 HTTP runtime 的分片上传。
func (rt *Runtime) UploadChunkHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		uploads := rt.ChunkUploads
		if uploads == nil || !uploads.beginRequest() {
			Fail(c, http.StatusTooManyRequests, "too many concurrent chunk upload requests")
			return
		}
		defer uploads.endRequest()
		defer func() {
			if c.Request.MultipartForm != nil {
				_ = c.Request.MultipartForm.RemoveAll()
			}
		}()

		baseDir, err := getWorkspaceDir()
		if err != nil {
			Fail(c, http.StatusInternalServerError, err.Error())
			return
		}

		uploadID := c.PostForm("uploadId")
		chunkIndexStr := c.PostForm("chunkIndex")
		totalChunksStr := c.PostForm("totalChunks")
		fileName := c.PostForm("fileName")

		if uploadID == "" || len(uploadID) > maxUploadIDBytes || chunkIndexStr == "" || totalChunksStr == "" || fileName == "" {
			Fail(c, http.StatusBadRequest, "缺少必要参数")
			return
		}

		chunkIndex, err := strconv.Atoi(chunkIndexStr)
		if err != nil || chunkIndex < 0 {
			Fail(c, http.StatusBadRequest, "无效的分片序号")
			return
		}

		totalChunks, err := strconv.Atoi(totalChunksStr)
		if err != nil || totalChunks <= 0 || totalChunks > maxUploadChunks {
			Fail(c, http.StatusBadRequest, "无效的总分片数")
			return
		}

		if chunkIndex >= totalChunks {
			Fail(c, http.StatusBadRequest, "分片序号超出范围")
			return
		}

		// 校验文件名安全性
		safeName, err := safeUploadedFileName(fileName)
		if err != nil {
			Fail(c, http.StatusBadRequest, "无效的文件名")
			return
		}

		// 验证目标子目录
		subPath := c.PostForm("path")
		resolvedTarget, err := pathguard.ResolveWorkspace(baseDir, subPath)
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}

		targetRelPath := resolvedTarget.RelPath
		finalPath := filepath.Join(resolvedTarget.AbsPath, safeName)
		relativePath := safeName
		if targetRelPath != "" {
			relativePath = filepath.Join(targetRelPath, safeName)
		}

		file, err := c.FormFile("file")
		if err != nil {
			Fail(c, http.StatusBadRequest, "未找到分片文件")
			return
		}
		if file.Size < 0 || file.Size > maxUploadChunkBytes {
			Fail(c, http.StatusRequestEntityTooLarge, "upload chunk is too large")
			return
		}

		uploads.removeExpired(time.Now())
		meta, err := uploads.getOrCreate(uploadID, totalChunks, finalPath, relativePath)
		if err != nil {
			if errors.Is(err, errChunkUploadCapacity) {
				Fail(c, http.StatusTooManyRequests, err.Error())
				return
			}
			Fail(c, http.StatusConflict, err.Error())
			return
		}

		meta.mu.Lock()
		defer meta.mu.Unlock()
		if meta.Expired {
			Fail(c, http.StatusGone, "chunk upload has expired")
			return
		}
		meta.UpdatedAt = time.Now()
		if meta.Completed {
			writeCompletedChunkUploadResponse(c, uploadID, meta, baseDir)
			return
		}

		previousSize := meta.Received[chunkIndex]
		if meta.TotalBytes-previousSize+file.Size > maxChunkedFileBytes {
			Fail(c, http.StatusRequestEntityTooLarge, "chunked file is too large")
			return
		}
		reservedDelta := file.Size - previousSize
		if !uploads.reserveBytes(reservedDelta) {
			Fail(c, http.StatusTooManyRequests, errChunkUploadCapacity.Error())
			return
		}
		reserved := true
		defer func() {
			if reserved {
				uploads.activeBytes.Add(-reservedDelta)
			}
		}()
		if err := os.MkdirAll(meta.ChunkDir, 0700); err != nil {
			log.Printf("failed to create chunk dir: path=%s, err=%v", meta.ChunkDir, err)
			Fail(c, http.StatusInternalServerError, "创建临时目录失败")
			return
		}
		chunkPath := filepath.Join(meta.ChunkDir, strconv.Itoa(chunkIndex))
		written, err := saveUploadedFileAtomic(file, chunkPath, maxUploadChunkBytes)
		if err != nil {
			log.Printf("failed to save chunk: path=%s, err=%v", chunkPath, err)
			Fail(c, http.StatusInternalServerError, "保存分片失败")
			return
		}
		actualDelta := written - previousSize
		uploads.activeBytes.Add(actualDelta - reservedDelta)
		reserved = false
		meta.Received[chunkIndex] = written
		meta.TotalBytes = meta.TotalBytes - previousSize + written
		receivedCount := len(meta.Received)
		allReceived := receivedCount == meta.TotalChunks

		if !allReceived {
			OK(c, gin.H{
				"uploadId":   uploadID,
				"chunkIndex": chunkIndex,
				"received":   receivedCount,
				"total":      meta.TotalChunks,
				"completed":  false,
			})
			return
		}

		if err := ensureWorkspaceDirectoryNoSymlinks(baseDir, targetRelPath); err != nil {
			Fail(c, http.StatusBadRequest, "invalid upload directory")
			return
		}
		root, err := os.OpenRoot(baseDir)
		if err != nil {
			Fail(c, http.StatusInternalServerError, "failed to open workspace")
			return
		}
		if err := rejectRootSymlinkComponents(root, targetRelPath); err != nil {
			root.Close()
			Fail(c, http.StatusBadRequest, "invalid upload directory")
			return
		}
		targetInfo, targetErr := root.Lstat(meta.RelativePath)
		if targetErr == nil && !targetInfo.Mode().IsRegular() {
			root.Close()
			Fail(c, http.StatusBadRequest, "upload target is not a regular file")
			return
		}
		if targetErr != nil && !errors.Is(targetErr, os.ErrNotExist) {
			root.Close()
			Fail(c, http.StatusInternalServerError, "failed to inspect upload target")
			return
		}
		_, assembleErr := assembleChunkUpload(root, meta.RelativePath, meta.ChunkDir, meta.TotalChunks)
		closeErr := root.Close()
		if assembleErr != nil {
			log.Printf("failed to assemble chunk upload: path=%s, err=%v", meta.FilePath, assembleErr)
			Fail(c, http.StatusInternalServerError, "合并分片失败")
			return
		}
		if closeErr != nil {
			Fail(c, http.StatusInternalServerError, "failed to close workspace")
			return
		}

		meta.Completed = true
		meta.UpdatedAt = time.Now()
		if err := os.RemoveAll(meta.ChunkDir); err != nil {
			log.Printf("failed to remove completed chunk upload: path=%s, err=%v", meta.ChunkDir, err)
		} else {
			uploads.finish(meta)
		}
		writeCompletedChunkUploadResponse(c, uploadID, meta, baseDir)
	}
}

func assembleChunkUpload(root *os.Root, destination, chunkDir string, totalChunks int) (int64, error) {
	return atomicfile.WriteInRoot(root, destination, 0644, func(writer io.Writer) (int64, error) {
		var total int64
		for index := 0; index < totalChunks; index++ {
			chunk, err := os.Open(filepath.Join(chunkDir, strconv.Itoa(index)))
			if err != nil {
				return total, fmt.Errorf("open chunk %d: %w", index, err)
			}
			written, copyErr := io.Copy(writer, io.LimitReader(chunk, maxChunkedFileBytes-total+1))
			closeErr := chunk.Close()
			total += written
			if copyErr != nil {
				return total, fmt.Errorf("copy chunk %d: %w", index, copyErr)
			}
			if closeErr != nil {
				return total, fmt.Errorf("close chunk %d: %w", index, closeErr)
			}
			if total > maxChunkedFileBytes {
				return total, fmt.Errorf("chunked file is too large")
			}
		}
		return total, nil
	})
}

func writeCompletedChunkUploadResponse(c *gin.Context, uploadID string, meta *chunkUploadMeta, baseDir string) {
	file, info, err := openWorkspaceRegularFileNoSymlinks(baseDir, meta.RelativePath)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "failed to inspect uploaded file")
		return
	}
	if err := file.Close(); err != nil {
		Fail(c, http.StatusInternalServerError, "failed to close uploaded file")
		return
	}
	OK(c, gin.H{
		"uploadId":  uploadID,
		"completed": true,
		"file": FileInfo{
			Name:    filepath.Base(meta.RelativePath),
			Path:    meta.RelativePath,
			IsDir:   false,
			Size:    info.Size(),
			ModTime: info.ModTime().Unix(),
		},
	})
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
		baseDir, err := getWorkspaceDir()
		if err != nil {
			Fail(c, http.StatusInternalServerError, err.Error())
			return
		}

		filePath := c.Query("path")
		if filePath == "" {
			Fail(c, http.StatusBadRequest, "缺少 path 参数")
			return
		}

		resolved, info, err := resolveWorkspaceEntryNoSymlinks(baseDir, filePath)
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}

		if !info.IsDir() {
			file, openedInfo, err := openWorkspaceRegularFileNoSymlinks(baseDir, resolved.RelPath)
			if err != nil {
				Fail(c, http.StatusBadRequest, err.Error())
				return
			}
			defer file.Close()
			c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{
				"filename": filepath.Base(resolved.RelPath),
			}))
			http.ServeContent(c.Writer, c.Request, openedInfo.Name(), openedInfo.ModTime(), file)
			return
		}
		if !acquireArchiveSlot() {
			Fail(c, http.StatusTooManyRequests, "too many archive requests")
			return
		}
		defer releaseArchiveSlot()

		root, err := os.OpenRoot(baseDir)
		if err != nil {
			Fail(c, http.StatusInternalServerError, "failed to open workspace")
			return
		}
		defer root.Close()
		source := workspaceFSPath(resolved.RelPath)
		if err := validateArchive(c.Request.Context(), root.FS(), []archiveSource{{source: source}}); err != nil {
			writeArchiveValidationError(c, err)
			return
		}

		// 目录：校验规模后流式打包，避免遍历中途才发现资源超限。
		dirName := filepath.Base(resolved.AbsPath)
		c.Header("Content-Type", "application/zip")
		c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{
			"filename": dirName + ".zip",
		}))
		c.Status(http.StatusOK)

		zw := zip.NewWriter(c.Writer)
		if err := writeArchive(c.Request.Context(), root.FS(), zw, []archiveSource{{source: source}}); err != nil {
			log.Printf("failed to stream directory archive: path=%s, err=%v", resolved.RelPath, err)
		}
		if err := zw.Close(); err != nil {
			log.Printf("failed to close directory archive: path=%s, err=%v", resolved.RelPath, err)
		}
	}
}

// BatchDownloadHandler 批量下载：将多个文件/文件夹打包为单个 zip。
func BatchDownloadHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		baseDir, err := getWorkspaceDir()
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
		if len(req.Paths) > maxBatchDownloadPaths {
			Fail(c, http.StatusRequestEntityTooLarge, "too many download paths")
			return
		}

		root, err := os.OpenRoot(baseDir)
		if err != nil {
			Fail(c, http.StatusInternalServerError, "failed to open workspace")
			return
		}
		defer root.Close()

		sources := make([]archiveSource, 0, len(req.Paths))
		seen := make(map[string]bool, len(req.Paths))
		for _, p := range req.Paths {
			resolved, info, err := resolveWorkspaceEntryNoSymlinks(baseDir, p)
			if err != nil {
				Fail(c, http.StatusBadRequest, err.Error())
				return
			}
			if _, exists := seen[resolved.RelPath]; exists {
				continue
			}
			seen[resolved.RelPath] = info.IsDir()
			sources = append(sources, archiveSource{
				source:      workspaceFSPath(resolved.RelPath),
				archiveBase: filepath.ToSlash(resolved.RelPath),
			})
		}
		if archiveSourcesOverlap(seen) {
			Fail(c, http.StatusBadRequest, "download paths must not overlap")
			return
		}
		if !acquireArchiveSlot() {
			Fail(c, http.StatusTooManyRequests, "too many archive requests")
			return
		}
		defer releaseArchiveSlot()
		if err := validateArchive(c.Request.Context(), root.FS(), sources); err != nil {
			writeArchiveValidationError(c, err)
			return
		}

		c.Header("Content-Type", "application/zip")
		c.Header("Content-Disposition", `attachment; filename="download.zip"`)
		c.Status(http.StatusOK)

		zw := zip.NewWriter(c.Writer)
		if err := writeArchive(c.Request.Context(), root.FS(), zw, sources); err != nil {
			log.Printf("failed to stream batch archive: err=%v", err)
		}
		if err := zw.Close(); err != nil {
			log.Printf("failed to close batch archive: err=%v", err)
		}
	}
}

type archiveSource struct {
	source      string
	archiveBase string
}

type archiveBudget struct {
	entries int
	bytes   int64
}

func acquireArchiveSlot() bool {
	select {
	case archiveSlots <- struct{}{}:
		return true
	default:
		return false
	}
}

func releaseArchiveSlot() {
	<-archiveSlots
}

func archiveSourcesOverlap(paths map[string]bool) bool {
	for parent, isDir := range paths {
		if !isDir {
			continue
		}
		if parent == "" && len(paths) > 1 {
			return true
		}
		prefix := parent + string(filepath.Separator)
		for candidate := range paths {
			if strings.HasPrefix(candidate, prefix) {
				return true
			}
		}
	}
	return false
}

func workspaceFSPath(relativePath string) string {
	path := filepath.ToSlash(relativePath)
	if path == "" {
		return "."
	}
	return path
}

func validateArchive(ctx context.Context, filesystem fs.FS, sources []archiveSource) error {
	return processArchive(ctx, filesystem, nil, sources)
}

func writeArchive(ctx context.Context, filesystem fs.FS, writer *zip.Writer, sources []archiveSource) error {
	return processArchive(ctx, filesystem, writer, sources)
}

func processArchive(ctx context.Context, filesystem fs.FS, writer *zip.Writer, sources []archiveSource) error {
	budget := &archiveBudget{}
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := processArchiveSource(ctx, filesystem, writer, source, budget); err != nil {
			return err
		}
	}
	return nil
}

func processArchiveSource(ctx context.Context, filesystem fs.FS, writer *zip.Writer, source archiveSource, budget *archiveBudget) error {
	rootInfo, err := fs.Stat(filesystem, source.source)
	if err != nil {
		return err
	}
	if !rootInfo.IsDir() {
		name := source.archiveBase
		if name == "" {
			name = rootInfo.Name()
		}
		return processArchiveEntry(ctx, filesystem, writer, source.source, name, rootInfo, budget)
	}

	return fs.WalkDir(filesystem, source.source, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if current != source.source && strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("symbolic links are not allowed in archives")
		}
		relative := strings.TrimPrefix(current, source.source)
		relative = strings.TrimPrefix(relative, "/")
		name := source.archiveBase
		if relative != "" {
			if name == "" {
				name = relative
			} else {
				name += "/" + relative
			}
		}
		if name == "" {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return processArchiveEntry(ctx, filesystem, writer, current, name, info, budget)
	})
}

func processArchiveEntry(ctx context.Context, filesystem fs.FS, writer *zip.Writer, sourcePath, name string, info fs.FileInfo, budget *archiveBudget) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	budget.entries++
	if budget.entries > maxArchiveEntries {
		return fmt.Errorf("%w: too many archive entries", errArchiveLimit)
	}
	if !info.IsDir() {
		if info.Size() < 0 || budget.bytes+info.Size() > maxArchiveUncompressedBytes {
			return fmt.Errorf("%w: archive is too large", errArchiveLimit)
		}
		budget.bytes += info.Size()
	}
	if writer == nil {
		return nil
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = filepath.ToSlash(name)
	if info.IsDir() {
		header.Name = strings.TrimSuffix(header.Name, "/") + "/"
	} else {
		header.Method = zip.Deflate
	}
	destination, err := writer.CreateHeader(header)
	if err != nil || info.IsDir() {
		return err
	}
	file, err := filesystem.Open(sourcePath)
	if err != nil {
		return err
	}
	remaining := maxArchiveUncompressedBytes - (budget.bytes - info.Size())
	written, copyErr := io.Copy(destination, io.LimitReader(&contextReader{ctx: ctx, reader: file}, remaining+1))
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written > remaining {
		return fmt.Errorf("%w: archive changed while streaming", errArchiveLimit)
	}
	budget.bytes += written - info.Size()
	return ctx.Err()
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}

func writeArchiveValidationError(c *gin.Context, err error) {
	if errors.Is(err, errArchiveLimit) {
		Fail(c, http.StatusRequestEntityTooLarge, err.Error())
		return
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return
	}
	log.Printf("failed to validate archive: err=%v", err)
	Fail(c, http.StatusInternalServerError, "failed to prepare archive")
}

// DeleteFileHandler 删除工作目录中的文件或目录
// JSON body: {"path": "相对路径", "force": false}
// force 为 true 时可删除非空目录
func DeleteFileHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		baseDir, err := getWorkspaceDir()
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

		resolved, info, err := resolveWorkspaceEntryNoSymlinks(baseDir, req.Path)
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}

		if resolved.RelPath == "" {
			Fail(c, http.StatusBadRequest, "无效的文件路径")
			return
		}
		root, err := os.OpenRoot(baseDir)
		if err != nil {
			Fail(c, http.StatusInternalServerError, "failed to open workspace")
			return
		}
		defer root.Close()
		if err := rejectRootSymlinkComponents(root, resolved.RelPath); err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}
		currentInfo, err := root.Lstat(workspaceFSPath(resolved.RelPath))
		if err != nil || !os.SameFile(info, currentInfo) {
			Fail(c, http.StatusConflict, "path changed before deletion")
			return
		}

		if info.IsDir() {
			if !req.Force {
				directory, _, err := openWorkspaceDirectoryNoSymlinks(baseDir, resolved.RelPath)
				if err != nil {
					Fail(c, http.StatusConflict, "path changed before deletion")
					return
				}
				entries, readErr := directory.ReadDir(1)
				closeErr := directory.Close()
				if readErr != nil && !errors.Is(readErr, io.EOF) {
					Fail(c, http.StatusInternalServerError, "读取目录失败")
					return
				}
				if closeErr != nil {
					log.Printf("failed to close directory before delete: path=%s, err=%v", resolved.RelPath, closeErr)
				}
				if len(entries) > 0 {
					Fail(c, http.StatusBadRequest, "目录非空，请设置 force:true 确认删除")
					return
				}
				if err := root.Remove(resolved.RelPath); err != nil {
					log.Printf("failed to delete empty directory: path=%s, err=%v", resolved.RelPath, err)
					Fail(c, http.StatusConflict, "directory changed before deletion")
					return
				}
			} else if err := root.RemoveAll(resolved.RelPath); err != nil {
				log.Printf("failed to delete directory: path=%s, err=%v", resolved.RelPath, err)
				Fail(c, http.StatusInternalServerError, "删除目录失败")
				return
			}
		} else {
			if err := root.Remove(resolved.RelPath); err != nil {
				log.Printf("failed to delete file: path=%s, err=%v", resolved.RelPath, err)
				Fail(c, http.StatusInternalServerError, "删除文件失败")
				return
			}
		}

		OK(c, nil)
	}
}

// serveOpenedFileContent 使用已校验并打开的文件描述符提供内容，避免再次按路径打开。
func serveOpenedFileContent(c *gin.Context, file *os.File, fullPath string, info os.FileInfo) {
	setUntrustedContentHeaders(c)
	if c.Writer.Header().Get("Content-Type") == "" {
		c.Header("Content-Type", previewContentType(fullPath))
	}
	http.ServeContent(c.Writer, c.Request, info.Name(), info.ModTime(), file)
}

// setUntrustedContentHeaders 将工作区文件限制在无脚本、无同源权限的文档沙箱中。
// 安全边界放在响应层，避免直接打开文件 URL 时绕过前端 iframe sandbox。
func setUntrustedContentHeaders(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Security-Policy", untrustedContentSecurityPolicy)
	c.Header("Cross-Origin-Resource-Policy", "same-origin")
	c.Header("Permissions-Policy", "camera=(), geolocation=(), microphone=()")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("X-Content-Type-Options", "nosniff")
}

// ServeFileHandler 以 inline 方式提供工作目录中的文件（用于 HTML 预览等场景）
// 相对路径通过 URL wildcard 传入，浏览器可自然解析相对引用
func ServeFileHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		baseDir, err := getWorkspaceDir()
		if err != nil {
			Fail(c, http.StatusInternalServerError, err.Error())
			return
		}

		relativePath := strings.TrimPrefix(c.Param("filepath"), "/")
		if relativePath == "" {
			Fail(c, http.StatusBadRequest, "缺少文件路径")
			return
		}

		resolved, info, err := resolveWorkspaceEntryNoSymlinks(baseDir, relativePath)
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}

		if info.IsDir() {
			relativePath = filepath.Join(resolved.RelPath, "index.html")
		}

		file, fileInfo, err := openWorkspaceRegularFileNoSymlinks(baseDir, relativePath)
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}
		defer file.Close()
		serveOpenedFileContent(c, file, filepath.Join(baseDir, relativePath), fileInfo)
	}
}
