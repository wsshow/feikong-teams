package message

import (
	"fmt"
	"strings"
)

// OutputPart 保存模型输出分段的位置；正文仍只保存在规范消息中。
type OutputPart struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Start      int    `json:"start,omitempty"`
	End        int    `json:"end,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// OutputLayout 在观察增量时记录有序分段，不重复持久化消息正文。
type OutputLayout struct {
	textBuffer      *strings.Builder
	reasoningBuffer *strings.Builder
	Parts           []OutputPart
	Text            string
	Reasoning       string
}

// Append 将连续同类增量归入同一个稳定分段。
func (l *OutputLayout) Append(messageID, kind, content, callID string) OutputPart {
	if l.textBuffer == nil {
		l.textBuffer = &strings.Builder{}
	}
	if l.reasoningBuffer == nil {
		l.reasoningBuffer = &strings.Builder{}
	}
	if kind == "tool" {
		for _, part := range l.Parts {
			if part.ToolCallID == callID {
				return part
			}
		}
		part := OutputPart{ID: messageID + ":tool:" + callID, Type: kind, ToolCallID: callID}
		l.Parts = append(l.Parts, part)
		return part
	}
	start := len(l.Text)
	if kind == "reasoning" {
		start = len(l.Reasoning)
		l.reasoningBuffer.WriteString(content)
		l.Reasoning = l.reasoningBuffer.String()
	} else {
		l.textBuffer.WriteString(content)
		l.Text = l.textBuffer.String()
	}
	if n := len(l.Parts); n > 0 && l.Parts[n-1].Type == kind {
		l.Parts[n-1].End += len(content)
		return l.Parts[n-1]
	}
	part := OutputPart{ID: fmt.Sprintf("%s:part:%d", messageID, len(l.Parts)), Type: kind, Start: start, End: start + len(content)}
	l.Parts = append(l.Parts, part)
	return part
}

// Complete 以最终消息校正草稿；被改写的正文保留首个分段身份并整体替换。
func (l *OutputLayout) Complete(messageID string, msg Message) []OutputPart {
	parts := append([]OutputPart(nil), l.Parts...)
	for _, field := range []struct{ kind, draft, final string }{{"reasoning", l.Reasoning, msg.ReasoningContent}, {"text", l.Text, msg.DisplayText()}} {
		if field.draft == field.final {
			continue
		}
		var updated []OutputPart
		found := false
		for _, part := range parts {
			if part.Type != field.kind {
				updated = append(updated, part)
				continue
			}
			if !found && field.final != "" {
				part.Start, part.End = 0, len(field.final)
				updated = append(updated, part)
			}
			found = true
		}
		if !found && field.final != "" {
			part := OutputPart{ID: messageID + ":" + field.kind, Type: field.kind, End: len(field.final)}
			index := 0
			if field.kind == "text" {
				for index < len(updated) && updated[index].Type == "reasoning" {
					index++
				}
			}
			updated = append(updated, OutputPart{})
			copy(updated[index+1:], updated[index:])
			updated[index] = part
		}
		parts = updated
	}
	validCalls := make(map[string]bool, len(msg.ToolCalls))
	for _, call := range msg.ToolCalls {
		validCalls[call.ID] = true
	}
	filtered := parts[:0]
	for _, part := range parts {
		if part.Type != "tool" || validCalls[part.ToolCallID] {
			filtered = append(filtered, part)
			delete(validCalls, part.ToolCallID)
		}
	}
	parts = filtered
	for _, call := range msg.ToolCalls {
		if validCalls[call.ID] {
			parts = append(parts, OutputPart{ID: messageID + ":tool:" + call.ID, Type: "tool", ToolCallID: call.ID})
		}
	}
	return parts
}
