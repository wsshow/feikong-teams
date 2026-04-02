package file

import (
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

const fileWriteDesc = `创建或覆盖文件，自动创建父目录。

## 重要：分段写入规则
content 参数单次不要超过 200 行。超过 200 行的内容会因为输出 token 限制导致 JSON 被截断而调用失败。

正确的大文件写入流程：
1. 先用 file_write 写入文件的前半部分（不超过 200 行）
2. 再用 file_append 分段追加剩余内容（每段不超过 200 行）
3. 重复调用 file_append 直到全部内容写完

示例 - 创建一个 500 行的文件：
  第1步: file_write(filepath="index.html", content="前200行内容...")
  第2步: file_append(filepath="index.html", content="第201-400行...")
  第3步: file_append(filepath="index.html", content="第401-500行...")`

const fileAppendDesc = `追加内容到文件末尾（文件不存在则创建）。

用于配合 file_write 分段写入大文件，每次追加不超过 200 行。
当需要创建超过 200 行的文件时，先用 file_write 写前半部分，再用 file_append 逐段追加直到写完。`

const filePatchDesc = `使用 unified diff 格式批量修改文件，支持模糊匹配（行号允许偏差）。

适用场景：对已有文件做少量精确修改（少于文件 50% 内容且少于 10 个 hunk）。
超出此范围时，建议用 file_write + file_append 重写整个文件。

格式：
  --- file
  +++ file
  @@ -start,count +start,count @@
   context line
  -deleted line
  +inserted line
   context line`

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

	fileWriteTool, err := utils.InferTool("file_write", fileWriteDesc, ft.FileWrite)
	if err != nil {
		return nil, err
	}
	tools = append(tools, fileWriteTool)

	fileAppendTool, err := utils.InferTool("file_append", fileAppendDesc, ft.FileAppend)
	if err != nil {
		return nil, err
	}
	tools = append(tools, fileAppendTool)

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

	filePatchTool, err := utils.InferTool("file_patch", filePatchDesc, ft.FilePatch)
	if err != nil {
		return nil, err
	}
	tools = append(tools, filePatchTool)

	return tools, nil
}
