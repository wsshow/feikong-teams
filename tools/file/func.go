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
	// 转换为绝对路径
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

	// 使用 BasePathFs 限制文件系统访问范围
	// 所有相对路径操作都会被限制在这个目录下
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

// readFileLinesFrom 读取文件全部内容并按行分割，同时记录是否有尾部换行
func readFileLinesFrom(fs afero.Fs, path string) (lines []string, hasTrailingNewline bool, err error) {
	content, err := afero.ReadFile(fs, path)
	if err != nil {
		return nil, false, err
	}
	text := string(content)
	hasTrailingNewline = len(text) > 0 && text[len(text)-1] == '\n'
	// 移除尾部换行后分割，避免产生多余的空行
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return []string{}, hasTrailingNewline, nil
	}
	lines = strings.Split(text, "\n")
	return lines, hasTrailingNewline, nil
}

// readFileLines 使用工作目录文件系统读取文件（兼容 fsAccessor）
func (ft *FileTools) readFileLines(relPath string) ([]string, bool, error) {
	return readFileLinesFrom(ft.securedFs, relPath)
}

// joinLines 将行数组拼接为文件内容，根据原始文件情况保留尾部换行
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

// workspacePath 验证并规范化路径（仅限工作目录内）
// 确保路径在允许的目录范围内，并返回相对路径
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
const maxDefaultLines = 200

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
		return &FileReadResponse{
			ErrorMessage: "filepath 参数是必需的",
		}, nil
	}

	rp, err := ft.resolvePath(ctx, req.Filepath)
	if err != nil {
		return nil, err
	}

	// 读取文件所有行
	lines, _, err := readFileLinesFrom(rp.fs, rp.path)
	if err != nil {
		return &FileReadResponse{
			ErrorMessage: fmt.Sprintf("读取文件失败: %v", err),
		}, nil
	}

	totalLines := len(lines)

	// 如果没有指定行范围
	if req.StartLine == 0 && req.EndLine == 0 {
		// 超过限制时截断，只返回前 N 行
		if totalLines > maxDefaultLines {
			return &FileReadResponse{
				Content:    strings.Join(lines[:maxDefaultLines], "\n"),
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

	// 指定行范围的部分读取
	startIdx := req.StartLine - 1
	if startIdx < 0 {
		startIdx = 0
	}
	if startIdx >= totalLines {
		return &FileReadResponse{
			ErrorMessage: fmt.Sprintf("起始行号 %d 超出文件总行数 %d", req.StartLine, totalLines),
			TotalLines:   totalLines,
		}, nil
	}

	endIdx := totalLines
	if req.EndLine > 0 && req.EndLine < totalLines {
		endIdx = req.EndLine
	}

	return &FileReadResponse{
		Content:    strings.Join(lines[startIdx:endIdx], "\n"),
		TotalLines: totalLines,
		ReadRange:  fmt.Sprintf("%d-%d", startIdx+1, endIdx),
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
		Message:    fmt.Sprintf("成功写入 %d 字节到文件 %s", len(req.Content), req.Filepath),
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
	TotalLines   int    `json:"total_lines" jsonschema:"description=追加后文件总行数"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// FileAppend 追加内容到文件末尾（文件不存在则创建）
func (ft *FileTools) FileAppend(ctx context.Context, req *FileAppendRequest) (*FileAppendResponse, error) {
	if req.Filepath == "" {
		return &FileAppendResponse{
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
			return &FileAppendResponse{
				ErrorMessage: fmt.Sprintf("创建目录失败: %v", err),
			}, nil
		}
	}

	// 读取已有内容
	existing, _ := afero.ReadFile(rp.fs, rp.path)

	// 追加新内容
	newContent := string(existing) + req.Content
	err = afero.WriteFile(rp.fs, rp.path, []byte(newContent), 0644)
	if err != nil {
		return &FileAppendResponse{
			ErrorMessage: fmt.Sprintf("追加写入文件失败: %v", err),
		}, nil
	}

	totalLines := countLines(newContent)

	return &FileAppendResponse{
		Message:    fmt.Sprintf("成功追加 %d 字节到文件 %s", len(req.Content), req.Filepath),
		TotalLines: totalLines,
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

	// 编译 glob 过滤器
	var includePattern string
	if req.Include != "" {
		includePattern = req.Include
	}

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
				matched, _ := filepath.Match(includePattern, fi.Name())
				if !matched {
					return nil
				}
			}

			// 跳过二进制文件（简单检测：大于 1MB 或扩展名为常见二进制）
			if fi.Size() > 1<<20 {
				return nil
			}

			// 构建显示路径
			displayPath := path
			if rp.path == "." {
				displayPath = path
			}

			remaining := maxCount - len(matches)
			fileMatches := ft.grepFile(rp.fs, path, displayPath, matcher, contextLines, remaining)
			matches = append(matches, fileMatches...)
			return nil
		})
	}

	return &GrepResponse{
		Matches:      matches,
		TotalMatches: len(matches),
	}, nil
}

// grepFile 在单个文件中搜索
func (ft *FileTools) grepFile(fs afero.Fs, fsPath, displayPath string, matcher func(string) bool, contextLines, maxCount int) []GrepMatch {
	file, err := fs.Open(fsPath)
	if err != nil {
		return nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var allLines []string
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}
	if scanner.Err() != nil {
		return nil
	}

	var matches []GrepMatch
	for i, line := range allLines {
		if len(matches) >= maxCount {
			break
		}
		if !matcher(line) {
			continue
		}

		m := GrepMatch{
			File:       displayPath,
			LineNumber: i + 1,
			Text:       line,
		}

		// 添加上下文
		if contextLines > 0 {
			start := i - contextLines
			if start < 0 {
				start = 0
			}
			end := i + contextLines + 1
			if end > len(allLines) {
				end = len(allLines)
			}
			var ctxLines []string
			for j := start; j < end; j++ {
				prefix := " "
				if j == i {
					prefix = ">"
				}
				ctxLines = append(ctxLines, fmt.Sprintf("%s%d: %s", prefix, j+1, allLines[j]))
			}
			m.Context = strings.Join(ctxLines, "\n")
		}

		matches = append(matches, m)
	}
	return matches
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

	content, err := afero.ReadFile(rp.fs, rp.path)
	if err != nil {
		return &FileEditResponse{ErrorMessage: fmt.Sprintf("读取文件失败: %v", err)}, nil
	}

	original := string(content)
	matchCount := strings.Count(original, req.OldString)
	if matchCount == 0 {
		return &FileEditResponse{
			ErrorMessage: fmt.Sprintf("未找到匹配文本，请检查 old_string 是否正确 (文件共 %d 行)", countLines(original)),
			TotalLines:   countLines(original),
		}, nil
	}
	if matchCount > 1 {
		return &FileEditResponse{
			ErrorMessage: fmt.Sprintf("找到 %d 处匹配，请包含更多上下文确保唯一匹配 (文件共 %d 行)", matchCount, countLines(original)),
			TotalLines:   countLines(original),
		}, nil
	}

	newContent := strings.Replace(original, req.OldString, req.NewString, 1)
	if err := afero.WriteFile(rp.fs, rp.path, []byte(newContent), 0644); err != nil {
		return &FileEditResponse{ErrorMessage: fmt.Sprintf("写入文件失败: %v", err)}, nil
	}

	return &FileEditResponse{
		Message:    fmt.Sprintf("成功编辑文件 %s", req.Filepath),
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
	content, err := afero.ReadFile(a.ft.securedFs, relPath)
	if err != nil {
		return "", err
	}
	return string(content), nil
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
