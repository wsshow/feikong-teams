package eventlog

import (
	"fkteams/agentcore"

	"fmt"

	"strings"

	"time"
)

func (h *HistoryRecorder) RecordUserMessage(message agentcore.Message) {
	if message.Role == "" {
		message.Role = agentcore.RoleUser
	}
	if message.Role != agentcore.RoleUser || message.IsEmpty() {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	h.finalizeAllActiveMessages()

	content := message.DisplayText()
	parts := append([]agentcore.ContentPart(nil), message.UserInputMultiContent...)
	if len(parts) == 0 {
		parts = append(parts, message.MultiContent...)
	}

	h.messages = append(h.messages, AgentMessage{
		AgentName: "用户",
		StartTime: time.Now(),
		EndTime:   time.Now(),
		Events: []MessageEvent{
			{Type: MsgTypeText, Content: content, ContentParts: parts},
		},
	})

}

// RecordCancelled 记录用户取消任务事件，并标记当前仍活跃的消息。
func (h *HistoryRecorder) RecordCancelled(message string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if strings.TrimSpace(message) == "" {
		message = "任务已取消"
	}
	cancelEvent := MessageEvent{Type: MsgTypeCancelled, Content: message}
	for _, key := range h.sortedActiveKeysLocked() {
		ctx := h.activeMessages[key]
		if ctx == nil || messageHasEventType(ctx.msg.Events, MsgTypeCancelled) {
			continue
		}
		ctx.msg.Events = append(ctx.msg.Events, cancelEvent)
	}
	h.finalizeAllActiveMessages()
	if len(h.messages) > 0 && h.messages[len(h.messages)-1].AgentName == "系统" && messageHasEventType(h.messages[len(h.messages)-1].Events, MsgTypeCancelled) {
		return
	}
	h.messages = append(h.messages, AgentMessage{
		AgentName: "系统",
		StartTime: time.Now(),
		EndTime:   time.Now(),
		Events: []MessageEvent{
			cancelEvent,
		},
	})
}

func messageHasEventType(events []MessageEvent, typ MsgEventType) bool {
	for _, event := range events {
		if event.Type == typ {
			return true
		}
	}
	return false
}

// FinalizeCurrent 完成当前消息记录，在对话结束时调用
func (h *HistoryRecorder) FinalizeCurrent() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.finalizeAllActiveMessages()
	h.activeMessages = make(map[string]*activeMessageContext)
	h.activeOrder = nil
}

func (h *HistoryRecorder) GetMessages() []AgentMessage {
	h.mu.RLock()
	defer h.mu.RUnlock()
	result := make([]AgentMessage, len(h.messages))
	copy(result, h.messages)
	return result
}

func (h *HistoryRecorder) GetAgentMessages(agentName string) []AgentMessage {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]AgentMessage, 0)
	for _, msg := range h.messages {
		if msg.AgentName == agentName {
			result = append(result, msg)
		}
	}
	return result
}

// GetCurrentMessage 返回当前构建中的 (agentName, textContent)
func (h *HistoryRecorder) GetCurrentMessage() (agentName, content string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var builder strings.Builder
	for _, key := range h.sortedActiveKeysLocked() {
		ctx := h.activeMessages[key]
		if ctx == nil {
			continue
		}
		if agentName == "" {
			agentName = ctx.msg.AgentName
		}
		for _, event := range ctx.msg.Events {
			if event.Type == MsgTypeText {
				builder.WriteString(event.Content)
			}
		}
	}
	if builder.Len() > 0 {
		return agentName, builder.String()
	}
	return "", ""
}

func (h *HistoryRecorder) GetFullHistory() string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	messages := h.snapshotMessagesLocked()
	var result strings.Builder
	for i, msg := range messages {
		if i > 0 {
			result.WriteString("\n\n")
		}
		result.WriteString("=== ")
		result.WriteString(msg.AgentName)
		result.WriteString(" ===\n")
		for _, event := range msg.Events {
			if event.Type == MsgTypeText {
				result.WriteString(event.Content)
			}
		}
	}

	return result.String()
}

func (h *HistoryRecorder) GetConversationSummary() string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var result strings.Builder
	for i, msg := range h.messages {
		duration := msg.EndTime.Sub(msg.StartTime)
		var contentLen int
		for _, event := range msg.Events {
			if event.Type == MsgTypeText {
				contentLen += len([]rune(event.Content))
			}
		}
		fmt.Fprintf(&result, "%d. [%s] %s - %d字 (%v)\n",
			i+1, msg.StartTime.Format("15:04:05"), msg.AgentName, contentLen, duration.Round(time.Millisecond))
	}
	return result.String()
}

func (h *HistoryRecorder) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.messages = make([]AgentMessage, 0)
	h.activeMessages = make(map[string]*activeMessageContext)
	h.activeOrder = nil
	h.summary = ""
	h.summarizedCount = 0
}

// SetSummary 设置上下文压缩摘要
func (h *HistoryRecorder) SetSummary(text string, count int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.summary = text
	h.summarizedCount = count
}

// GetSummary 获取上下文压缩摘要和已覆盖的消息数量
func (h *HistoryRecorder) GetSummary() (string, int) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.summary, h.summarizedCount
}

// reconstructSummaryFromEvents 从事件流中重建上下文压缩摘要状态（需在持锁状态下调用）
func (h *HistoryRecorder) reconstructSummaryFromEvents() {
	h.summary = ""
	h.summarizedCount = 0

	for i := len(h.messages) - 1; i >= 0; i-- {
		for _, evt := range h.messages[i].Events {
			if evt.Type == MsgTypeAction && evt.Action != nil &&
				evt.Action.ActionType == ActionContextCompress && evt.Action.Detail != "" {
				h.summary = evt.Action.Detail

				for j := i - 1; j >= 0; j-- {
					if h.messages[j].AgentName == "用户" {
						h.summarizedCount = j
						return
					}
				}
				h.summarizedCount = 0
				return
			}
		}
	}
}
func (h *HistoryRecorder) GetAgentNames() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	nameMap := make(map[string]bool)
	for _, msg := range h.messages {
		nameMap[msg.AgentName] = true
	}
	for _, ctx := range h.activeMessages {
		if ctx != nil && ctx.msg.AgentName != "" {
			nameMap[ctx.msg.AgentName] = true
		}
	}

	names := make([]string, 0, len(nameMap))
	for name := range nameMap {
		names = append(names, name)
	}
	return names
}

func (h *HistoryRecorder) GetMessageCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.snapshotMessagesLocked())
}
