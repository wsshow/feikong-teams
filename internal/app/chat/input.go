// Package chat 提供聊天用例服务和输入构建能力。
package chat

import (
	"fmt"
	"strings"

	"fkteams/appstate"
	"fkteams/events/log"
	"fkteams/fkenv"
	domainmessage "fkteams/internal/domain/message"
	"fkteams/log"
	"fkteams/memory"
)

// BuildTurnInput 构建一轮输入（长期记忆 + 对话历史 + 用户输入）
func BuildTurnInput(recorder *eventlog.HistoryRecorder, userInput string) domainmessage.TurnInput {
	return BuildTurnInputWithMemory(recorder, userInput, nil)
}

// BuildTurnInputWithMemory 构建一轮输入并按需注入长期记忆。
func BuildTurnInputWithMemory(recorder *eventlog.HistoryRecorder, userInput string, manager appstate.MemoryManager) domainmessage.TurnInput {
	var contextMessages []domainmessage.Message

	// 注入长期记忆
	if manager != nil {
		memories := manager.Search(userInput, 5)
		if memCtx := memory.BuildMemoryContext(memories); memCtx != "" {
			contextMessages = append(contextMessages, domainmessage.Message{Role: domainmessage.RoleSystem, Content: memCtx})
		}
	}

	// 对话历史
	contextMessages = append(contextMessages, buildHistoryMessages(recorder)...)
	message := domainmessage.Message{Role: domainmessage.RoleUser, Content: userInput}

	if debugContextEnabled() {
		logMessages("BuildTurnInput", append(contextMessages, message))
	}
	return domainmessage.TurnInput{
		Context: contextMessages,
		Message: message,
	}
}

// BuildMultimodalTurnInput 构建一轮多模态输入（长期记忆 + 对话历史 + 多模态内容）
func BuildMultimodalTurnInput(recorder *eventlog.HistoryRecorder, textContent string, parts []domainmessage.ContentPart) domainmessage.TurnInput {
	return BuildMultimodalTurnInputWithMemory(recorder, textContent, parts, nil)
}

// BuildMultimodalTurnInputWithMemory 构建多模态输入并按需注入长期记忆。
func BuildMultimodalTurnInputWithMemory(recorder *eventlog.HistoryRecorder, textContent string, parts []domainmessage.ContentPart, manager appstate.MemoryManager) domainmessage.TurnInput {
	var contextMessages []domainmessage.Message

	// 注入长期记忆（使用文本部分进行搜索）
	if manager != nil {
		memories := manager.Search(textContent, 5)
		if memCtx := memory.BuildMemoryContext(memories); memCtx != "" {
			contextMessages = append(contextMessages, domainmessage.Message{Role: domainmessage.RoleSystem, Content: memCtx})
		}
	}

	// 对话历史
	contextMessages = append(contextMessages, buildHistoryMessages(recorder)...)
	message := domainmessage.Message{
		Role:         domainmessage.RoleUser,
		ContentParts: parts,
	}

	if debugContextEnabled() {
		logMessages("BuildMultimodalTurnInput", append(contextMessages, message))
	}
	return domainmessage.TurnInput{
		Context: contextMessages,
		Message: message,
	}
}

// TextPart 创建文本内容部分
func TextPart(text string) domainmessage.ContentPart {
	return domainmessage.ContentPart{
		Type: domainmessage.ContentPartText,
		Text: text,
	}
}

// ImageURLPart 创建图片 URL 内容部分
func ImageURLPart(url string, detail ...string) domainmessage.ContentPart {
	d := "auto"
	if len(detail) > 0 {
		d = detail[0]
	}
	return domainmessage.ContentPart{
		Type:   domainmessage.ContentPartImageURL,
		URL:    url,
		Detail: d,
	}
}

// ImageBase64Part 创建 Base64 编码图片内容部分
func ImageBase64Part(base64Data, mimeType string) domainmessage.ContentPart {
	return domainmessage.ContentPart{
		Type:       domainmessage.ContentPartImageURL,
		Base64Data: base64Data,
		MIMEType:   mimeType,
	}
}

// AudioURLPart 创建音频 URL 内容部分
func AudioURLPart(url string) domainmessage.ContentPart {
	return domainmessage.ContentPart{
		Type: domainmessage.ContentPartAudioURL,
		URL:  url,
	}
}

