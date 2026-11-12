package handler

import (
	"bytes"
	"encoding/json"
	"fkteams/internal/app/chat/presentation"
	"fkteams/internal/domain/event"
	"fkteams/internal/domain/message"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestPresentationContractFixture 让浏览器更新器逐帧对照 Go 的权威快照。
func TestPresentationContractFixture(t *testing.T) {
	p := presentation.New("contract")
	msg := message.Message{Role: message.RoleAssistant, Content: "corrected plan", ReasoningContent: "thinking", ToolCalls: []message.ToolCall{{ID: "delegate", Function: message.FunctionCall{Name: "ask_fkagent_researcher"}}, {ID: "search", Function: message.FunctionCall{Name: "search"}}}}
	facts := []event.Event{
		{Type: event.TypeUserMessage, MessageID: "user", Content: "research"},
		{Type: event.TypeAssistantStarted, MessageID: "parent", AgentName: "assistant", MessageOrder: 2},
		{Type: event.TypeAssistantReasoning, MessageID: "parent", AgentName: "assistant", BlockID: "parent:thinking", Content: "think"},
		{Type: event.TypeAssistantReasoning, MessageID: "parent", AgentName: "assistant", BlockID: "parent:thinking", Content: "ing"},
		{Type: event.TypeAssistantText, MessageID: "parent", AgentName: "assistant", BlockID: "parent:plan", Content: "draft"},
		{Type: event.TypeAssistantText, MessageID: "parent", AgentName: "assistant", BlockID: "parent:plan", Content: " plan"},
		{Type: event.TypeAssistantCompleted, MessageID: "parent", AgentName: "assistant", Message: &msg},
		{Type: event.TypeAssistantStarted, MessageID: "child", MemberCallID: "delegate", MemberName: "researcher"},
		{Type: event.TypeAssistantText, MessageID: "child", MemberCallID: "delegate", MemberName: "researcher", BlockID: "child:text", Content: "child"},
		{Type: event.TypeAssistantText, MessageID: "child", MemberCallID: "delegate", MemberName: "researcher", BlockID: "child:text", Content: " draft"},
		{Type: event.TypeAskRequested, MemberCallID: "delegate", MemberName: "researcher", Ask: &event.AskPayload{ID: "question", Question: "choose", Options: []string{"one", "two"}, MultiSelect: true}},
		{Type: event.TypeAskAnswered, MemberCallID: "delegate", MemberName: "researcher", Ask: &event.AskPayload{ID: "question", Selected: []string{"one"}}},
		{Type: event.TypeAssistantCompleted, MessageID: "child", MemberCallID: "delegate", MemberName: "researcher", Content: "child final"},
		{Type: event.TypeToolCallCompleted, MessageID: "parent", ToolCallID: "delegate", ToolName: "ask_fkagent_researcher", ToolResult: "child final"},
		{Type: event.TypeToolCallFailed, MessageID: "parent", ToolCallID: "search", ToolName: "search", ToolResult: "failed"},
		{Type: event.TypeApprovalRequested, Detail: "approval", Content: "continue?"},
		{Type: event.TypeApprovalAnswered, Detail: "approval", Approval: &event.ApprovalPayload{Decision: "denied"}},
		{Type: event.TypeAssistantStarted, MessageID: "discarded"},
		{Type: event.TypeAskRequested, Ask: &event.AskPayload{ID: "during-draft", Question: "continue?"}},
		{Type: event.TypeAssistantText, MessageID: "discarded", BlockID: "discarded:text", Content: "temporary draft"},
		{Type: event.TypeAssistantCompleted, MessageID: "discarded"},
		{Type: event.TypeTurnCompleted},
	}
	type frame struct {
		Update   presentation.Update   `json:"update"`
		Snapshot presentation.Snapshot `json:"snapshot"`
	}
	frames := []frame{}
	for i, e := range facts {
		e.EventID = fmt.Sprintf("fixture-%d", i)
		e.Sequence = int64(i + 1)
		e.RunID = "run"
		e.CreatedAt = time.Date(2026, 11, 12, 0, 0, i, 0, time.UTC)
		frames = append(frames, frame{p.Apply(e), p.Snapshot()})
	}
	data, err := json.MarshalIndent(struct {
		Frames []frame `json:"frames"`
	}{frames}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	path := filepath.Join("..", "..", "..", "..", "..", "web", "src", "types", "testdata", "chat-view.json")
	if os.Getenv("FKTEAMS_UPDATE_PROTOCOL_FIXTURE") == "1" {
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved, data) {
		t.Fatal("presentation contract changed; regenerate the shared Go/Bun fixture")
	}
}
