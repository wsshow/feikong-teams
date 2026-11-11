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

	return runtimeport.NewTools([]runtimeport.ToolDefinition{
		{Name: "file_read", Description: fileReadDesc, Handler: ft.FileRead},
		{Name: "file_write", Description: fileWriteDesc, Handler: ft.FileWrite},
		{Name: "file_append", Description: fileAppendDesc, Handler: ft.FileAppend},
		{Name: "file_edit", Description: fileEditDesc, Handler: ft.FileEdit},
		{Name: "grep", Description: grepDesc, Handler: ft.Grep},
		{Name: "file_list", Description: fileListDesc, Handler: ft.FileList},
		{Name: "glob", Description: globDesc, Handler: ft.Glob},
		{Name: "file_patch", Description: filePatchDesc, Handler: ft.FilePatch},
	})
}
