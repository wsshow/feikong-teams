package eventlog

import domainmemory "fkteams/internal/domain/memory"

// ConvertMemoryMessages 将历史记录转换为长期记忆提取消息。
func ConvertMemoryMessages(recorder *HistoryRecorder) []domainmemory.Message {
	recorder.FinalizeCurrent()
	agentMessages := recorder.GetMessages()
	var msgs []domainmemory.Message
	for _, am := range agentMessages {
		role := "assistant"
		if isUserAgentName(am.AgentName) {
			role = "user"
		}
		content := am.GetTextContent()
		if content != "" {
			msgs = append(msgs, domainmemory.Message{Role: role, Content: content})
		}
	}
	return msgs
}