// VideoURLPart 创建视频 URL 内容部分
func VideoURLPart(url string) domainmessage.ContentPart {
	return domainmessage.ContentPart{
		Type: domainmessage.ContentPartVideoURL,
		URL:  url,
	}
}

// FileURLPart 创建文件 URL 内容部分
func FileURLPart(url string) domainmessage.ContentPart {
	return domainmessage.ContentPart{
		Type: domainmessage.ContentPartFileURL,
		URL:  url,
	}
}

// ExtractTextFromParts 从多模态内容中提取纯文本
func ExtractTextFromParts(parts []domainmessage.ContentPart) string {
	var texts []string
	for _, p := range parts {
		if p.Type == domainmessage.ContentPartText && p.Text != "" {
			texts = append(texts, p.Text)
		}
	}
	return strings.Join(texts, " ")
}

// buildHistoryMessages 构建结构化历史消息列表
func buildHistoryMessages(recorder *eventlog.HistoryRecorder) []domainmessage.Message {
	agentMessages := recorder.GetMessages()
	summaryText, summarizedCount := recorder.GetSummary()

	var messages []domainmessage.Message

	if summaryText != "" && summarizedCount > 0 {
		messages = append(messages, domainmessage.Message{Role: domainmessage.RoleSystem, Content: "## 对话历史摘要\n" + summaryText + "\n\n以上对话均已处理完毕，请仅回答用户当前的最新问题。"})

		// 摘要未覆盖的最近记录
		for i, msg := range agentMessages[summarizedCount:] {
			messages = append(messages, agentMessageToCoreMessages(msg, summarizedCount+i)...)
		}
	} else if len(agentMessages) > 0 {
		for i, msg := range agentMessages {
			messages = append(messages, agentMessageToCoreMessages(msg, i)...)
		}
	}

	return messages
}

// debugContextEnabled 检查是否启用上下文调试日志
func debugContextEnabled() bool {
	return fkenv.Get(fkenv.DebugContext) == "1"
}

// logMessages 打印消息列表摘要
func logMessages(tag string, msgs []domainmessage.Message) {
	totalChars := 0
	for _, m := range msgs {
		totalChars += len(m.Content)
		if m.ReasoningContent != "" {
			totalChars += len(m.ReasoningContent)
		}
	}
	log.Debugf("[%s] 共 %d 条消息, 约 %d 字符", tag, len(msgs), totalChars)
	for i, m := range msgs {
		role := string(m.Role)
		preview := truncatePreview(m.Content, 120)
		extra := ""

		// 工具调用：拆分为独立的 tool_call / tool_result 展示
		if len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				name := tc.Function.Name
				if tc.Function.Arguments != "" {
					name += "(" + tc.Function.Arguments + ")"
				}
				log.Debugf("  [%d] %-10s | %s", i+1, "tool_call", truncatePreview(name, 160))
			}
			continue
		}
		if m.Role == domainmessage.RoleTool {
			log.Debugf("  [%d] %-10s | %s%s", i+1, "tool_result", preview, extra)
			continue
		}

		if m.ReasoningContent != "" {
			extra += fmt.Sprintf(" reasoning=%dchars", len([]rune(m.ReasoningContent)))
		}
		if len(m.ContentParts) > 0 {
			extra += fmt.Sprintf(" multimodal_parts=%d", len(m.ContentParts))
		}
		if m.Name != "" {
			extra += fmt.Sprintf(" name=%s", m.Name)
		}
		log.Debugf("  [%d] %-10s | %s%s", i+1, role, preview, extra)
	}
}

