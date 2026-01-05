package fkevent

import (
	"fmt"
	"strings"
)

var PrintEvent = printEvent()

func printEvent() func(Event) {
	agentName := ""
	return func(event Event) {
		switch event.Type {
		case "stream_chunk":
			// 流式输出内容块，显示代理名称和路径
			if agentName != event.AgentName {
				agentName = event.AgentName
				fmt.Printf("\n\033[1;36m╭─ [%s] %s\033[0m\n", agentName, event.RunPath)
				fmt.Printf("\033[1;36m╰─▶\033[0m ")
			}
			fmt.Printf("%s", event.Content)

		case "message":
			// 完整消息输出
			if event.Content != "" {
				fmt.Printf("\n\033[1;32m✓ [%s] 消息:\033[0m %s\n", event.AgentName, event.Content)
			}

		case "tool_result":
			// 工具执行结果
			fmt.Printf("\n\033[1;33m⚙ [%s] 工具结果:\033[0m\n", event.AgentName)
			if event.Content != "" {
				// 缩进显示工具结果
				lines := strings.Split(event.Content, "\n")
				for _, line := range lines {
					if line != "" {
						fmt.Printf("  │ %s\n", line)
					}
				}
			}
			fmt.Println()

		case "tool_result_chunk":
			// 工具结果流式输出块
			fmt.Printf("%s", event.Content)

		case "tool_calls":
			// 工具调用信息
			fmt.Printf("\n\033[1;35m🔧 [%s] 调用工具:\033[0m\n", event.AgentName)
			for i, tool := range event.ToolCalls {
				fmt.Printf("  %d. \033[1m%s\033[0m\n", i+1, tool.Function.Name)
				if tool.Function.Arguments != "" {
					// 显示参数（截断过长的参数）
					args := tool.Function.Arguments
					if len(args) > 200 {
						args = args[:200] + "..."
					}
					fmt.Printf("     参数: %s\n", args)
				}
			}
			fmt.Println()

		case "action":
			// 动作类型事件
			fmt.Printf("\n\033[1;34m▸ [%s] 动作: %s\033[0m\n", event.AgentName, event.ActionType)
			if event.Content != "" {
				fmt.Printf("  详情: %s\n", event.Content)
			}

		case "error":
			// 错误信息，红色高亮显示
			fmt.Printf("\n\033[1;31m✗ [%s] 错误:\033[0m\n", event.AgentName)
			fmt.Printf("  \033[31m%s\033[0m\n", event.Error)
			if event.RunPath != "" {
				fmt.Printf("  路径: %s\n", event.RunPath)
			}
			fmt.Println()

		default:
			// 未知事件类型
			fmt.Printf("\n\033[1;90m? 未知事件类型: %s\033[0m\n", event.Type)
			if event.AgentName != "" {
				fmt.Printf("  代理: %s\n", event.AgentName)
			}
			if event.Content != "" {
				fmt.Printf("  内容: %s\n", event.Content)
			}
		}
	}
}
