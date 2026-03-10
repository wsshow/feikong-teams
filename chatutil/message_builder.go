// Package chatutil 提供 CLI 和 Web 共享的聊天工具函数
package chatutil

import (
	"fkteams/fkevent"
	"fkteams/g"
	"fkteams/memory"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// BuildInputMessages 构建输入消息列表（长期记忆 + 对话历史 + 用户输入）
func BuildInputMessages(recorder *fkevent.HistoryRecorder, userInput string) []adk.Message {
	var inputMessages []adk.Message

	// 注入长期记忆
	if g.MemManager != nil {
		memories := g.MemManager.Search(userInput, 5)
		if memCtx := memory.BuildMemoryContext(memories); memCtx != "" {
			inputMessages = append(inputMessages, schema.SystemMessage(memCtx))
		}
	}

	// 对话历史
	inputMessages = append(inputMessages, buildHistoryMessages(recorder)...)

	// 添加用户输入
	inputMessages = append(inputMessages, schema.UserMessage(userInput))
	return inputMessages
}

// buildHistoryMessages 根据历史记录构建消息
func buildHistoryMessages(recorder *fkevent.HistoryRecorder) []adk.Message {
	agentMessages := recorder.GetMessages()
	summaryText, summarizedCount := recorder.GetSummary()

	if summaryText != "" && summarizedCount > 0 {
		// 有摘要：摘要 + 最近未覆盖的记录
		var historyMessage strings.Builder
		historyMessage.WriteString("## 对话历史摘要\n")
		historyMessage.WriteString(summaryText)

		if summarizedCount < len(agentMessages) {
			historyMessage.WriteString("\n\n## 最近的对话记录\n")
			for _, msg := range agentMessages[summarizedCount:] {
				fmt.Fprintf(&historyMessage, "%s: %s\n", msg.AgentName, msg.GetTextContent())
			}
		}

		return []adk.Message{
			schema.SystemMessage(
				fmt.Sprintf("以下是之前的对话历史:\n---\n%s\n---\n", historyMessage.String()),
			),
		}
	}

	if len(agentMessages) > 0 {
		// 无摘要：全部历史
		var historyMessage strings.Builder
		for _, msg := range agentMessages {
			fmt.Fprintf(&historyMessage, "%s: %s\n", msg.AgentName, msg.GetTextContent())
		}
		return []adk.Message{
			schema.SystemMessage(
				fmt.Sprintf("以下是之前的对话历史:\n---\n%s\n---\n", historyMessage.String()),
			),
		}
	}

	return nil
}
