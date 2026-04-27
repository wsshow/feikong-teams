package file

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"fkteams/mdiff"
	"fkteams/tools/approval"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/spf13/afero"
)

// FileTools 文件工具实例，每个agent可以有独立的实例
type FileTools struct {
	// securedFs 是受限制的文件系统，只允许访问指定目录
	securedFs afero.Fs
	// osFs 是无限制的操作系统文件系统，用于访问已审批的外部路径
	osFs afero.Fs
	// allowedBaseDir 是允许访问的基础目录
	allowedBaseDir string
}

// NewFileTools 创建一个新的文件工具实例
// baseDir 是允许操作的基础目录（默认 ~/.fkteams/workspace）
func NewFileTools(baseDir string) (*FileTools, error) {
	absPath, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("无法获取绝对路径: %w", err)
	}

	// 检查目录是否存在，如果不存在则创建
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		if err := os.MkdirAll(absPath, 0755); err != nil {
			return nil, fmt.Errorf("无法创建目录 %s: %w", absPath, err)
		}
	}

	securedFs := afero.NewBasePathFs(afero.NewOsFs(), absPath)

	return &FileTools{
		securedFs:      securedFs,
		osFs:           afero.NewOsFs(),
		allowedBaseDir: absPath,
	}, nil
}

// resolvedPath 路径解析结果
type resolvedPath struct {
	fs   afero.Fs
	path string
}

// resolvePath 解析用户路径，支持工作目录内和工作目录外的路径访问
// 工作目录内的路径直接访问，工作目录外的绝对路径需要用户审批
func (ft *FileTools) resolvePath(ctx context.Context, userPath string) (*resolvedPath, error) {
	if userPath == "" {
		return nil, fmt.Errorf("路径不能为空")
	}

	// 1. 尝试解析为工作目录内的路径
	if relPath, err := ft.workspacePath(userPath); err == nil {
		return &resolvedPath{fs: ft.securedFs, path: relPath}, nil
	}

	// 2. 仅支持绝对路径访问外部文件
	cleanPath := filepath.Clean(userPath)
	if !filepath.IsAbs(cleanPath) {
		return nil, fmt.Errorf("路径 %s 不在工作目录 %s 内，如需访问外部文件请使用绝对路径", userPath, ft.allowedBaseDir)
	}

	// 3. 统一审批流程（文件工具使用父目录作为审批 key）
	parentDir := filepath.Dir(cleanPath)
	info := fmt.Sprintf("需要审批: 访问工作目录外的路径\n  路径: %s\n  工作目录: %s", cleanPath, ft.allowedBaseDir)
	if err := approval.Require(ctx, approval.StoreFile, parentDir, info); err != nil {
		if errors.Is(err, approval.ErrRejected) {
			return nil, fmt.Errorf("用户拒绝了对 %s 的访问", cleanPath)
		}
		return nil, err
	}

	return &resolvedPath{fs: ft.osFs, path: cleanPath}, nil
}

// readFileLines 流式读取文件全部行
func readFileLines(fs afero.Fs, path string) (lines []string, totalLines int, hasTrailingNewline bool, err error) {
	scanner, file, err := openScanner(fs, path)
	if err != nil {
		return nil, 0, false, err
	}
	defer file.Close()

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, false, err
	}

	totalLines = len(lines)
	hasTrailingNewline = hasFileTrailingNewline(fs, path)

	// Scanner 在 "text\n\n" 时会产出尾随空行，需去掉以还原真实内容
	if totalLines > 0 && hasTrailingNewline && lines[totalLines-1] == "" {
		lines = lines[:totalLines-1]
		totalLines--
	}
	return lines, totalLines, hasTrailingNewline, nil
}

