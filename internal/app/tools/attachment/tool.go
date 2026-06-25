package attachment

import (
	runtimeport "fkteams/internal/ports/runtime"
)

const listDesc = `列出当前会话历史中的多模态附件。

当历史上下文提示某条历史消息包含图片、音频、视频或文件，且你需要确认这些附件时，先使用本工具查看附件 ID、类型、来源消息和可用数据。`

const readDesc = `读取当前会话历史中的指定附件。

用法：
- attachment_id 来自历史上下文提示或 session_attachment_list
- 默认只返回附件元数据、URL 或数据摘要，不返回大体积 base64
- include_data_url=true 时，仅在附件数据较小时返回 data URL
- 对图片，纯文本模型不能直接理解像素；如果需要视觉理解，应说明需要支持视觉的模型或图像描述能力`

func GetTools() ([]runtimeport.Tool, error) {
	listTool, err := runtimeport.InferTool("session_attachment_list", listDesc, List)
	if err != nil {
		return nil, err
	}
	readTool, err := runtimeport.InferTool("session_attachment_read", readDesc, Read)
	if err != nil {
		return nil, err
	}
	return []runtimeport.Tool{listTool, readTool}, nil
}
