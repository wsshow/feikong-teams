package eventlog

import (
	domainmessage "fkteams/internal/domain/message"
	"testing"
)

func TestConvertMemoryMessages(t *testing.T) {
	recorder := NewHistoryRecorder()
	recorder.RecordUserMessage(domainmessage.Message{Role: domainmessage.RoleUser, Content: "用户消息"})
	recorder.RecordEvent(Event{Type: EventAssistantText, AgentName: "assistant", Content: "助手回复"})
	recorder.FinalizeCurrent()

	messages := ConvertMemoryMessages(recorder)
	if len(messages) != 2 {
		t.Fatalf("messages = %#v, want 2", messages)
	}
	if messages[0].Role != "user" || messages[0].Content != "用户消息" {
		t.Fatalf("user message = %#v", messages[0])
	}
	if messages[1].Role != "assistant" || messages[1].Content != "助手回复" {
		t.Fatalf("assistant message = %#v", messages[1])
	}
}