// readFileLinesRange 流式读取指定行范围，同时统计总行数
func readFileLinesRange(fs afero.Fs, path string, startLine, endLine int) (lines []string, totalLines int, err error) {
	scanner, file, err := openScanner(fs, path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum >= startLine && (endLine == 0 || lineNum <= endLine) {
			lines = append(lines, scanner.Text())
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, err
	}
	return lines, lineNum, nil
}

// hasFileTrailingNewline 检测文件末尾是否为换行符
func hasFileTrailingNewline(fs afero.Fs, path string) bool {
	info, err := fs.Stat(path)
	if err != nil || info.Size() == 0 {
		return false
	}
	f, err := fs.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	buf := make([]byte, 1)
	_, err = f.ReadAt(buf, info.Size()-1)
	if err == nil {
		return buf[0] == '\n'
	}
	// ReadAt 不可用时的 fallback
	if _, seekErr := f.Seek(info.Size()-1, 0); seekErr != nil {
		return false
	}
	_, readErr := f.Read(buf)
	return readErr == nil && buf[0] == '\n'
}

// joinLines 将行数组拼接为文件内容，按需保留尾部换行
func joinLines(lines []string, hasTrailingNewline bool) string {
	content := strings.Join(lines, "\n")
	if hasTrailingNewline {
		content += "\n"
	}
	return content
}

// countLines 统一计算文本行数，忽略尾部换行符
func countLines(content string) int {
	text := strings.TrimRight(content, "\n")
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}

// workspacePath 验证路径在工作目录内，返回相对路径
func (ft *FileTools) workspacePath(userPath string) (string, error) {
	if userPath == "" {
		return "", fmt.Errorf("路径不能为空")
	}

	// 清理路径
	cleanPath := filepath.Clean(userPath)

	// 转换为绝对路径以检查路径穿越
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return "", fmt.Errorf("无法解析路径: %w", err)
	}

	// 检查路径是否在允许的目录内
	if strings.HasPrefix(absPath, ft.allowedBaseDir) {
		relPath, err := filepath.Rel(ft.allowedBaseDir, absPath)
		if err != nil {
			return "", fmt.Errorf("无法计算相对路径: %w", err)
		}
		if !strings.HasPrefix(relPath, "..") {
			return relPath, nil
		}
	}

	// 回退策略：将路径视为相对于 allowedBaseDir 解析
	// 适用于 file_patch 等工具中路径不包含工作目录前缀的场景
	if !filepath.IsAbs(cleanPath) {
		altAbsPath := filepath.Clean(filepath.Join(ft.allowedBaseDir, cleanPath))
		if strings.HasPrefix(altAbsPath, ft.allowedBaseDir) {
			relPath, err := filepath.Rel(ft.allowedBaseDir, altAbsPath)
			if err == nil && !strings.HasPrefix(relPath, "..") {
				return relPath, nil
			}
		}
	}

	return "", fmt.Errorf("访问被拒绝: 路径 %s 不在允许的目录 %s 内", absPath, ft.allowedBaseDir)
}

// maxDefaultLines 默认最大读取行数限制
const maxDefaultLines = 1000

// maxFileSize 单文件最大大小 (10MB)，防止 OOM
const maxFileSize = 10 << 20

// maxScannerLineSize bufio.Scanner 单行最大长度 (1MB)
const maxScannerLineSize = 1 << 20

// maxReadLines 单次读取最大行数
const maxReadLines = 2000

// openScanner 打开文件并返回配置好的 bufio.Scanner
func openScanner(fs afero.Fs, path string) (*bufio.Scanner, afero.File, error) {
	info, err := fs.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	if info.Size() > maxFileSize {
		return nil, nil, fmt.Errorf("文件过大 (%d bytes)，超过限制 (%d bytes)", info.Size(), maxFileSize)
	}
	file, err := fs.Open(path)
	if err != nil {
		return nil, nil, err
	}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(nil, maxScannerLineSize)
	return scanner, file, nil
}

// FileReadRequest 读取文件请求
type FileReadRequest struct {
	Filepath  string `json:"filepath" jsonschema:"description=要读取的文件路径,required"`
	StartLine int    `json:"start_line,omitempty" jsonschema:"description=起始行号(从1开始),不填则从第一行开始"`
	EndLine   int    `json:"end_line,omitempty" jsonschema:"description=结束行号,不填则读到文件末尾"`
}

