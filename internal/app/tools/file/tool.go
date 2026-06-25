package file

import (
	runtimeport "fkteams/internal/ports/runtime"
	"fmt"
)

// GetTools 获取所有文件操作工具
func (ft *FileTools) GetTools() ([]runtimeport.Tool, error) {
	if ft == nil || ft.securedFs == nil {
		return nil, fmt.Errorf("文件工具未初始化")
	}

	var tools []runtimeport.Tool

	fileReadTool, err := runtimeport.InferTool("file_read", fileReadDesc, ft.FileRead)
	if err != nil {
		return nil, err
	}
	tools = append(tools, fileReadTool)

	fileWriteTool, err := runtimeport.InferTool("file_write", fileWriteDesc, ft.FileWrite)
	if err != nil {
		return nil, err
	}
	tools = append(tools, fileWriteTool)

	fileAppendTool, err := runtimeport.InferTool("file_append", fileAppendDesc, ft.FileAppend)
	if err != nil {
		return nil, err
	}
	tools = append(tools, fileAppendTool)

	fileEditTool, err := runtimeport.InferTool("file_edit", fileEditDesc, ft.FileEdit)
	if err != nil {
		return nil, err
	}
	tools = append(tools, fileEditTool)

	grepTool, err := runtimeport.InferTool("grep", grepDesc, ft.Grep)
	if err != nil {
		return nil, err
	}
	tools = append(tools, grepTool)

	fileListTool, err := runtimeport.InferTool("file_list", fileListDesc, ft.FileList)
	if err != nil {
		return nil, err
	}
	tools = append(tools, fileListTool)

	globTool, err := runtimeport.InferTool("glob", globDesc, ft.Glob)
	if err != nil {
		return nil, err
	}
	tools = append(tools, globTool)

	filePatchTool, err := runtimeport.InferTool("file_patch", filePatchDesc, ft.FilePatch)
	if err != nil {
		return nil, err
	}
	tools = append(tools, filePatchTool)

	return tools, nil
}
