package file

import (
	"fmt"

	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/resources"
)

// GetTools 获取所有文件操作工具
func (ft *FileTools) GetTools() ([]runtimeport.Tool, error) {
	if ft == nil || ft.securedFs == nil {
		return nil, fmt.Errorf("文件工具未初始化")
	}

	return runtimeport.NewTools([]runtimeport.ToolDefinition{
		{Name: "file_read", Description: fileReadDesc, Handler: resources.Read(ft.locks, ft.allowedBaseDir, ft.FileRead)},
		{Name: "file_write", Description: fileWriteDesc, Handler: resources.Serialize(ft.locks, ft.allowedBaseDir, ft.FileWrite)},
		{Name: "file_append", Description: fileAppendDesc, Handler: resources.Serialize(ft.locks, ft.allowedBaseDir, ft.FileAppend)},
		{Name: "file_edit", Description: fileEditDesc, Handler: resources.Serialize(ft.locks, ft.allowedBaseDir, ft.FileEdit)},
		{Name: "grep", Description: grepDesc, Handler: resources.Read(ft.locks, ft.allowedBaseDir, ft.Grep)},
		{Name: "file_list", Description: fileListDesc, Handler: resources.Read(ft.locks, ft.allowedBaseDir, ft.FileList)},
		{Name: "glob", Description: globDesc, Handler: resources.Read(ft.locks, ft.allowedBaseDir, ft.Glob)},
		{Name: "file_patch", Description: filePatchDesc, Handler: resources.Serialize(ft.locks, ft.allowedBaseDir, ft.FilePatch)},
	})
}
