package eventlog

import (
	domainevent "fkteams/internal/domain/event"
	"fkteams/internal/domain/message"
	"fkteams/internal/runtime/events"
	"fmt"
)

func cloneRuntimeEvent(event Event) *Event {
	copy := event
	copy.OutputParts = append([]message.OutputPart(nil), event.OutputParts...)
	if event.Message != nil {
		msg := event.Message.Clone()
		copy.Message = &msg
	}
	if event.ToolCall != nil {
		calls := message.Message{ToolCalls: []message.ToolCall{*event.ToolCall}}
		call := calls.Clone().ToolCalls[0]
		copy.ToolCall = &call
	}
	copy.ToolCalls = (message.Message{ToolCalls: event.ToolCalls}).Clone().ToolCalls
	if event.ToolCallRefs != nil {
		copy.ToolCallRefs = map[int]string{}
		for key, value := range event.ToolCallRefs {
			copy.ToolCallRefs[key] = value
		}
	}
	if event.Ask != nil {
		ask := *event.Ask
		ask.Options = append([]string{}, ask.Options...)
		ask.Selected = append([]string{}, ask.Selected...)
		copy.Ask = &ask
	}
	if event.Approval != nil {
		approval := *event.Approval
		copy.Approval = &approval
	}
	if event.Usage != nil {
		usage := *event.Usage
		copy.Usage = &usage
	}
	if event.Notice != nil {
		notice := *event.Notice
		copy.Notice = &notice
	}
	return &copy
}

func transcriptEnvelope(event Event) *Event {
	event.Content, event.ReasoningContent, event.ToolArgs, event.ToolResult, event.Error = "", "", "", "", ""
	event.Message, event.ToolCall, event.ToolCalls, event.Ask = nil, nil, nil, nil
	event.OutputParts = append([]message.OutputPart(nil), event.OutputParts...)
	return cloneRuntimeEvent(event)
}

// SequenceEvent 为展示和持久化分配同一个会话顺序，进程重启后从已保存位置继续。
func (h *HistoryRecorder) SequenceEvent(event Event) Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sequenceEventLocked(event)
}
func (h *HistoryRecorder) sequenceEventLocked(event Event) Event {
	if event.ToolStatus == "" && event.Type == domainevent.TypeToolCallCompleted {
		result := event.ToolResult
		if result == "" {
			result = event.Content
		}
		event.ToolStatus = domainevent.ToolResultStatus(result)
	}
	if event.Type == domainevent.TypeToolCallFailed {
		event.ToolStatus = domainevent.ToolError
	}

	if event.SessionSequence == 0 {
		h.nextSequence++
		event.SessionSequence = h.nextSequence
	}
	if h.messageOrders == nil {
		h.messageOrders = map[string]int64{}
	}
	if event.MessageID != "" {
		if h.messageOrders[event.MessageID] == 0 {
			h.messageOrders[event.MessageID] = event.SessionSequence
		}
		event.MessageOrder = h.messageOrders[event.MessageID]
	}
	return event
}

// RecordAndEmit 串行完成会话排序、记录和发布，成员交互不会越过其他运行事实。
func (h *HistoryRecorder) RecordAndEmit(event Event, sink func(Event) error) error {
	h.eventMu.Lock()
	defer h.eventMu.Unlock()
	event = h.SequenceEvent(events.NormalizeEvent(event))
	h.RecordEvent(event)
	h.mu.RLock()
	persistErr := h.persistErr
	h.mu.RUnlock()
	if persistErr != nil {
		return fmt.Errorf("persist conversation event: %w", persistErr)
	}
	if sink != nil {
		return sink(event)
	}
	return nil
}
