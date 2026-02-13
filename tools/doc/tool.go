package doc

import (
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

func GetTools() (tools []tool.BaseTool, err error) {
	// 1. 获取文档信息工具
	getInfoTool, err := utils.InferTool(
		"get_document_info",
		`获取文档的基本信息，包括文件类型、大小、页数、元数据等。支持格式：.docx, .pdf, .xlsx, .pptx, .txt, .csv, .md, .rtf。
这是读取文档前的第一步，帮助你了解文档结构，决定如何读取。`,
		GetDocumentInfo,
	)
	if err != nil {
		return nil, err
	}
	tools = append(tools, getInfoTool)

	// 2. 智能读取工具（推荐首选）
	smartReadTool, err := utils.InferTool(
		"read_document_smart",
		`智能读取文档内容，自动处理大文档。特点：
- 自动适配上下文限制（默认50000字符）
- 支持采样模式（均匀采样）或从头截断
- 自动清理多余空格和空行
- 文档过大时会提供建议
适用于：首次阅读文档，快速了解内容概况`,
		ReadDocumentSmart,
	)
	if err != nil {
		return nil, err
	}
	tools = append(tools, smartReadTool)

	// 3. 按页读取工具
	readByPagesTool, err := utils.InferTool(
		"read_document_by_pages",
		`按页码范围读取文档内容。支持多页文档如 PDF、PPTX。
参数：
- start_page: 起始页（从0开始）
- end_page: 结束页（-1表示到末尾）
返回每页的详细信息（页码、行数）。
适用于：需要读取特定页面或章节`,
		ReadDocumentByPages,
	)
	if err != nil {
		return nil, err
	}
	tools = append(tools, readByPagesTool)

	// 4. 按行读取工具
	readByLinesTool, err := utils.InferTool(
		"read_document_by_lines",
		`按行号范围读取文档内容。支持指定页面和行范围。
参数：
- start_line: 起始行（从0开始）
- end_line: 结束行（-1表示到末尾）
- page_index: 页面索引（-1表示第一页）
适用于：需要读取特定行或段落`,
		ReadDocumentByLines,
	)
	if err != nil {
		return nil, err
	}
	tools = append(tools, readByLinesTool)

	return tools, nil
}
