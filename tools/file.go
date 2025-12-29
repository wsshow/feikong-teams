package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// FileReadRequest 读取文件请求
type FileReadRequest struct {
	Filepath  string `json:"filepath" jsonschema:"description=要读取的文件路径,required"`
	StartLine int    `json:"start_line,omitempty" jsonschema:"description=起始行号(从1开始),不填则从第一行开始"`
	EndLine   int    `json:"end_line,omitempty" jsonschema:"description=结束行号,不填则读到文件末尾"`
}

// FileReadResponse 读取文件响应
type FileReadResponse struct {
	Content      string `json:"content" jsonschema:"description=文件内容"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// FileRead 读取文件内容
func FileRead(ctx context.Context, req *FileReadRequest) (*FileReadResponse, error) {
	if req.Filepath == "" {
		return &FileReadResponse{
			ErrorMessage: "filepath 参数是必需的",
		}, nil
	}

	file, err := os.Open(req.Filepath)
	if err != nil {
		return &FileReadResponse{
			ErrorMessage: fmt.Sprintf("无法打开文件: %v", err),
		}, nil
	}
	defer file.Close()

	// 如果没有指定行范围，读取全部内容
	if req.StartLine == 0 && req.EndLine == 0 {
		content, err := os.ReadFile(req.Filepath)
		if err != nil {
			return &FileReadResponse{
				ErrorMessage: fmt.Sprintf("读取文件失败: %v", err),
			}, nil
		}
		return &FileReadResponse{
			Content: string(content),
		}, nil
	}

	// 部分读取
	scanner := bufio.NewScanner(file)
	var lines []string
	currentLine := 1

	for scanner.Scan() {
		if req.StartLine > 0 && currentLine < req.StartLine {
			currentLine++
			continue
		}
		if req.EndLine > 0 && currentLine > req.EndLine {
			break
		}
		lines = append(lines, scanner.Text())
		currentLine++
	}

	if err := scanner.Err(); err != nil {
		return &FileReadResponse{
			ErrorMessage: fmt.Sprintf("读取文件时出错: %v", err),
		}, nil
	}

	return &FileReadResponse{
		Content: strings.Join(lines, "\n"),
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
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// FileWrite 写入文件（覆盖模式）
func FileWrite(ctx context.Context, req *FileWriteRequest) (*FileWriteResponse, error) {
	if req.Filepath == "" {
		return &FileWriteResponse{
			ErrorMessage: "filepath 参数是必需的",
		}, nil
	}

	err := os.WriteFile(req.Filepath, []byte(req.Content), 0644)
	if err != nil {
		return &FileWriteResponse{
			ErrorMessage: fmt.Sprintf("写入文件失败: %v", err),
		}, nil
	}

	return &FileWriteResponse{
		Message: fmt.Sprintf("成功写入 %d 字节到文件 %s", len(req.Content), req.Filepath),
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
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// FileAppend 在文件末尾追加内容
func FileAppend(ctx context.Context, req *FileAppendRequest) (*FileAppendResponse, error) {
	if req.Filepath == "" {
		return &FileAppendResponse{
			ErrorMessage: "filepath 参数是必需的",
		}, nil
	}

	file, err := os.OpenFile(req.Filepath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return &FileAppendResponse{
			ErrorMessage: fmt.Sprintf("无法打开文件: %v", err),
		}, nil
	}
	defer file.Close()

	n, err := file.WriteString(req.Content)
	if err != nil {
		return &FileAppendResponse{
			ErrorMessage: fmt.Sprintf("追加内容失败: %v", err),
		}, nil
	}

	return &FileAppendResponse{
		Message: fmt.Sprintf("成功追加 %d 字节到文件 %s", n, req.Filepath),
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
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// FileModify 修改文件中指定行范围的内容
func FileModify(ctx context.Context, req *FileModifyRequest) (*FileModifyResponse, error) {
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

	// 读取原文件
	file, err := os.Open(req.Filepath)
	if err != nil {
		return &FileModifyResponse{
			ErrorMessage: fmt.Sprintf("无法打开文件: %v", err),
		}, nil
	}

	scanner := bufio.NewScanner(file)
	var lines []string
	currentLine := 1

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		currentLine++
	}
	file.Close()

	if err := scanner.Err(); err != nil {
		return &FileModifyResponse{
			ErrorMessage: fmt.Sprintf("读取文件时出错: %v", err),
		}, nil
	}

	if req.StartLine > len(lines) {
		return &FileModifyResponse{
			ErrorMessage: fmt.Sprintf("起始行号 %d 超出文件总行数 %d", req.StartLine, len(lines)),
		}, nil
	}

	// 构建新内容
	var result []string
	result = append(result, lines[:req.StartLine-1]...)
	result = append(result, req.NewContent)
	if req.EndLine < len(lines) {
		result = append(result, lines[req.EndLine:]...)
	}

	// 写回文件
	finalContent := strings.Join(result, "\n")
	err = os.WriteFile(req.Filepath, []byte(finalContent), 0644)
	if err != nil {
		return &FileModifyResponse{
			ErrorMessage: fmt.Sprintf("写入文件失败: %v", err),
		}, nil
	}

	return &FileModifyResponse{
		Message: fmt.Sprintf("成功修改文件 %s 的第 %d-%d 行", req.Filepath, req.StartLine, req.EndLine),
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
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// FileInsert 在文件的指定行之后插入新内容
func FileInsert(ctx context.Context, req *FileInsertRequest) (*FileInsertResponse, error) {
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

	// 读取原文件
	file, err := os.Open(req.Filepath)
	if err != nil {
		return &FileInsertResponse{
			ErrorMessage: fmt.Sprintf("无法打开文件: %v", err),
		}, nil
	}

	scanner := bufio.NewScanner(file)
	var lines []string

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	file.Close()

	if err := scanner.Err(); err != nil {
		return &FileInsertResponse{
			ErrorMessage: fmt.Sprintf("读取文件时出错: %v", err),
		}, nil
	}

	if req.LineNumber > len(lines) {
		return &FileInsertResponse{
			ErrorMessage: fmt.Sprintf("行号 %d 超出文件总行数 %d", req.LineNumber, len(lines)),
		}, nil
	}

	// 构建新内容
	var result []string
	if req.LineNumber == 0 {
		result = append(result, req.Content)
		result = append(result, lines...)
	} else {
		result = append(result, lines[:req.LineNumber]...)
		result = append(result, req.Content)
		result = append(result, lines[req.LineNumber:]...)
	}

	// 写回文件
	finalContent := strings.Join(result, "\n")
	err = os.WriteFile(req.Filepath, []byte(finalContent), 0644)
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
		Message: fmt.Sprintf("成功在文件 %s 的%s插入内容", req.Filepath, position),
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
func FileList(ctx context.Context, req *FileListRequest) (*FileListResponse, error) {
	if req.Dirpath == "" {
		return &FileListResponse{
			ErrorMessage: "dirpath 参数是必需的",
		}, nil
	}

	entries, err := os.ReadDir(req.Dirpath)
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
			info, _ := entry.Info()
			size := info.Size()
			result = append(result, fmt.Sprintf("[FILE] %s (%d bytes)", entry.Name(), size))
		}
	}

	return &FileListResponse{
		Content: strings.Join(result, "\n"),
	}, nil
}

// GetFileTools 获取所有文件操作工具
func GetFileTools() ([]tool.BaseTool, error) {
	var tools []tool.BaseTool

	// 文件读取工具
	fileReadTool, err := utils.InferTool("file_read", "读取文件内容。支持完整读取或部分读取（指定行范围）", FileRead)
	if err != nil {
		return nil, err
	}
	tools = append(tools, fileReadTool)

	// 文件写入工具
	fileWriteTool, err := utils.InferTool("file_write", "写入内容到文件（覆盖模式）。如果文件不存在会创建，如果存在会覆盖全部内容", FileWrite)
	if err != nil {
		return nil, err
	}
	tools = append(tools, fileWriteTool)

	// 文件追加工具
	fileAppendTool, err := utils.InferTool("file_append", "在文件末尾追加内容。如果文件不存在会创建", FileAppend)
	if err != nil {
		return nil, err
	}
	tools = append(tools, fileAppendTool)

	// 文件修改工具
	fileModifyTool, err := utils.InferTool("file_modify", "修改文件中指定行范围的内容。可以替换指定的行", FileModify)
	if err != nil {
		return nil, err
	}
	tools = append(tools, fileModifyTool)

	// 文件插入工具
	fileInsertTool, err := utils.InferTool("file_insert", "在文件的指定行之后插入新内容", FileInsert)
	if err != nil {
		return nil, err
	}
	tools = append(tools, fileInsertTool)

	// 文件列表工具
	fileListTool, err := utils.InferTool("file_list", "列出目录下的文件和文件夹", FileList)
	if err != nil {
		return nil, err
	}
	tools = append(tools, fileListTool)

	return tools, nil
}