func truncatePreview(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

// agentMessageToCoreMessages 将 AgentMessage 转为结构化消息列表。
// 用户消息 → UserMessage；Agent 消息 → 文本 AssistantMessage + 工具调用拆分为 ToolCall/ToolMessage 对。
func agentMessageToCoreMessages(msg eventlog.AgentMessage, messageIndex int) []domainmessage.Message {
	if msg.AgentName == "用户" {
		var text strings.Builder
		var parts []domainmessage.ContentPart
		for _, event := range msg.Events {
			if event.Type != eventlog.MsgTypeText {
				continue
			}
			text.WriteString(event.Content)
			parts = append(parts, event.ContentParts...)
		}
		content := text.String()
		refs := eventlog.AttachmentsForMessage(msg, messageIndex)
		if notice := omittedContentPartsNotice(parts, refs); notice != "" {
			content = strings.TrimSpace(content)
			if content != "" {
				content += "\n\n"
			}
			content += notice
		}
		return []domainmessage.Message{{Role: domainmessage.RoleUser, Content: content}}
	}

	var messages []domainmessage.Message
	var textBuf strings.Builder
	var reasoningBuf strings.Builder

	flushText := func() {
		content := strings.TrimSpace(textBuf.String())
		reasoning := strings.TrimSpace(reasoningBuf.String())
		textBuf.Reset()
		reasoningBuf.Reset()
		if content == "" && reasoning == "" {
			return
		}
		m := domainmessage.Message{Role: domainmessage.RoleAssistant, Content: content}
		m.Name = msg.AgentName
		if reasoning != "" {
			m.ReasoningContent = reasoning
		}
		messages = append(messages, m)
	}

	for _, event := range msg.Events {
		switch event.Type {
		case eventlog.MsgTypeText:
			textBuf.WriteString(event.Content)

		case eventlog.MsgTypeReasoning:
			reasoningBuf.WriteString(event.Content)

		case eventlog.MsgTypeToolCall:
			tc := event.ToolCall
			if tc == nil {
				continue
			}
			flushText()
			// AssistantMessage 携带 ToolCall
			messages = append(messages, domainmessage.Message{Role: domainmessage.RoleAssistant, ToolCalls: []domainmessage.ToolCall{{
				ID:   tc.ID,
				Type: "function",
				Function: domainmessage.FunctionCall{
					Name:      tc.Name,
					Arguments: tc.Arguments,
				},
			}}})
			// ToolMessage 携带结果
			messages = append(messages, domainmessage.Message{Role: domainmessage.RoleTool, Content: tc.Result, ToolCallID: tc.ID, ToolName: tc.Name})

		case eventlog.MsgTypeAction:
			if event.Action != nil && (event.Action.ActionType != "" || event.Action.Content != "") {
				fmt.Fprintf(&textBuf, "[%s] %s\n", event.Action.ActionType, event.Action.Content)
			}

		case eventlog.MsgTypeError:
			fmt.Fprintf(&textBuf, "[错误] %s\n", event.Content)

		case eventlog.MsgTypeCancelled:
			fmt.Fprintf(&textBuf, "[用户取消] %s\n", cancellationNotice(event.Content))
		}
	}

	flushText()
	return messages
}

func omittedContentPartsNotice(parts []domainmessage.ContentPart, refs []eventlog.AttachmentRef) string {
	counts := map[domainmessage.ContentPartType]int{}
	total := 0
	for _, part := range parts {
		if part.Type == domainmessage.ContentPartText || part.Type == "" {
			continue
		}
		counts[part.Type]++
		total++
	}
	if total == 0 {
		return ""
	}
	var labels []string
	if n := counts[domainmessage.ContentPartImageURL]; n > 0 {
		labels = append(labels, fmt.Sprintf("%d 张图片", n))
	}
	if n := counts[domainmessage.ContentPartAudioURL]; n > 0 {
		labels = append(labels, fmt.Sprintf("%d 段音频", n))
	}
	if n := counts[domainmessage.ContentPartVideoURL]; n > 0 {
		labels = append(labels, fmt.Sprintf("%d 段视频", n))
	}
	if n := counts[domainmessage.ContentPartFileURL]; n > 0 {
		labels = append(labels, fmt.Sprintf("%d 个文件", n))
	}
	if len(labels) == 0 {
		labels = append(labels, fmt.Sprintf("%d 个多模态附件", total))
	}
	var notice strings.Builder
	fmt.Fprintf(&notice, "（历史消息包含 %s，已从当前模型上下文中省略。", strings.Join(labels, "、"))
	if len(refs) > 0 {
		notice.WriteString("如需查看附件，请调用 session_attachment_read，附件 ID：")
		for i, ref := range refs {
			if i > 0 {
				notice.WriteString("、")
			}
			fmt.Fprintf(&notice, "%s(%s)", ref.ID, ref.Part.Type)
		}
	} else {
		notice.WriteString("如需查看附件，请调用 session_attachment_list 或 session_attachment_read。")
	}
	notice.WriteString("）")
	return notice.String()
}

func cancellationNotice(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		content = "任务已取消"
	}
	return content + "。用户刚才取消了上一轮任务；继续对话时不要把上一轮未完成的执行当作已经完成。"
}
