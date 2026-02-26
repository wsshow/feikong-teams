package file

import (
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// GetTools 获取所有文件操作工具
// ft 是文件工具实例，包含文件系统配置
func (ft *FileTools) GetTools() ([]tool.BaseTool, error) {
	if ft == nil || ft.securedFs == nil {
		return nil, fmt.Errorf("文件工具未初始化")
	}

	var tools []tool.BaseTool

	// 文件读取工具
	fileReadTool, err := utils.InferTool("file_read", "读取文件内容。支持完整读取或指定行范围读取，超过200行自动截断", ft.FileRead)
	if err != nil {
		return nil, err
	}
	tools = append(tools, fileReadTool)

	// 统一文件编辑工具（推荐）
	fileEditTool, err := utils.InferTool("file_edit", "统一文件编辑工具。支持三种模式: write(创建/覆盖文件)、append(追加内容)、replace(精确查找并替换文本，需唯一匹配)", ft.FileEdit)
	if err != nil {
		return nil, err
	}
	tools = append(tools, fileEditTool)

	// 文件列表工具
	fileListTool, err := utils.InferTool("file_list", "列出目录下的文件和文件夹", ft.FileList)
	if err != nil {
		return nil, err
	}
	tools = append(tools, fileListTool)

	// 创建空文件工具
	fileCreateTool, err := utils.InferTool("file_create", "创建一个新的空文件。如果文件已存在则会清空其内容", ft.FileCreate)
	if err != nil {
		return nil, err
	}
	tools = append(tools, fileCreateTool)

	// 删除文件工具
	fileDeleteTool, err := utils.InferTool("file_delete", "删除指定的文件。注意：删除操作不可恢复", ft.FileDelete)
	if err != nil {
		return nil, err
	}
	tools = append(tools, fileDeleteTool)

	// 创建目录工具
	dirCreateTool, err := utils.InferTool("dir_create", "创建一个新的目录（文件夹）。支持递归创建多级目录", ft.DirCreate)
	if err != nil {
		return nil, err
	}
	tools = append(tools, dirCreateTool)

	// 删除目录工具
	dirDeleteTool, err := utils.InferTool("dir_delete", "删除指定的目录（文件夹）。注意：删除操作不可恢复，目录必须为空才能删除", ft.DirDelete)
	if err != nil {
		return nil, err
	}
	tools = append(tools, dirDeleteTool)

	// 文件搜索工具
	fileSearchTool, err := utils.InferTool("file_search", "在文件中搜索指定的文本。默认纯文本精确匹配，设置 use_regex=true 启用正则表达式", ft.FileSearch)
	if err != nil {
		return nil, err
	}
	tools = append(tools, fileSearchTool)

	// 多文件 patch 工具
	filePatchTool, err := utils.InferTool("file_patch", "使用 unified diff 格式批量修改多个文件。支持模糊匹配，适合精确的代码批量修改。注意: 当修改超过文件50%内容或超过10个hunk时，建议使用 file_edit(action=write) 直接重写文件。patch 格式: --- file\n+++ file\n@@ -start,count +start,count @@\n context\n-deleted\n+inserted", ft.FilePatch)
	if err != nil {
		return nil, err
	}
	tools = append(tools, filePatchTool)

	// 文件 diff 工具
	fileDiffTool, err := utils.InferTool("file_diff", "计算文件当前内容与新内容之间的差异，返回 unified diff 格式", ft.FileDiff)
	if err != nil {
		return nil, err
	}
	tools = append(tools, fileDiffTool)

	return tools, nil
}
