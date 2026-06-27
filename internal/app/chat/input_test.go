package chat

import (
	"fkteams/internal/app/memory"
	domainhistory "fkteams/internal/domain/history"
	domainmessage "fkteams/internal/domain/message"
	"strings"
	"testing"
)

type testHistory struct {
	messages        []domainhistory.AgentMessage
	summary         string
	summarizedCount int
}

func (h *testHistory) GetMessages() []domainhistory.AgentMessage {
	return h.messages
}

func (h *testHistory) GetSummary() (string, int) {
	return h.summary, h.summarizedCount
}

func TestBuildTurnInputReturnsTurnInput(t *testing.T) {
	recorder := &testHistory{}

	input := BuildTurnInput(recorder, "hello")

	if input.Message.DisplayText() != "hello" {
		t.Fatalf("input message = %q, want hello", input.Message.DisplayText())
	}
	messages := input.AllMessages()
	if len(messages) == 0 || messages[len(messages)-1].Content != "hello" {
		t.Fatalf("messages = %#v, want final user message", messages)
	}
}

func TestBuildTurnInputWithMemoryInjectsMemoryContext(t *testing.T) {
	recorder := &testHistory{}
	manager := &testMemoryManager{
		entries: []memory.MemoryEntry{{
			Type:    memory.Preference,
			Summary: "用户偏好中文回复",
			Detail:  "回答需要简洁明确",
		}},
	}

	input := BuildTurnInputWithMemory(recorder, "hello", manager)

	if manager.query != "hello" || manager.topK != 5 {
		t.Fatalf("search query = %q/%d, want hello/5", manager.query, manager.topK)
	}
	if len(input.Context) == 0 || input.Context[0].Role != domainmessage.RoleSystem {
		t.Fatalf("context = %#v, want memory system message", input.Context)
	}
	if !strings.Contains(input.Context[0].Content, "用户偏好中文回复") {
		t.Fatalf("memory context = %q, want injected summary", input.Context[0].Content)
	}
}

func TestBuildMultimodalTurnInputReturnsDisplayText(t *testing.T) {
	recorder := &testHistory{}
	parts := []domainmessage.ContentPart{TextPart("describe this")}

	input := BuildMultimodalTurnInput(recorder, "describe this", parts)

	if input.Message.DisplayText() != "describe this" {
		t.Fatalf("input message = %q, want display text", input.Message.DisplayText())
	}
	messages := input.AllMessages()
	if len(messages) == 0 || len(messages[len(messages)-1].ContentParts) != 1 {
		t.Fatalf("messages = %#v, want multimodal user message", messages)
	}
}

type testMemoryManager struct {
	query   string
	topK    int
	entries []memory.MemoryEntry
}

func (m *testMemoryManager) Search(query string, topK int) []memory.MemoryEntry {
	m.query = query
	m.topK = topK
	return m.entries
}

func TestHistoryRecorderOmitMultimodalUserInputFromModelContext(t *testing.T) {
	parts := []domainmessage.ContentPart{
		TextPart("describe this"),
		ImageURLPart("https://example.com/a.png", "high"),
	}
	recorder := &testHistory{
		messages: []domainhistory.AgentMessage{{
			AgentName: "user",
			Events: []domainhistory.MessageEvent{{
				Type:         domainhistory.MsgTypeText,
				Content:      "describe this",
				ContentParts: parts,
			}},
		}},
	}

	input := BuildTurnInput(recorder, "continue")
	if len(input.Context) == 0 {
		t.Fatal("expected history context")
	}
	historyMessage := input.Context[0]
	if historyMessage.Role != domainmessage.RoleUser {
		t.Fatalf("history role = %q, want user", historyMessage.Role)
	}
	if len(historyMessage.ContentParts) != 0 {
		t.Fatalf("history parts = %#v, want omitted multimodal parts", historyMessage.ContentParts)
	}
	if !strings.Contains(historyMessage.Content, "describe this") {
		t.Fatalf("history content = %q, want original text", historyMessage.Content)
	}
	if !strings.Contains(historyMessage.Content, "历史消息包含 1 张图片") {
		t.Fatalf("history content = %q, want omitted image notice", historyMessage.Content)
	}
	if !strings.Contains(historyMessage.Content, "history:000000:00:01") {
		t.Fatalf("history content = %q, want attachment id", historyMessage.Content)
	}
	if !strings.Contains(historyMessage.Content, "session_attachment_read") {
		t.Fatalf("history content = %q, want read tool hint", historyMessage.Content)
	}
}

func TestAgentMessageToSchemaMessagesIncludesCancellationNotice(t *testing.T) {
	msg := domainhistory.AgentMessage{
		AgentName: "系统",
		Events: []domainhistory.MessageEvent{
			{Type: domainhistory.MsgTypeCancelled, Content: "任务已取消"},
		},
	}

	messages := agentMessageToCoreMessages(msg, 0)
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(messages))
	}
	if messages[0].Role != domainmessage.RoleAssistant {
		t.Fatalf("role = %q, want %q", messages[0].Role, domainmessage.RoleAssistant)
	}
	if !strings.Contains(messages[0].Content, "用户刚才取消了上一轮任务") {
		t.Fatalf("content = %q, want cancellation notice", messages[0].Content)
	}
}

func TestAgentMessageToSchemaMessagesMarksCancelledAssistantOutput(t *testing.T) {
	msg := domainhistory.AgentMessage{
		AgentName: "assistant",
		Events: []domainhistory.MessageEvent{
			{Type: domainhistory.MsgTypeText, Content: "处理中"},
			{Type: domainhistory.MsgTypeCancelled, Content: "任务已取消"},
		},
	}

	messages := agentMessageToCoreMessages(msg, 0)
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(messages))
	}
	if messages[0].Role != domainmessage.RoleAssistant {
		t.Fatalf("role = %q, want %q", messages[0].Role, domainmessage.RoleAssistant)
	}
	if !strings.Contains(messages[0].Content, "[用户取消]") {
		t.Fatalf("content = %q, want cancellation marker", messages[0].Content)
	}
}
