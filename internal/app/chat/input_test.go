package chat

import (
	"fkteams/internal/app/memory"
	domainhistory "fkteams/internal/domain/history"
	domainmessage "fkteams/internal/domain/message"
	"reflect"
	"strings"
	"testing"
)

func TestContextProjectionUsesCanonicalMessagesInsteadOfDisplayFields(t *testing.T) {
	source := domainmessage.Message{Role: domainmessage.RoleAssistant, Content: "original", Extra: map[string]any{"provider": "metadata"}, ToolCalls: []domainmessage.ToolCall{{
		ID: "call", Function: domainmessage.FunctionCall{Name: "lookup", Arguments: `{}`},
	}}}
	result := domainmessage.Message{Role: domainmessage.RoleTool, ToolCallID: "call", ToolName: "lookup", Content: "original result"}
	got := ProjectAgentMessages([]domainhistory.AgentMessage{{AgentName: "assistant", Events: []domainhistory.MessageEvent{
		{Type: domainhistory.MsgTypeText, MessageID: "source", Content: "display text", Message: &source},
		{Type: domainhistory.MsgTypeToolCall, ToolCall: &domainhistory.ToolCallRecord{ID: "display-id", Name: "display-name", Arguments: "display args", Result: "display result"}, Message: &result},
	}}})
	if !reflect.DeepEqual(got, []domainmessage.Message{source, result}) {
		t.Fatalf("display fields affected model history: %#v", got)
	}
	got[0].Extra["provider"] = "changed"
	if source.Extra["provider"] != "metadata" {
		t.Fatal("model context aliases recorded message metadata")
	}
}

type testHistory struct {
	messages        []domainhistory.AgentMessage
	summary         string
	summarizedCount int
}

func (h *testHistory) ProjectContextMessages(offset int) []domainmessage.Message {
	if offset < 0 {
		offset = 0
	}
	if offset > len(h.messages) {
		offset = len(h.messages)
	}
	return ProjectAgentMessages(h.messages[offset:])
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

func TestAgentMessageToCoreMessagesKeepsCoordinatorReasoningAndTools(t *testing.T) {
	msg := domainhistory.AgentMessage{
		AgentName: "coordinator",
		Events: []domainhistory.MessageEvent{
			{Type: domainhistory.MsgTypeReasoning, Content: "think"},
			{Type: domainhistory.MsgTypeText, Content: "before"},
			{Type: domainhistory.MsgTypeToolCall, ToolCall: &domainhistory.ToolCallRecord{
				ID:        "call_1",
				Name:      "fetch",
				Arguments: `{"url":"https://example.com"}`,
				Result:    "result",
			}},
			{Type: domainhistory.MsgTypeText, Content: "after"},
		},
	}

	messages := agentMessageToCoreMessages(msg, 0)
	if len(messages) != 4 {
		t.Fatalf("message count = %d, want 4: %#v", len(messages), messages)
	}
	if messages[0].ReasoningContent != "think" || messages[0].Content != "before" {
		t.Fatalf("first message = %#v, want reasoning and text", messages[0])
	}
	if len(messages[1].ToolCalls) != 1 || messages[1].ToolCalls[0].Function.Name != "fetch" {
		t.Fatalf("tool call message = %#v", messages[1])
	}
	if messages[2].Role != domainmessage.RoleTool || messages[2].Content != "result" {
		t.Fatalf("tool result message = %#v", messages[2])
	}
	if messages[3].Content != "after" {
		t.Fatalf("final text = %#v", messages[3])
	}
}

func TestAgentMessageToCoreMessagesSkipsReasoningOnlyAssistantOutput(t *testing.T) {
	msg := domainhistory.AgentMessage{
		AgentName: "assistant",
		Events: []domainhistory.MessageEvent{
			{Type: domainhistory.MsgTypeReasoning, Content: "thinking without final output"},
		},
	}

	messages := agentMessageToCoreMessages(msg, 0)
	if len(messages) != 0 {
		t.Fatalf("message count = %d, want 0: %#v", len(messages), messages)
	}
}

func TestAgentMessageToCoreMessagesKeepsOnlyMemberFinalText(t *testing.T) {
	msg := domainhistory.AgentMessage{
		AgentName:      "researcher",
		MemberCallID:   "call_1",
		MemberToolName: "ask_fkagent_researcher",
		MemberName:     "researcher",
		Events: []domainhistory.MessageEvent{
			{Type: domainhistory.MsgTypeReasoning, Content: "member thinking"},
			{Type: domainhistory.MsgTypeToolCall, ToolCall: &domainhistory.ToolCallRecord{
				ID:     "call_inner",
				Name:   "search",
				Result: "raw search result",
			}},
			{Type: domainhistory.MsgTypeText, Content: "final member answer"},
		},
	}

	messages := agentMessageToCoreMessages(msg, 0)
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want 1: %#v", len(messages), messages)
	}
	if messages[0].Content != "final member answer" {
		t.Fatalf("content = %q, want final member answer", messages[0].Content)
	}
	if messages[0].ReasoningContent != "" || len(messages[0].ToolCalls) != 0 {
		t.Fatalf("member context leaked internals: %#v", messages[0])
	}
}
