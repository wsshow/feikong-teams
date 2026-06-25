package fetch

import (
	runtimeport "fkteams/internal/ports/runtime"
)

const toolDescription = `从 URL 获取网络资源内容并返回指定格式的内容。

## 何时使用
使用此工具当你需要:
- 获取网页的原始内容
- 访问API接口获取JSON数据
- 下载HTML/文本/Markdown内容
- 对搜索结果中的关键来源进行原文核验

不要使用此工具当你需要:
- 只需要候选链接（先用搜索工具）
- 访问需要登录、私有权限或动态交互的页面
- 分析或总结网页内容（本工具只负责抓取；抓取后你再分析）

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
- 根据网站速度设置合适的超时时间(默认30秒,最大120秒)
- 抓取多个独立来源时可以并行调用
- 如果 HTTP 状态、跳转或内容类型异常，要在结论中保留该限制`

func GetTools() (tools []runtimeport.Tool, err error) {
	// HTTP请求工具
	fetchTool, err := runtimeport.InferTool("fetch", toolDescription, Fetch)
	if err != nil {
		return nil, err
	}
	tools = append(tools, fetchTool)

	return tools, nil
}