// FileReadResponse 读取文件响应
type FileReadResponse struct {
	Content      string `json:"content" jsonschema:"description=文件内容"`
	TotalLines   int    `json:"total_lines" jsonschema:"description=文件总行数"`
	ReadRange    string `json:"read_range,omitempty" jsonschema:"description=实际读取的行范围"`
	Truncated    bool   `json:"truncated,omitempty" jsonschema:"description=内容是否被截断"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// FileRead 读取文件内容
func (ft *FileTools) FileRead(ctx context.Context, req *FileReadRequest) (*FileReadResponse, error) {
	if req.Filepath == "" {
		return &FileReadResponse{ErrorMessage: "filepath 参数是必需的"}, nil
	}

	rp, err := ft.resolvePath(ctx, req.Filepath)
	if err != nil {
		return nil, err
	}

	// 部分读取：只读需要的行，同时统计总行数
	if req.StartLine > 0 || req.EndLine > 0 {
		startLine := req.StartLine
		if startLine < 1 {
			startLine = 1
		}
		endLine := req.EndLine
		if endLine > 0 && endLine-startLine+1 > maxReadLines {
			return &FileReadResponse{
				ErrorMessage: fmt.Sprintf("读取范围过大 (%d 行)，超过上限 %d 行", endLine-startLine+1, maxReadLines),
			}, nil
		}

		lines, totalLines, err := readFileLinesRange(rp.fs, rp.path, startLine, endLine)
		if err != nil {
			return &FileReadResponse{ErrorMessage: fmt.Sprintf("读取文件失败: %v", err)}, nil
		}
		if len(lines) == 0 {
			return &FileReadResponse{
				ErrorMessage: fmt.Sprintf("起始行号 %d 超出文件总行数 %d", startLine, totalLines),
				TotalLines:   totalLines,
			}, nil
		}
		return &FileReadResponse{
			Content:    strings.Join(lines, "\n"),
			TotalLines: totalLines,
			ReadRange:  fmt.Sprintf("%d-%d", startLine, startLine+len(lines)-1),
		}, nil
	}

	// 全量读取：默认限制前 N 行
	lines, totalLines, _, err := readFileLines(rp.fs, rp.path)
	if err != nil {
		return &FileReadResponse{ErrorMessage: fmt.Sprintf("读取文件失败: %v", err)}, nil
	}
	if totalLines > maxDefaultLines {
		content := strings.Join(lines[:maxDefaultLines], "\n")
		content += fmt.Sprintf("\n\n... 已截断，用 start_line=%d 继续读取剩余 %d 行", maxDefaultLines+1, totalLines-maxDefaultLines)
		return &FileReadResponse{
			Content:    content,
			TotalLines: totalLines,
			ReadRange:  fmt.Sprintf("1-%d", maxDefaultLines),
			Truncated:  true,
		}, nil
	}
	return &FileReadResponse{
		Content:    strings.Join(lines, "\n"),
		TotalLines: totalLines,
		ReadRange:  fmt.Sprintf("1-%d", totalLines),
	}, nil
}

// FileWriteRequest 写入文件请求
type FileWriteRequest struct {
	Filepath string `json:"filepath" jsonschema:"description=要写入的文件路径,required"`
	Content  string `json:"content" jsonschema:"description=要写入的内容,required"`
}

// FileWriteResponse 写入文件响应
type FileWriteResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	TotalLines   int    `json:"total_lines" jsonschema:"description=写入后文件总行数"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// FileWrite 写入文件（覆盖模式）
func (ft *FileTools) FileWrite(ctx context.Context, req *FileWriteRequest) (*FileWriteResponse, error) {
	if req.Filepath == "" {
		return &FileWriteResponse{
			ErrorMessage: "filepath 参数是必需的",
		}, nil
	}

	rp, err := ft.resolvePath(ctx, req.Filepath)
	if err != nil {
		return nil, err
	}

	// 自动创建父目录
	dir := filepath.Dir(rp.path)
	if dir != "." {
		if err := rp.fs.MkdirAll(dir, 0755); err != nil {
			return &FileWriteResponse{
				ErrorMessage: fmt.Sprintf("创建目录失败: %v", err),
			}, nil
		}
	}

	err = afero.WriteFile(rp.fs, rp.path, []byte(req.Content), 0644)
	if err != nil {
		return &FileWriteResponse{
			ErrorMessage: fmt.Sprintf("写入文件失败: %v", err),
		}, nil
	}

	totalLines := countLines(req.Content)

	return &FileWriteResponse{
		Message:    fmt.Sprintf("已写入 %s (%d 行)", req.Filepath, totalLines),
		TotalLines: totalLines,
	}, nil
}

// FileAppendRequest 追加写入文件请求
type FileAppendRequest struct {
	Filepath string `json:"filepath" jsonschema:"description=要追加写入的文件路径,required"`
	Content  string `json:"content" jsonschema:"description=要追加的内容,required"`
}

// FileAppendResponse 追加写入文件响应
type FileAppendResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	TotalLines   int    `json:"total_lines" jsonschema:"description=追加内容的行数"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// FileAppend 追加内容到文件末尾（文件不存在则创建）
func (ft *FileTools) FileAppend(ctx context.Context, req *FileAppendRequest) (*FileAppendResponse, error) {
	if req.Filepath == "" {
		return &FileAppendResponse{ErrorMessage: "filepath 参数是必需的"}, nil
	}

	rp, err := ft.resolvePath(ctx, req.Filepath)
	if err != nil {
		return nil, err
	}

	// 自动创建父目录
	dir := filepath.Dir(rp.path)
	if dir != "." {
		if err := rp.fs.MkdirAll(dir, 0755); err != nil {
			return &FileAppendResponse{ErrorMessage: fmt.Sprintf("创建目录失败: %v", err)}, nil
		}
	}

	// 使用 O_APPEND 直接追加，避免读全文件
	file, err := rp.fs.OpenFile(rp.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return &FileAppendResponse{ErrorMessage: fmt.Sprintf("打开文件失败: %v", err)}, nil
	}
	defer file.Close()

	if _, err := file.WriteString(req.Content); err != nil {
		return &FileAppendResponse{ErrorMessage: fmt.Sprintf("追加写入失败: %v", err)}, nil
	}

	addedLines := countLines(req.Content)
	return &FileAppendResponse{
		Message:    fmt.Sprintf("已追加到 %s (+%d 行)", req.Filepath, addedLines),
		TotalLines: addedLines,
	}, nil
}

// FileListRequest 列出目录请求
type FileListRequest struct {
	Dirpath string `json:"dirpath" jsonschema:"description=要列出的目录路径,required"`
}

// FileListResponse 列出目录响应
type FileListResponse struct {
	Content      string `json:"content" jsonschema:"description=目录内容列表"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// FileList 列出目录下的文件和文件夹
func (ft *FileTools) FileList(ctx context.Context, req *FileListRequest) (*FileListResponse, error) {
	if req.Dirpath == "" {
		return &FileListResponse{
			ErrorMessage: "dirpath 参数是必需的",
		}, nil
	}

	rp, err := ft.resolvePath(ctx, req.Dirpath)
	if err != nil {
		return nil, err
	}

	entries, err := afero.ReadDir(rp.fs, rp.path)
	if err != nil {
		return &FileListResponse{
			ErrorMessage: fmt.Sprintf("无法读取目录: %v", err),
		}, nil
	}

	var result []string
	result = append(result, fmt.Sprintf("目录 %s 下的文件和文件夹：\n", req.Dirpath))

	for _, entry := range entries {
		if entry.IsDir() {
			result = append(result, fmt.Sprintf("[DIR]  %s", entry.Name()))
		} else {
			result = append(result, fmt.Sprintf("[FILE] %s (%d bytes)", entry.Name(), entry.Size()))
		}
	}

	return &FileListResponse{
		Content: strings.Join(result, "\n"),
	}, nil
}

// GlobRequest 文件模式搜索请求
type GlobRequest struct {
	Pattern string `json:"pattern" jsonschema:"description=glob 模式，如 **/*.go 或 src/**/*.{js,ts},required"`
	Path    string `json:"path,omitempty" jsonschema:"description=搜索目录，默认为工作目录"`
}

// GlobResponse 文件模式搜索响应
type GlobResponse struct {
	Files        []string `json:"files,omitempty" jsonschema:"description=匹配的文件路径列表"`
	TotalFiles   int      `json:"total_files" jsonschema:"description=匹配文件总数"`
	Truncated    bool     `json:"truncated,omitempty" jsonschema:"description=结果是否被截断"`
	ErrorMessage string   `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// Glob 按 glob 模式搜索文件路径
func (ft *FileTools) Glob(ctx context.Context, req *GlobRequest) (*GlobResponse, error) {
	if req.Pattern == "" {
		return &GlobResponse{ErrorMessage: "pattern is required"}, nil
	}

	searchPath := req.Path
	if searchPath == "" {
		searchPath = "."
	}

	rp, err := ft.resolvePath(ctx, searchPath)
	if err != nil {
		return nil, err
	}

	info, err := rp.fs.Stat(rp.path)
	if err != nil {
		return &GlobResponse{ErrorMessage: fmt.Sprintf("路径不存在: %v", err)}, nil
	}
	if !info.IsDir() {
		return &GlobResponse{ErrorMessage: fmt.Sprintf("路径不是目录: %s", searchPath)}, nil
	}

	const maxFiles = 100
	var files []string
	truncated := false

	_ = afero.Walk(rp.fs, rp.path, func(path string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil || fi.IsDir() {
			return walkErr
		}
		if len(files) >= maxFiles {
			truncated = true
			return fmt.Errorf("max reached")
		}

		matchPath := filepath.ToSlash(path)
		matched, _ := doublestar.Match(req.Pattern, matchPath)
		if matched {
			files = append(files, path)
		}
		return nil
	})

	return &GlobResponse{
		Files:      files,
		TotalFiles: len(files),
		Truncated:  truncated,
	}, nil
}

// GrepRequest 搜索请求
type GrepRequest struct {
	Pattern  string `json:"pattern" jsonschema:"description=搜索模式,required"`
	Path     string `json:"path,omitempty" jsonschema:"description=搜索路径(文件或目录)，默认为工作目录"`
	Include  string `json:"include,omitempty" jsonschema:"description=文件名 glob 过滤，如 *.go 或 *.{js,ts}"`
	UseRegex bool   `json:"use_regex,omitempty" jsonschema:"description=是否启用正则表达式(默认false即纯文本匹配)"`
	Context  int    `json:"context,omitempty" jsonschema:"description=显示匹配行前后的上下文行数(默认0)"`
	MaxCount int    `json:"max_count,omitempty" jsonschema:"description=最大返回结果数(默认100)"`
}

// GrepMatch 搜索匹配结果
type GrepMatch struct {
	File       string `json:"file" jsonschema:"description=文件路径"`
	LineNumber int    `json:"line" jsonschema:"description=行号"`
	Text       string `json:"text" jsonschema:"description=匹配行内容"`
	Context    string `json:"context,omitempty" jsonschema:"description=前后上下文"`
}

// GrepResponse 搜索响应
type GrepResponse struct {
	Matches      []GrepMatch `json:"matches,omitempty" jsonschema:"description=匹配结果"`
	TotalMatches int         `json:"total_matches" jsonschema:"description=总匹配数"`
	ErrorMessage string      `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// Grep 在文件或目录中搜索文本
func (ft *FileTools) Grep(ctx context.Context, req *GrepRequest) (*GrepResponse, error) {
	if req.Pattern == "" {
		return &GrepResponse{ErrorMessage: "pattern is required"}, nil
	}

	// 确定搜索路径
	searchPath := req.Path
	if searchPath == "" {
		searchPath = "."
	}

	rp, err := ft.resolvePath(ctx, searchPath)
	if err != nil {
		return nil, err
	}

	// 编译匹配器
	var matcher func(string) bool
	if req.UseRegex {
		re, err := regexp.Compile(req.Pattern)
		if err != nil {
			return &GrepResponse{ErrorMessage: fmt.Sprintf("invalid regex: %v", err)}, nil
		}
		matcher = re.MatchString
	} else {
		matcher = func(line string) bool { return strings.Contains(line, req.Pattern) }
	}

	includePattern := req.Include
	maxCount := req.MaxCount
	if maxCount <= 0 {
		maxCount = 100
	}
	contextLines := req.Context
	if contextLines < 0 {
		contextLines = 0
	}

	var matches []GrepMatch

	// 检查搜索路径是文件还是目录
	info, err := rp.fs.Stat(rp.path)
	if err != nil {
		return &GrepResponse{ErrorMessage: fmt.Sprintf("路径不存在: %v", err)}, nil
	}

	if !info.IsDir() {
		// 单文件搜索
		fileMatches := ft.grepFile(rp.fs, rp.path, searchPath, matcher, contextLines, maxCount)
		matches = append(matches, fileMatches...)
	} else {
		// 目录递归搜索
		_ = afero.Walk(rp.fs, rp.path, func(path string, fi os.FileInfo, walkErr error) error {
			if walkErr != nil || fi.IsDir() {
				return walkErr
			}
			if len(matches) >= maxCount {
				return fmt.Errorf("max reached")
			}

			// glob 过滤
			if includePattern != "" {
				matched, _ := doublestar.Match(includePattern, fi.Name())
				if !matched {
					return nil
				}
			}

			// 跳过超大文件
			if fi.Size() > maxFileSize {
				return nil
			}

			remaining := maxCount - len(matches)
			fileMatches := ft.grepFile(rp.fs, path, path, matcher, contextLines, remaining)
			matches = append(matches, fileMatches...)
			return nil
		})
	}

	return &GrepResponse{
		Matches:      matches,
		TotalMatches: len(matches),
	}, nil
}

// grepFile 在单个文件中搜索，流式匹配不加载全文件
func (ft *FileTools) grepFile(fs afero.Fs, fsPath, displayPath string, matcher func(string) bool, contextLines, maxCount int) []GrepMatch {
	info, err := fs.Stat(fsPath)
	if err != nil || info.Size() > maxFileSize {
		return nil
	}
	file, err := fs.Open(fsPath)
	if err != nil {
		return nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(nil, maxScannerLineSize)

	var matches []GrepMatch
	var beforeCtx *ringBuffer
	if contextLines > 0 {
		beforeCtx = newRingBuffer(contextLines)
	}

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if !matcher(line) {
			if beforeCtx != nil {
				beforeCtx.add(lineNum, line)
			}
			continue
		}

		m := GrepMatch{
			File:       displayPath,
			LineNumber: lineNum,
			Text:       line,
		}

		if contextLines > 0 {
			m.Context = ft.buildGrepContext(scanner, beforeCtx, lineNum, contextLines)
		}

		matches = append(matches, m)
		if len(matches) >= maxCount {
			break
		}

		// 重置前置缓冲区
		if beforeCtx != nil {
			beforeCtx.add(lineNum, line)
		}
	}

	return matches
}

func (ft *FileTools) buildGrepContext(scanner *bufio.Scanner, beforeCtx *ringBuffer, matchLine, contextLines int) string {
	var ctxLines []string

	// 前置上下文
	if beforeCtx != nil {
		for _, entry := range beforeCtx.getAll() {
			ctxLines = append(ctxLines, fmt.Sprintf(" %d: %s", entry.line, entry.text))
		}
	}

	// 后置上下文：继续读 contextLines 行
	for i := 1; i <= contextLines; i++ {
		if !scanner.Scan() {
			break
		}
		ctxLines = append(ctxLines, fmt.Sprintf(" %d: %s", matchLine+i, scanner.Text()))
	}

	return strings.Join(ctxLines, "\n")
}

type ringEntry struct {
	line int
	text string
}

type ringBuffer struct {
	entries []ringEntry
	size    int
	pos     int
	full    bool
}

func newRingBuffer(size int) *ringBuffer {
	return &ringBuffer{entries: make([]ringEntry, size), size: size}
}

func (rb *ringBuffer) add(lineNum int, line string) {
	rb.entries[rb.pos] = ringEntry{line: lineNum, text: line}
	rb.pos = (rb.pos + 1) % rb.size
	if rb.pos == 0 {
		rb.full = true
	}
}

func (rb *ringBuffer) getAll() []ringEntry {
	if !rb.full {
		return rb.entries[:rb.pos]
	}
	result := make([]ringEntry, rb.size)
	copy(result, rb.entries[rb.pos:])
	copy(result[rb.size-rb.pos:], rb.entries[:rb.pos])
	return result
}

// FileEditRequest 精确查找替换请求
type FileEditRequest struct {
	Filepath  string `json:"filepath" jsonschema:"description=文件路径,required"`
	OldString string `json:"old_string" jsonschema:"description=要查找的原始文本(精确匹配,必须唯一)。应包含足够上下文确保唯一匹配,required"`
	NewString string `json:"new_string" jsonschema:"description=替换后的新文本。为空则删除匹配文本,required"`
}

// FileEditResponse 文件编辑响应
type FileEditResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	TotalLines   int    `json:"total_lines" jsonschema:"description=操作后文件总行数"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// FileEdit 精确查找并替换文件内容（old_string 必须唯一匹配）
func (ft *FileTools) FileEdit(ctx context.Context, req *FileEditRequest) (*FileEditResponse, error) {
	if req.Filepath == "" {
		return &FileEditResponse{ErrorMessage: "filepath is required"}, nil
	}
	if req.OldString == "" {
		return &FileEditResponse{ErrorMessage: "old_string is required"}, nil
	}

	rp, err := ft.resolvePath(ctx, req.Filepath)
	if err != nil {
		return nil, err
	}

	// 流式读取文件内容
	lines, totalLines, hasTrailing, err := readFileLines(rp.fs, rp.path)
	if err != nil {
		return &FileEditResponse{ErrorMessage: fmt.Sprintf("读取文件失败: %v", err)}, nil
	}

	original := joinLines(lines, hasTrailing)
	matchCount := strings.Count(original, req.OldString)
	if matchCount == 0 {
		return &FileEditResponse{
			ErrorMessage: fmt.Sprintf("未找到匹配文本。请先用 file_read 确认文件内容，确保 old_string 的缩进和空白完全一致 (文件共 %d 行)", totalLines),
			TotalLines:   totalLines,
		}, nil
	}
	if matchCount > 1 {
		return &FileEditResponse{
			ErrorMessage: fmt.Sprintf("找到 %d 处匹配，old_string 不唯一。请包含更多上下文行使其唯一 (文件共 %d 行)", matchCount, totalLines),
			TotalLines:   totalLines,
		}, nil
	}

	newContent := strings.Replace(original, req.OldString, req.NewString, 1)
	if err := afero.WriteFile(rp.fs, rp.path, []byte(newContent), 0644); err != nil {
		return &FileEditResponse{ErrorMessage: fmt.Sprintf("写入文件失败: %v", err)}, nil
	}

	return &FileEditResponse{
		Message:    fmt.Sprintf("已编辑 %s (%d 行)", req.Filepath, countLines(newContent)),
		TotalLines: countLines(newContent),
	}, nil
}

// FilePatchRequest 多文件 patch 请求
type FilePatchRequest struct {
	Patch string `json:"patch" jsonschema:"description=标准 unified diff 格式的 patch 内容。支持同时修改多个文件。格式示例:\n--- file.go\n+++ file.go\n@@ -1,3 +1,3 @@\n context\n-old line\n+new line\n context,required"`
}

// FilePatchResult 单个文件的 patch 结果
type FilePatchResult struct {
	Path    string `json:"path" jsonschema:"description=文件路径"`
	Success bool   `json:"success" jsonschema:"description=是否成功"`
	Error   string `json:"error,omitempty" jsonschema:"description=错误信息"`
	Warning string `json:"warning,omitempty" jsonschema:"description=警告信息(如部分hunk应用失败)"`
}

// FilePatchResponse 多文件 patch 响应
type FilePatchResponse struct {
	Message      string            `json:"message" jsonschema:"description=操作结果消息"`
	Results      []FilePatchResult `json:"results,omitempty" jsonschema:"description=每个文件的结果"`
	TotalFiles   int               `json:"total_files" jsonschema:"description=总文件数"`
	Succeeded    int               `json:"succeeded" jsonschema:"description=成功数"`
	Failed       int               `json:"failed" jsonschema:"description=失败数"`
	ErrorMessage string            `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// FilePatch 使用 unified diff 格式批量修改多个文件
func (ft *FileTools) FilePatch(ctx context.Context, req *FilePatchRequest) (*FilePatchResponse, error) {
	if req.Patch == "" {
		return &FilePatchResponse{
			ErrorMessage: "patch is required",
		}, nil
	}

	// 解析 patch
	mfd, err := mdiff.ParseMultiFileDiff(req.Patch)
	if err != nil {
		return &FilePatchResponse{
			ErrorMessage: fmt.Sprintf("failed to parse patch: %v", err),
		}, nil
	}

	if len(mfd.Files) == 0 {
		return &FilePatchResponse{
			ErrorMessage: "no file changes found in patch",
		}, nil
	}

	// 使用 FileAccessor 适配器
	accessor := &fsAccessor{ft: ft}
	result := mdiff.ApplyMultiFileDiff(mfd, accessor)

	// 转换结果
	resp := &FilePatchResponse{
		TotalFiles: result.TotalFiles,
		Succeeded:  result.Succeeded,
		Failed:     result.Failed,
	}

	hasWarning := false
	for _, r := range result.Results {
		fpr := FilePatchResult{
			Path:    r.Path,
			Success: r.Success,
			Error:   r.Error,
			Warning: r.Warning,
		}
		if r.Warning != "" {
			hasWarning = true
		}
		resp.Results = append(resp.Results, fpr)
	}

	switch {
	case result.Failed > 0:
		resp.Message = fmt.Sprintf("patch 完成: %d/%d 文件成功, %d 文件失败", result.Succeeded, result.TotalFiles, result.Failed)
	case hasWarning:
		resp.Message = fmt.Sprintf("patch 完成: %d 个文件已更新(部分 hunk 应用失败，请检查 warning)", result.Succeeded)
	default:
		resp.Message = fmt.Sprintf("patch 成功: %d 个文件已更新", result.Succeeded)
	}

	return resp, nil
}

// fsAccessor 适配 FileTools 到 mdiff.FileAccessor 接口
type fsAccessor struct {
	ft *FileTools
}

func (a *fsAccessor) ReadFile(path string) (string, error) {
	relPath, err := a.ft.workspacePath(path)
	if err != nil {
		return "", err
	}
	lines, _, hasTrailing, err := readFileLines(a.ft.securedFs, relPath)
	if err != nil {
		return "", err
	}
	return joinLines(lines, hasTrailing), nil
}

func (a *fsAccessor) WriteFile(path string, content string) error {
	relPath, err := a.ft.workspacePath(path)
	if err != nil {
		return err
	}
	// 自动创建父目录
	dir := filepath.Dir(relPath)
	if dir != "." {
		if err := a.ft.securedFs.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return afero.WriteFile(a.ft.securedFs, relPath, []byte(content), 0644)
}

func (a *fsAccessor) DeleteFile(path string) error {
	relPath, err := a.ft.workspacePath(path)
	if err != nil {
		return err
	}
	return a.ft.securedFs.Remove(relPath)
}
