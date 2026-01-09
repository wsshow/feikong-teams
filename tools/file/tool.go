package file

import (
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// GetTools 获取所有文件操作工具
// 注意: 调用此函数前必须先调用 InitSecuredFileSystem 初始化文件系统
func GetTools() ([]tool.BaseTool, error) {
	if securedFs == nil {
		return nil, fmt.Errorf("文件系统未初始化，请先调用 InitSecuredFileSystem")
	}

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

	// 创建空文件工具
	fileCreateTool, err := utils.InferTool("file_create", "创建一个新的空文件。如果文件已存在则会清空其内容", FileCreate)
	if err != nil {
		return nil, err
	}
	tools = append(tools, fileCreateTool)

	// 删除文件工具
	fileDeleteTool, err := utils.InferTool("file_delete", "删除指定的文件。注意：删除操作不可恢复", FileDelete)
	if err != nil {
		return nil, err
	}
	tools = append(tools, fileDeleteTool)

	// 创建目录工具
	dirCreateTool, err := utils.InferTool("dir_create", "创建一个新的目录（文件夹）。支持递归创建多级目录", DirCreate)
	if err != nil {
		return nil, err
	}
	tools = append(tools, dirCreateTool)

	// 删除目录工具
	dirDeleteTool, err := utils.InferTool("dir_delete", "删除指定的目录（文件夹）。注意：删除操作不可恢复，目录必须为空才能删除", DirDelete)
	if err != nil {
		return nil, err
	}
	tools = append(tools, dirDeleteTool)

	return tools, nil
}
