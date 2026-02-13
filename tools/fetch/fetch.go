package fetch

import (
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

const toolDescription = `从URL获取网络资源内容并返回指定格式的内容。

## 何时使用
使用此工具当你需要:
- 获取网页的原始内容
- 访问API接口获取JSON数据
- 下载HTML/文本/Markdown内容
- 快速获取网络资源而无需复杂处理

不要使用此工具当你需要:
- 从网页中提取特定信息(应使用专门的提取工具)
- 分析或总结网页内容(应先获取再分析)

## 功能特性
- 支持四种输出格式: text(纯文本)、markdown(Markdown格式)、html(HTML格式)、json(JSON格式)
- 自动处理HTTP重定向
- 自动从HTML提取纯文本(text格式)
- 自动将HTML转换为Markdown(markdown格式)
- 设置合理的超时时间防止长时间等待
- 限制响应大小(最大5MB)防止内存溢出

## 使用提示
- text格式: 适用于获取纯文本内容或从HTML中提取文本
- markdown格式: 适用于需要格式化渲染的内容
- html格式: 适用于需要原始HTML结构的场景
- json格式: 适用于API接口返回的JSON数据
- 根据网站速度设置合适的超时时间(默认30秒,最大120秒)`

func GetTools() (tools []tool.BaseTool, err error) {
	// HTTP请求工具
	fetchTool, err := utils.InferTool("fetch", toolDescription, Fetch)
	if err != nil {
		return nil, err
	}
	tools = append(tools, fetchTool)

	return tools, nil
}
