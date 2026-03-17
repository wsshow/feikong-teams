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
	if g.MemoryManager != nil {
		memories := g.MemoryManager.Search(userInput, 5)
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

// BuildMultimodalInputMessages 构建多模态输入消息列表（长期记忆 + 对话历史 + 多模态内容）
func BuildMultimodalInputMessages(recorder *fkevent.HistoryRecorder, textContent string, parts []schema.MessageInputPart) []adk.Message {
	var inputMessages []adk.Message

	// 注入长期记忆（使用文本部分进行搜索）
	if g.MemoryManager != nil {
		memories := g.MemoryManager.Search(textContent, 5)
		if memCtx := memory.BuildMemoryContext(memories); memCtx != "" {
			inputMessages = append(inputMessages, schema.SystemMessage(memCtx))
		}
	}

	// 对话历史
	inputMessages = append(inputMessages, buildHistoryMessages(recorder)...)

	// 添加多模态用户输入
	inputMessages = append(inputMessages, &schema.Message{
		Role:                  schema.User,
		UserInputMultiContent: parts,
	})
	return inputMessages
}

// TextPart 创建文本内容部分
func TextPart(text string) schema.MessageInputPart {
	return schema.MessageInputPart{
		Type: schema.ChatMessagePartTypeText,
		Text: text,
	}
}

// ImageURLPart 创建图片 URL 内容部分
func ImageURLPart(url string, detail ...schema.ImageURLDetail) schema.MessageInputPart {
	d := schema.ImageURLDetailAuto
	if len(detail) > 0 {
		d = detail[0]
	}
	return schema.MessageInputPart{
		Type: schema.ChatMessagePartTypeImageURL,
		Image: &schema.MessageInputImage{
			MessagePartCommon: schema.MessagePartCommon{
				URL: &url,
			},
			Detail: d,
		},
	}
}

// ImageBase64Part 创建 Base64 编码图片内容部分
func ImageBase64Part(base64Data, mimeType string) schema.MessageInputPart {
	return schema.MessageInputPart{
		Type: schema.ChatMessagePartTypeImageURL,
		Image: &schema.MessageInputImage{
			MessagePartCommon: schema.MessagePartCommon{
				Base64Data: &base64Data,
				MIMEType:   mimeType,
			},
		},
	}
}

// AudioURLPart 创建音频 URL 内容部分
func AudioURLPart(url string) schema.MessageInputPart {
	return schema.MessageInputPart{
		Type: schema.ChatMessagePartTypeAudioURL,
		Audio: &schema.MessageInputAudio{
			MessagePartCommon: schema.MessagePartCommon{
				URL: &url,
			},
		},
	}
}

// VideoURLPart 创建视频 URL 内容部分
func VideoURLPart(url string) schema.MessageInputPart {
	return schema.MessageInputPart{
		Type: schema.ChatMessagePartTypeVideoURL,
		Video: &schema.MessageInputVideo{
			MessagePartCommon: schema.MessagePartCommon{
				URL: &url,
			},
		},
	}
}

// FileURLPart 创建文件 URL 内容部分
func FileURLPart(url string) schema.MessageInputPart {
	return schema.MessageInputPart{
		Type: schema.ChatMessagePartTypeFileURL,
		File: &schema.MessageInputFile{
			MessagePartCommon: schema.MessagePartCommon{
				URL: &url,
			},
		},
	}
}

// ExtractTextFromParts 从多模态内容中提取纯文本
func ExtractTextFromParts(parts []schema.MessageInputPart) string {
	var texts []string
	for _, p := range parts {
		if p.Type == schema.ChatMessagePartTypeText && p.Text != "" {
			texts = append(texts, p.Text)
		}
	}
	return strings.Join(texts, " ")
}

// buildHistoryMessages 将历史记录构建为消息列表，支持摘要压缩
func buildHistoryMessages(recorder *fkevent.HistoryRecorder) []adk.Message {
	agentMessages := recorder.GetMessages()
	summaryText, summarizedCount := recorder.GetSummary()

	if summaryText != "" && summarizedCount > 0 {
		// 有摘要时：摘要 + 未被摘要覆盖的最近记录
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
		// 无摘要时：使用全部历史记录
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
