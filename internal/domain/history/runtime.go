package history

import (
	"errors"
	"fkteams/internal/domain/event"
	"fkteams/internal/domain/message"
	"fmt"
)

// ErrUnsupportedTranscriptFormat 表示历史缺少当前展示协议需要的运行事实。
var ErrUnsupportedTranscriptFormat = errors.New("unsupported transcript format")

// RuntimeEvent 从规范消息和原始事件信封恢复事实，不生成新的运行身份或序号。
func (e TranscriptEvent) RuntimeEvent() (event.Event, error) {
	if e.Envelope == nil {
		return event.Event{}, fmt.Errorf("%w: record %s has no event envelope", ErrUnsupportedTranscriptFormat, e.ID)
	}
	result := *e.Envelope
	if e.Usage != nil {
		usage := *e.Usage
		result.Usage = &usage
		result.PromptTokens = usage.PromptTokens
		result.CompletionTokens = usage.CompletionTokens
		result.TotalTokens = usage.TotalTokens
	}
	result.OutputParts = append([]message.OutputPart(nil), result.OutputParts...)
	if e.Type == TranscriptRuntime {
		return result, nil
	}
	if e.Message != nil {
		msg := e.Message.Clone()
		result.Message = &msg
		result.Content, result.ReasoningContent, result.ToolCalls = msg.DisplayText(), msg.ReasoningContent, msg.ToolCalls
		if len(msg.ToolCalls) > 0 {
			result.ToolCallRefs = make(map[int]string, len(msg.ToolCalls))
			for i, call := range msg.ToolCalls {
				index := i
				if call.Index != nil {
					index = *call.Index
				}
				result.ToolCallRefs[index] = "tool_call:" + call.ID
			}
		}
	}
	switch e.Type {
	case TranscriptToolCallStart:
		result.ToolName, result.ToolArgs = e.Name, e.Args
		result.ToolCall = &message.ToolCall{ID: e.CallID, Extra: message.CloneMetadata(e.Extra), Function: message.FunctionCall{Name: e.Name, Arguments: e.Args}}
	case TranscriptToolCallEnd:
		result.ToolResult = e.Result
		if result.Message != nil {
			result.ToolResult = result.Message.Content
			result.ToolName = result.Message.ToolName
		}
		if result.ToolResult == "" {
			result.ToolResult = e.Summary
		}
		result.Content = result.ToolResult
	case TranscriptAskRequested, TranscriptAskAnswered:
		if e.Ask != nil {
			result.Ask = &event.AskPayload{ID: e.Ask.ID, Question: e.Ask.Question, Options: append([]string(nil), e.Ask.Options...), MultiSelect: e.Ask.MultiSelect, Selected: append([]string(nil), e.Ask.Selected...), FreeText: e.Ask.FreeText}
		}
		result.Content = e.Content
	case TranscriptSystemNotice, TranscriptCancelled:
		result.Content = e.Content
	case TranscriptError:
		if e.Error != nil {
			result.Error = e.Error.TechnicalDetail
			if result.Error == "" {
				result.Error = e.Error.Message
			}
		}
	}
	return result, nil
}
