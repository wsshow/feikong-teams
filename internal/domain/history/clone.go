package history

import "fkteams/internal/domain/message"

// Clone 返回独立历史快照，读取方修改投影不会影响已记录的消息事实。
func (m AgentMessage) Clone() AgentMessage {
	if m.Events == nil {
		return m
	}
	m.Events = append([]MessageEvent{}, m.Events...)
	for i := range m.Events {
		e := &m.Events[i]
		e.ContentParts = (message.Message{ContentParts: e.ContentParts}).Clone().ContentParts
		if e.Message != nil {
			msg := e.Message.Clone()
			e.Message = &msg
		}
		if e.ToolCall != nil {
			call := *e.ToolCall
			call.Extra = message.CloneMetadata(call.Extra)
			if call.Index != nil {
				index := *call.Index
				call.Index = &index
			}
			e.ToolCall = &call
		}
		if e.Usage != nil {
			usage := *e.Usage
			e.Usage = &usage
		}
		if e.Ask != nil {
			ask := *e.Ask
			ask.Options = append([]string(nil), ask.Options...)
			ask.Selected = append([]string(nil), ask.Selected...)
			e.Ask = &ask
		}
		if e.Error != nil {
			err := *e.Error
			err.Suggestions = append([]string(nil), err.Suggestions...)
			e.Error = &err
		}
	}
	return m
}
