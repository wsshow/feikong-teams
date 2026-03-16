package file

import (
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// GetTools 获取所有文件操作工具
func (ft *FileTools) GetTools() ([]tool.BaseTool, error) {
	if ft == nil || ft.securedFs == nil {
		return nil, fmt.Errorf("文件工具未初始化")
	}

	var tools []tool.BaseTool

	fileReadTool, err := utils.InferTool("file_read", "读取文件内容，支持完整读取或按行范围读取，超过200行自动截断", ft.FileRead)
	if err != nil {
		return nil, err
	}
	tools = append(tools, fileReadTool)

	fileWriteTool, err := utils.InferTool("file_write", "创建或覆盖文件，自动创建父目录", ft.FileWrite)
	if err != nil {
		return nil, err
	}
	tools = append(tools, fileWriteTool)

	fileEditTool, err := utils.InferTool("file_edit", "精确查找并替换文件内容。old_string 必须唯一匹配，new_string 为空则删除匹配文本", ft.FileEdit)
	if err != nil {
		return nil, err
	}
	tools = append(tools, fileEditTool)

	grepTool, err := utils.InferTool("grep", "搜索文件或目录中的文本。支持正则、glob过滤、上下文行显示", ft.Grep)
	if err != nil {
		return nil, err
	}
	tools = append(tools, grepTool)

	fileListTool, err := utils.InferTool("file_list", "列出目录下的文件和文件夹", ft.FileList)
	if err != nil {
		return nil, err
	}
	tools = append(tools, fileListTool)

	filePatchTool, err := utils.InferTool("file_patch", "使用 unified diff 格式批量修改文件。支持模糊匹配(行号允许偏差)。当修改超过文件50%内容或超过10个hunk时，建议用 file_write 重写。格式: --- file\\n+++ file\\n@@ -start,count +start,count @@\\n context\\n-deleted\\n+inserted", ft.FilePatch)
	if err != nil {
		return nil, err
	}
	tools = append(tools, filePatchTool)

	return tools, nil
}
