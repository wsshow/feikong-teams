package file

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"fkteams/mdiff"

	"github.com/spf13/afero"
)

// FileTools 文件工具实例，每个agent可以有独立的实例
type FileTools struct {
	// securedFs 是受限制的文件系统，只允许访问指定目录
	securedFs afero.Fs
	// allowedBaseDir 是允许访问的基础目录
	allowedBaseDir string
}

// NewFileTools 创建一个新的文件工具实例
// baseDir 是允许操作的基础目录（通常是 ./workspace 目录）
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
		allowedBaseDir: absPath,
	}, nil
}

// readFileLines 读取文件全部内容并按行分割，同时记录是否有尾部换行
func (ft *FileTools) readFileLines(relPath string) (lines []string, hasTrailingNewline bool, err error) {
	content, err := afero.ReadFile(ft.securedFs, relPath)
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

// validatePath 验证并规范化路径
// 确保路径在允许的目录范围内，并返回相对路径
func (ft *FileTools) validatePath(userPath string) (string, error) {
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

	// 验证并规范化路径
	relPath, err := ft.validatePath(req.Filepath)
	if err != nil {
		return &FileReadResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	// 读取文件所有行
	lines, _, err := ft.readFileLines(relPath)
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

	// 验证并规范化路径
	relPath, err := ft.validatePath(req.Filepath)
	if err != nil {
		return &FileWriteResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	// 自动创建父目录
	dir := filepath.Dir(relPath)
	if dir != "." {
		if err := ft.securedFs.MkdirAll(dir, 0755); err != nil {
			return &FileWriteResponse{
				ErrorMessage: fmt.Sprintf("创建目录失败: %v", err),
			}, nil
		}
	}

	err = afero.WriteFile(ft.securedFs, relPath, []byte(req.Content), 0644)
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

// FileAppendRequest 追加文件请求
type FileAppendRequest struct {
	Filepath string `json:"filepath" jsonschema:"description=要追加的文件路径,required"`
	Content  string `json:"content" jsonschema:"description=要追加的内容,required"`
}

// FileAppendResponse 追加文件响应
type FileAppendResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	TotalLines   int    `json:"total_lines" jsonschema:"description=追加后文件总行数"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// FileAppend 在文件末尾追加内容
func (ft *FileTools) FileAppend(ctx context.Context, req *FileAppendRequest) (*FileAppendResponse, error) {
	if req.Filepath == "" {
		return &FileAppendResponse{
			ErrorMessage: "filepath 参数是必需的",
		}, nil
	}

	// 验证并规范化路径
	relPath, err := ft.validatePath(req.Filepath)
	if err != nil {
		return &FileAppendResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	file, err := ft.securedFs.OpenFile(relPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return &FileAppendResponse{
			ErrorMessage: fmt.Sprintf("无法打开文件: %v", err),
		}, nil
	}

	n, err := file.WriteString(req.Content)
	file.Close()
	if err != nil {
		return &FileAppendResponse{
			ErrorMessage: fmt.Sprintf("追加内容失败: %v", err),
		}, nil
	}

	// 读取追加后的总行数
	lines, _, _ := ft.readFileLines(relPath)

	return &FileAppendResponse{
		Message:    fmt.Sprintf("成功追加 %d 字节到文件 %s", n, req.Filepath),
		TotalLines: len(lines),
	}, nil
}

// FileModifyRequest 修改文件请求
type FileModifyRequest struct {
	Filepath   string `json:"filepath" jsonschema:"description=要修改的文件路径,required"`
	StartLine  int    `json:"start_line" jsonschema:"description=起始行号(从1开始),required"`
	EndLine    int    `json:"end_line" jsonschema:"description=结束行号,required"`
	NewContent string `json:"new_content" jsonschema:"description=新的内容(将替换指定行范围),required"`
}

// FileModifyResponse 修改文件响应
type FileModifyResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	TotalLines   int    `json:"total_lines" jsonschema:"description=修改后文件总行数"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// FileModify 修改文件中指定行范围的内容
func (ft *FileTools) FileModify(ctx context.Context, req *FileModifyRequest) (*FileModifyResponse, error) {
	if req.Filepath == "" {
		return &FileModifyResponse{
			ErrorMessage: "filepath 参数是必需的",
		}, nil
	}

	if req.StartLine < 1 || req.EndLine < req.StartLine {
		return &FileModifyResponse{
			ErrorMessage: fmt.Sprintf("行号无效: start_line=%d, end_line=%d", req.StartLine, req.EndLine),
		}, nil
	}

	// 验证并规范化路径
	relPath, err := ft.validatePath(req.Filepath)
	if err != nil {
		return &FileModifyResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	// 读取原文件
	lines, hasTrailingNewline, err := ft.readFileLines(relPath)
	if err != nil {
		return &FileModifyResponse{
			ErrorMessage: fmt.Sprintf("读取文件失败: %v", err),
		}, nil
	}

	if req.StartLine > len(lines) {
		return &FileModifyResponse{
			ErrorMessage: fmt.Sprintf("起始行号 %d 超出文件总行数 %d", req.StartLine, len(lines)),
		}, nil
	}

	// 构建新内容：将 NewContent 按行分割后插入
	newLines := strings.Split(req.NewContent, "\n")
	var result []string
	result = append(result, lines[:req.StartLine-1]...)
	result = append(result, newLines...)
	if req.EndLine < len(lines) {
		result = append(result, lines[req.EndLine:]...)
	}

	// 写回文件，保留原始尾部换行符
	err = afero.WriteFile(ft.securedFs, relPath, []byte(joinLines(result, hasTrailingNewline)), 0644)
	if err != nil {
		return &FileModifyResponse{
			ErrorMessage: fmt.Sprintf("写入文件失败: %v", err),
		}, nil
	}

	oldLineCount := req.EndLine - req.StartLine + 1
	return &FileModifyResponse{
		Message:    fmt.Sprintf("成功修改文件 %s 的第 %d-%d 行（替换了 %d 行为 %d 行）", req.Filepath, req.StartLine, req.EndLine, oldLineCount, len(newLines)),
		TotalLines: len(result),
	}, nil
}

// FileInsertRequest 插入文件请求
type FileInsertRequest struct {
	Filepath   string `json:"filepath" jsonschema:"description=要修改的文件路径,required"`
	LineNumber int    `json:"line_number" jsonschema:"description=在该行之后插入内容(从1开始),0表示插入到文件开头,required"`
	Content    string `json:"content" jsonschema:"description=要插入的内容,required"`
}

// FileInsertResponse 插入文件响应
type FileInsertResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	TotalLines   int    `json:"total_lines" jsonschema:"description=插入后文件总行数"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// FileInsert 在文件的指定行之后插入新内容
func (ft *FileTools) FileInsert(ctx context.Context, req *FileInsertRequest) (*FileInsertResponse, error) {
	if req.Filepath == "" {
		return &FileInsertResponse{
			ErrorMessage: "filepath 参数是必需的",
		}, nil
	}

	if req.LineNumber < 0 {
		return &FileInsertResponse{
			ErrorMessage: fmt.Sprintf("行号无效: line_number=%d", req.LineNumber),
		}, nil
	}

	// 验证并规范化路径
	relPath, err := ft.validatePath(req.Filepath)
	if err != nil {
		return &FileInsertResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	// 读取原文件
	lines, hasTrailingNewline, err := ft.readFileLines(relPath)
	if err != nil {
		return &FileInsertResponse{
			ErrorMessage: fmt.Sprintf("读取文件失败: %v", err),
		}, nil
	}

	if req.LineNumber > len(lines) {
		return &FileInsertResponse{
			ErrorMessage: fmt.Sprintf("行号 %d 超出文件总行数 %d", req.LineNumber, len(lines)),
		}, nil
	}

	// 将插入内容按行分割
	newLines := strings.Split(req.Content, "\n")
	var result []string
	if req.LineNumber == 0 {
		result = append(result, newLines...)
		result = append(result, lines...)
	} else {
		result = append(result, lines[:req.LineNumber]...)
		result = append(result, newLines...)
		result = append(result, lines[req.LineNumber:]...)
	}

	// 写回文件，保留原始尾部换行符
	err = afero.WriteFile(ft.securedFs, relPath, []byte(joinLines(result, hasTrailingNewline)), 0644)
	if err != nil {
		return &FileInsertResponse{
			ErrorMessage: fmt.Sprintf("写入文件失败: %v", err),
		}, nil
	}

	position := "开头"
	if req.LineNumber > 0 {
		position = "第 " + strconv.Itoa(req.LineNumber) + " 行之后"
	}
	return &FileInsertResponse{
		Message:    fmt.Sprintf("成功在文件 %s 的%s插入 %d 行内容", req.Filepath, position, len(newLines)),
		TotalLines: len(result),
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

	// 验证并规范化路径
	relPath, err := ft.validatePath(req.Dirpath)
	if err != nil {
		return &FileListResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	entries, err := afero.ReadDir(ft.securedFs, relPath)
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

// FileCreateRequest 创建文件请求
type FileCreateRequest struct {
	Filepath string `json:"filepath" jsonschema:"description=要创建的文件路径,required"`
}

// FileCreateResponse 创建文件响应
type FileCreateResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// FileCreate 创建一个新的空文件
func (ft *FileTools) FileCreate(ctx context.Context, req *FileCreateRequest) (*FileCreateResponse, error) {
	if req.Filepath == "" {
		return &FileCreateResponse{
			ErrorMessage: "filepath 参数是必需的",
		}, nil
	}

	// 验证并规范化路径
	relPath, err := ft.validatePath(req.Filepath)
	if err != nil {
		return &FileCreateResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	// 创建文件（如果已存在则清空）
	file, err := ft.securedFs.Create(relPath)
	if err != nil {
		return &FileCreateResponse{
			ErrorMessage: fmt.Sprintf("创建文件失败: %v", err),
		}, nil
	}
	defer file.Close()

	return &FileCreateResponse{
		Message: fmt.Sprintf("成功创建空文件 %s", req.Filepath),
	}, nil
}

// FileDeleteRequest 删除文件请求
type FileDeleteRequest struct {
	Filepath string `json:"filepath" jsonschema:"description=要删除的文件路径,required"`
}

// FileDeleteResponse 删除文件响应
type FileDeleteResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// FileDelete 删除指定的文件
func (ft *FileTools) FileDelete(ctx context.Context, req *FileDeleteRequest) (*FileDeleteResponse, error) {
	if req.Filepath == "" {
		return &FileDeleteResponse{
			ErrorMessage: "filepath 参数是必需的",
		}, nil
	}

	// 验证并规范化路径
	relPath, err := ft.validatePath(req.Filepath)
	if err != nil {
		return &FileDeleteResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	err = ft.securedFs.Remove(relPath)
	if err != nil {
		return &FileDeleteResponse{
			ErrorMessage: fmt.Sprintf("删除文件失败: %v", err),
		}, nil
	}

	return &FileDeleteResponse{
		Message: fmt.Sprintf("成功删除文件 %s", req.Filepath),
	}, nil
}

// DirCreateRequest 创建目录请求
type DirCreateRequest struct {
	Dirpath string `json:"dirpath" jsonschema:"description=要创建的目录路径,required"`
}

// DirCreateResponse 创建目录响应
type DirCreateResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// DirCreate 创建一个新的目录（支持递归创建）
func (ft *FileTools) DirCreate(ctx context.Context, req *DirCreateRequest) (*DirCreateResponse, error) {
	if req.Dirpath == "" {
		return &DirCreateResponse{
			ErrorMessage: "dirpath 参数是必需的",
		}, nil
	}

	// 验证并规范化路径
	relPath, err := ft.validatePath(req.Dirpath)
	if err != nil {
		return &DirCreateResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	err = ft.securedFs.MkdirAll(relPath, 0755)
	if err != nil {
		return &DirCreateResponse{
			ErrorMessage: fmt.Sprintf("创建目录失败: %v", err),
		}, nil
	}

	return &DirCreateResponse{
		Message: fmt.Sprintf("成功创建目录 %s", req.Dirpath),
	}, nil
}

// DirDeleteRequest 删除目录请求
type DirDeleteRequest struct {
	Dirpath string `json:"dirpath" jsonschema:"description=要删除的目录路径,required"`
}

// DirDeleteResponse 删除目录响应
type DirDeleteResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// DirDelete 删除指定的目录（仅限空目录）
func (ft *FileTools) DirDelete(ctx context.Context, req *DirDeleteRequest) (*DirDeleteResponse, error) {
	if req.Dirpath == "" {
		return &DirDeleteResponse{
			ErrorMessage: "dirpath 参数是必需的",
		}, nil
	}

	// 验证并规范化路径
	relPath, err := ft.validatePath(req.Dirpath)
	if err != nil {
		return &DirDeleteResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	err = ft.securedFs.Remove(relPath)
	if err != nil {
		return &DirDeleteResponse{
			ErrorMessage: fmt.Sprintf("删除目录失败: %v", err),
		}, nil
	}

	return &DirDeleteResponse{
		Message: fmt.Sprintf("成功删除目录 %s", req.Dirpath),
	}, nil
}

// FileSearchRequest 搜索文件内容请求
type FileSearchRequest struct {
	Filepath string `json:"filepath" jsonschema:"description=要搜索的文件路径,required"`
	Pattern  string `json:"pattern" jsonschema:"description=搜索文本,required"`
	UseRegex bool   `json:"use_regex,omitempty" jsonschema:"description=是否启用正则表达式匹配(默认false即纯文本匹配)"`
	MaxCount int    `json:"max_count,omitempty" jsonschema:"description=最大返回结果数,默认100"`
}

// SearchMatch 搜索匹配结果
type SearchMatch struct {
	LineNumber int    `json:"line_number" jsonschema:"description=行号(从1开始)"`
	LineText   string `json:"line_text" jsonschema:"description=匹配的行内容"`
}

// FileSearchResponse 搜索文件内容响应
type FileSearchResponse struct {
	Matches      []SearchMatch `json:"matches,omitempty" jsonschema:"description=匹配结果列表"`
	TotalMatches int           `json:"total_matches" jsonschema:"description=总匹配数"`
	ErrorMessage string        `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// FileSearch 在文件中搜索指定模式
func (ft *FileTools) FileSearch(ctx context.Context, req *FileSearchRequest) (*FileSearchResponse, error) {
	if req.Filepath == "" {
		return &FileSearchResponse{
			ErrorMessage: "filepath 参数是必需的",
		}, nil
	}

	if req.Pattern == "" {
		return &FileSearchResponse{
			ErrorMessage: "pattern 参数是必需的",
		}, nil
	}

	// 验证并规范化路径
	relPath, err := ft.validatePath(req.Filepath)
	if err != nil {
		return &FileSearchResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	// 根据 use_regex 参数决定匹配方式
	var regex *regexp.Regexp
	if req.UseRegex {
		var err error
		regex, err = regexp.Compile(req.Pattern)
		if err != nil {
			return &FileSearchResponse{
				ErrorMessage: fmt.Sprintf("invalid regex pattern: %v", err),
			}, nil
		}
	}

	// 读取文件
	file, err := ft.securedFs.Open(relPath)
	if err != nil {
		return &FileSearchResponse{
			ErrorMessage: fmt.Sprintf("无法打开文件: %v", err),
		}, nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var matches []SearchMatch
	lineNumber := 1
	maxCount := req.MaxCount
	if maxCount <= 0 {
		maxCount = 100
	}

	for scanner.Scan() && len(matches) < maxCount {
		line := scanner.Text()
		matched := false

		if regex != nil {
			matched = regex.MatchString(line)
		} else {
			matched = strings.Contains(line, req.Pattern)
		}

		if matched {
			matches = append(matches, SearchMatch{
				LineNumber: lineNumber,
				LineText:   line,
			})
		}
		lineNumber++
	}

	if err := scanner.Err(); err != nil {
		return &FileSearchResponse{
			ErrorMessage: fmt.Sprintf("读取文件时出错: %v", err),
		}, nil
	}

	return &FileSearchResponse{
		Matches:      matches,
		TotalMatches: len(matches),
	}, nil
}

// FileReplaceRequest 替换文件内容请求
type FileReplaceRequest struct {
	Filepath   string `json:"filepath" jsonschema:"description=要修改的文件路径,required"`
	OldPattern string `json:"old_pattern" jsonschema:"description=要替换的文本(精确匹配),required"`
	NewText    string `json:"new_text" jsonschema:"description=替换后的文本,required"`
	UseRegex   bool   `json:"use_regex,omitempty" jsonschema:"description=是否启用正则表达式匹配(默认false即纯文本精确匹配)"`
	MaxCount   int    `json:"max_count,omitempty" jsonschema:"description=最大替换次数,0表示替换所有,默认0"`
}

// FileReplaceResponse 替换文件内容响应
type FileReplaceResponse struct {
	Message       string `json:"message" jsonschema:"description=操作结果消息"`
	ReplacedCount int    `json:"replaced_count" jsonschema:"description=实际替换次数"`
	TotalLines    int    `json:"total_lines" jsonschema:"description=替换后文件总行数"`
	ErrorMessage  string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// FileReplace 在文件中替换指定模式
func (ft *FileTools) FileReplace(ctx context.Context, req *FileReplaceRequest) (*FileReplaceResponse, error) {
	if req.Filepath == "" {
		return &FileReplaceResponse{
			ErrorMessage: "filepath 参数是必需的",
		}, nil
	}

	if req.OldPattern == "" {
		return &FileReplaceResponse{
			ErrorMessage: "old_pattern 参数是必需的",
		}, nil
	}

	// 验证并规范化路径
	relPath, err := ft.validatePath(req.Filepath)
	if err != nil {
		return &FileReplaceResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	// 读取文件内容
	content, err := afero.ReadFile(ft.securedFs, relPath)
	if err != nil {
		return &FileReplaceResponse{
			ErrorMessage: fmt.Sprintf("读取文件失败: %v", err),
		}, nil
	}

	// 执行替换
	originalContent := string(content)
	newContent := originalContent
	replacedCount := 0

	if req.UseRegex {
		// 正则替换模式
		regex, err := regexp.Compile(req.OldPattern)
		if err != nil {
			return &FileReplaceResponse{
				ErrorMessage: fmt.Sprintf("invalid regex pattern: %v", err),
			}, nil
		}
		if req.MaxCount > 0 {
			count := 0
			newContent = regex.ReplaceAllStringFunc(originalContent, func(match string) string {
				if count < req.MaxCount {
					count++
					return req.NewText
				}
				return match
			})
			replacedCount = count
		} else {
			matches := regex.FindAllStringIndex(originalContent, -1)
			replacedCount = len(matches)
			newContent = regex.ReplaceAllString(originalContent, req.NewText)
		}
	} else {
		// 默认纯文本精确匹配
		if req.MaxCount > 0 {
			newContent = strings.Replace(originalContent, req.OldPattern, req.NewText, req.MaxCount)
			replacedCount = strings.Count(originalContent, req.OldPattern) - strings.Count(newContent, req.OldPattern)
		} else {
			replacedCount = strings.Count(originalContent, req.OldPattern)
			newContent = strings.ReplaceAll(originalContent, req.OldPattern, req.NewText)
		}
	}

	// 如果没有任何替换,返回提示
	if newContent == originalContent {
		return &FileReplaceResponse{
			Message:       "未找到匹配的内容,文件未修改",
			ReplacedCount: 0,
			TotalLines:    countLines(originalContent),
		}, nil
	}

	// 写回文件
	err = afero.WriteFile(ft.securedFs, relPath, []byte(newContent), 0644)
	if err != nil {
		return &FileReplaceResponse{
			ErrorMessage: fmt.Sprintf("写入文件失败: %v", err),
		}, nil
	}

	return &FileReplaceResponse{
		Message:       fmt.Sprintf("成功在文件 %s 中替换了 %d 处内容", req.Filepath, replacedCount),
		ReplacedCount: replacedCount,
		TotalLines:    countLines(newContent),
	}, nil
}

// FileEditRequest 统一文件编辑请求
type FileEditRequest struct {
	Filepath string `json:"filepath" jsonschema:"description=文件路径,required"`
	Action   string `json:"action" jsonschema:"description=操作类型: write(创建或覆盖整个文件) | append(追加内容到文件末尾) | replace(精确查找并替换文本),required"`
	Content  string `json:"content" jsonschema:"description=新内容。write模式为整个文件内容;append模式为追加的内容;replace模式为替换后的新文本,required"`
	OldText  string `json:"old_text,omitempty" jsonschema:"description=replace模式下要查找的原始文本(精确匹配)。应包含足够上下文确保唯一匹配"`
}

// FileEditResponse 统一文件编辑响应
type FileEditResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	TotalLines   int    `json:"total_lines" jsonschema:"description=操作后文件总行数"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// FileEdit 统一文件编辑工具，支持 write/append/replace 三种模式
func (ft *FileTools) FileEdit(ctx context.Context, req *FileEditRequest) (*FileEditResponse, error) {
	if req.Filepath == "" {
		return &FileEditResponse{
			ErrorMessage: "filepath is required",
		}, nil
	}

	relPath, err := ft.validatePath(req.Filepath)
	if err != nil {
		return &FileEditResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	switch req.Action {
	case "write":
		// 自动创建父目录
		dir := filepath.Dir(relPath)
		if dir != "." {
			if err := ft.securedFs.MkdirAll(dir, 0755); err != nil {
				return &FileEditResponse{
					ErrorMessage: fmt.Sprintf("failed to create directory: %v", err),
				}, nil
			}
		}
		if err := afero.WriteFile(ft.securedFs, relPath, []byte(req.Content), 0644); err != nil {
			return &FileEditResponse{
				ErrorMessage: fmt.Sprintf("failed to write file: %v", err),
			}, nil
		}
		return &FileEditResponse{
			Message:    fmt.Sprintf("成功写入文件 %s (%d 字节)", req.Filepath, len(req.Content)),
			TotalLines: countLines(req.Content),
		}, nil

	case "append":
		file, err := ft.securedFs.OpenFile(relPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return &FileEditResponse{
				ErrorMessage: fmt.Sprintf("failed to open file: %v", err),
			}, nil
		}
		_, err = file.WriteString(req.Content)
		file.Close()
		if err != nil {
			return &FileEditResponse{
				ErrorMessage: fmt.Sprintf("failed to append: %v", err),
			}, nil
		}
		lines, _, _ := ft.readFileLines(relPath)
		return &FileEditResponse{
			Message:    fmt.Sprintf("成功追加内容到文件 %s", req.Filepath),
			TotalLines: len(lines),
		}, nil

	case "replace":
		if req.OldText == "" {
			return &FileEditResponse{
				ErrorMessage: "replace mode requires old_text parameter",
			}, nil
		}
		content, err := afero.ReadFile(ft.securedFs, relPath)
		if err != nil {
			return &FileEditResponse{
				ErrorMessage: fmt.Sprintf("failed to read file: %v", err),
			}, nil
		}
		original := string(content)
		matchCount := strings.Count(original, req.OldText)
		if matchCount == 0 {
			return &FileEditResponse{
				ErrorMessage: fmt.Sprintf("未找到匹配的文本，请检查 old_text 是否完全正确 (文件共 %d 行)", countLines(original)),
				TotalLines:   countLines(original),
			}, nil
		}
		if matchCount > 1 {
			return &FileEditResponse{
				ErrorMessage: fmt.Sprintf("找到 %d 处匹配，请在 old_text 中包含更多上下文以确保唯一匹配 (文件共 %d 行)", matchCount, countLines(original)),
				TotalLines:   countLines(original),
			}, nil
		}
		// 精确替换唯一匹配
		newContent := strings.Replace(original, req.OldText, req.Content, 1)
		if err := afero.WriteFile(ft.securedFs, relPath, []byte(newContent), 0644); err != nil {
			return &FileEditResponse{
				ErrorMessage: fmt.Sprintf("failed to write file: %v", err),
			}, nil
		}
		return &FileEditResponse{
			Message:    fmt.Sprintf("成功替换文件 %s 中的内容", req.Filepath),
			TotalLines: countLines(newContent),
		}, nil

	default:
		return &FileEditResponse{
			ErrorMessage: fmt.Sprintf("unsupported action: %s, use write/append/replace", req.Action),
		}, nil
	}
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

// FileDiffRequest 计算文件 diff 请求
type FileDiffRequest struct {
	Filepath   string `json:"filepath" jsonschema:"description=文件路径,required"`
	NewContent string `json:"new_content" jsonschema:"description=新的文件内容,required"`
}

// FileDiffResponse 文件 diff 响应
type FileDiffResponse struct {
	Diff         string `json:"diff" jsonschema:"description=unified diff 格式的差异内容"`
	Insertions   int    `json:"insertions" jsonschema:"description=新增行数"`
	Deletions    int    `json:"deletions" jsonschema:"description=删除行数"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// FileDiff 计算文件与新内容之间的 diff
func (ft *FileTools) FileDiff(ctx context.Context, req *FileDiffRequest) (*FileDiffResponse, error) {
	if req.Filepath == "" {
		return &FileDiffResponse{
			ErrorMessage: "filepath is required",
		}, nil
	}

	relPath, err := ft.validatePath(req.Filepath)
	if err != nil {
		return &FileDiffResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	// 读取原始文件内容（如果文件不存在，视为空文件）
	oldContent := ""
	content, err := afero.ReadFile(ft.securedFs, relPath)
	if err == nil {
		oldContent = string(content)
	}

	fd := mdiff.DiffFiles(req.Filepath, oldContent, req.Filepath, req.NewContent, 3)
	diffText := mdiff.FormatFileDiff(fd)

	stat := mdiff.Stat(&mdiff.MultiFileDiff{Files: []mdiff.FileDiff{*fd}})

	return &FileDiffResponse{
		Diff:       diffText,
		Insertions: stat.Insertions,
		Deletions:  stat.Deletions,
	}, nil
}

// fsAccessor 适配 FileTools 到 mdiff.FileAccessor 接口
type fsAccessor struct {
	ft *FileTools
}

func (a *fsAccessor) ReadFile(path string) (string, error) {
	relPath, err := a.ft.validatePath(path)
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
	relPath, err := a.ft.validatePath(path)
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
	relPath, err := a.ft.validatePath(path)
	if err != nil {
		return err
	}
	return a.ft.securedFs.Remove(relPath)
}
