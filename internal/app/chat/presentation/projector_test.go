package presentation

import (
	"encoding/json"
	"fkteams/internal/domain/event"
	"fkteams/internal/domain/message"
	"reflect"
	"testing"
	"time"
)

func TestFinalMessageReplacesDraftAndRejectsLateDeltas(t *testing.T) {
	p := New("session")
	p.Apply(event.Event{Type: event.TypeAssistantText, EventID: "delta", Sequence: 1, MessageID: "m", BlockID: "part", Content: "draft"})
	final := event.Event{Type: event.TypeAssistantCompleted, EventID: "final", Sequence: 2, MessageID: "m", Content: "corrected"}
	p.Apply(final)
	p.Apply(event.Event{Type: event.TypeAssistantText, EventID: "late", Sequence: 3, MessageID: "m", Content: "duplicate"})
	if got := p.Snapshot().Items[0].Node.Parts; len(got) != 1 || got[0].Content != "corrected" || got[0].Key != "part" {
		t.Fatalf("parts = %#v", got)
	}
	before := p.Snapshot()
	p.Apply(final)
	if !reflect.DeepEqual(before, p.Snapshot()) {
		t.Fatal("duplicate completion changed the view")
	}
}
func TestOrderedPartsAndParallelToolsSurviveCompletion(t *testing.T) {
	p := New("session")
	var layout message.OutputLayout
	for i, delta := range []struct{ kind, content string }{{"reasoning", "first"}, {"text", "plan"}, {"reasoning", "second"}} {
		part := layout.Append("m", delta.kind, delta.content, "")
		typ := event.TypeAssistantText
		if delta.kind == "reasoning" {
			typ = event.TypeAssistantReasoning
		}
		p.Apply(event.Event{Type: typ, EventID: part.ID, Sequence: int64(i + 1), MessageID: "m", BlockID: part.ID, Content: delta.content})
	}
	msg := message.Message{Content: "plan", ReasoningContent: "firstsecond", ToolCalls: []message.ToolCall{{ID: "one", Function: message.FunctionCall{Name: "search"}}, {ID: "two", Function: message.FunctionCall{Name: "search"}}}}
	p.Apply(event.Event{Type: event.TypeAssistantCompleted, EventID: "final", Sequence: 4, MessageID: "m", Message: &msg, OutputParts: layout.Complete("m", msg)})
	p.Apply(event.Event{Type: event.TypeToolCallCompleted, EventID: "two", Sequence: 5, MessageID: "m", ToolCallID: "two", ToolName: "search", ToolResult: "second result"})
	parts := p.Snapshot().Items[0].Node.Parts
	if len(parts) != 5 || parts[0].Content != "first" || parts[1].Content != "plan" || parts[2].Content != "second" || parts[3].Tool.ID != "one" || parts[4].Tool.Result != "second result" {
		t.Fatalf("parts = %#v", parts)
	}
	if parts[3].Tool.Status != "pending" || parts[4].Tool.Status != "completed" {
		t.Fatal("parallel call states were conflated")
	}
}
func TestAppendFramesAndSnapshotsAreIndependent(t *testing.T) {
	p := New("session")
	e := event.Event{Type: event.TypeAssistantText, MessageID: "m", BlockID: "part", Content: "a", EventID: "1", Sequence: 1}
	first := p.Apply(e)
	e.Content, e.EventID, e.Sequence = "b", "2", 2
	second := p.Apply(e)
	if len(second.Appends) != 1 || second.Appends[0].Content != "b" || len(second.Items) != 0 {
		t.Fatalf("append=%#v", second)
	}
	if first.Items[0].Node.Parts[0].Content != "a" {
		t.Fatal("published frame changed after next delta")
	}
	snapshot := p.Snapshot()
	snapshot.Items[0].Node.Parts[0].Content = "mutated"
	if p.Snapshot().Items[0].Node.Parts[0].Content != "ab" {
		t.Fatal("snapshot leaked mutable projection")
	}
}
func TestToolTerminalStatusAndEmptySuccessAreExplicit(t *testing.T) {
	for _, tc := range []struct {
		result, status string
		typ            event.Type
	}{{"", "completed", event.TypeToolCallCompleted}, {`{"success":false}`, "error", event.TypeToolCallCompleted}, {"error", "error", event.TypeToolCallFailed}} {
		p := New("s")
		p.Apply(event.Event{Type: tc.typ, EventID: "end", MessageID: "m", ToolCallID: "call", ToolName: "execute", Content: tc.result, Sequence: 1})
		if got := p.Snapshot().Items[0].Node.Parts[0].Tool.Status; got != tc.status {
			t.Fatalf("status=%s", got)
		}
	}
	p := New("s")
	p.Apply(event.Event{Type: event.TypeToolCallStarted, EventID: "start", RunID: "run", MessageID: "m", ToolCallID: "call", ToolName: "execute", Sequence: 1})
	p.Apply(event.Event{Type: event.TypeTurnCompleted, EventID: "end", RunID: "run", Sequence: 2})
	if got := p.Snapshot().Items[0].Node.Parts[0].Tool.Status; got != "interrupted" {
		t.Fatalf("unfinished call was reported successful: %s", got)
	}
}
func TestApprovalsAndQuestionsKeepTheirOwnIdentities(t *testing.T) {
	p := New("s")
	p.Apply(event.Event{Type: event.TypeApprovalRequested, EventID: "request", Detail: "approval", Content: "run?", Sequence: 1})
	p.Apply(event.Event{Type: event.TypeApprovalAnswered, EventID: "decision", Detail: "approval", Approval: &event.ApprovalPayload{Decision: "denied"}, Sequence: 2})
	p.Apply(event.Event{Type: event.TypeAskRequested, EventID: "ask", Ask: &event.AskPayload{ID: "question", Question: "which?"}, Sequence: 3})
	p.Apply(event.Event{Type: event.TypeAskAnswered, EventID: "answer", Ask: &event.AskPayload{ID: "question", Selected: []string{"one"}}, Sequence: 4})
	view := p.Snapshot()
	if !view.Items[0].Approval.Answered || view.Items[0].Approval.Decision != "denied" || !view.Items[1].Ask.Answered || len(view.Items[1].Ask.Selected) != 1 {
		t.Fatalf("view=%#v", view)
	}
	data, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var restored Snapshot
	if err = json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
}
func TestSessionSequenceControlsOrderAcrossRuns(t *testing.T) {
	p := New("s")
	p.Apply(event.Event{Type: event.TypeUserMessage, EventID: "old", MessageID: "old", Sequence: 100, SessionSequence: 1, CreatedAt: time.Unix(20, 0), Content: "old"})
	p.Apply(event.Event{Type: event.TypeUserMessage, EventID: "new", MessageID: "new", Sequence: 1, SessionSequence: 2, CreatedAt: time.Unix(10, 0), Content: "new"})
	if got := p.Snapshot().Items; got[0].ID != "old" || got[1].ID != "new" {
		t.Fatalf("order=%#v", got)
	}
}

func TestMemberQuestionKeepsItsPlaceBetweenModelMessages(t *testing.T) {
	p := New("s")
	p.Apply(event.Event{Type: event.TypeAssistantCompleted, EventID: "parent", MessageID: "parent", Sequence: 1, ToolCalls: []message.ToolCall{{ID: "member", Function: message.FunctionCall{Name: "ask_fkagent_member"}}}})
	p.Apply(event.Event{Type: event.TypeAssistantCompleted, EventID: "before", MessageID: "before", Sequence: 2, MemberCallID: "member", MemberName: "member", Content: "before"})
	p.Apply(event.Event{Type: event.TypeAskRequested, EventID: "ask", Sequence: 3, MemberCallID: "member", Ask: &event.AskPayload{ID: "question", Question: "which?"}})
	p.Apply(event.Event{Type: event.TypeAssistantCompleted, EventID: "after", MessageID: "after", Sequence: 4, MemberCallID: "member", MemberName: "member", Content: "after"})
	parts := p.Snapshot().Items[0].Node.Parts[0].Member.Parts
	if len(parts) != 3 || parts[0].Content != "before" || parts[1].Type != "ask" || parts[2].Content != "after" {
		t.Fatalf("member parts=%#v", parts)
	}
}

func TestClosedRunDoesNotReactivateEarlierInteractions(t *testing.T) {
	p := New("s")
	p.Apply(event.Event{Type: event.TypeAskRequested, EventID: "ask", RunID: "old", Sequence: 1, Ask: &event.AskPayload{ID: "question", Question: "which?"}})
	p.Apply(event.Event{Type: event.TypeApprovalRequested, EventID: "approval", RunID: "old", Sequence: 2, Approval: &event.ApprovalPayload{ID: "permission", Message: "run?"}})
	p.Apply(event.Event{Type: event.TypeTurnCancelled, EventID: "end", RunID: "old", Sequence: 3})
	p.Apply(event.Event{Type: event.TypeUserMessage, EventID: "next", MessageID: "next", RunID: "new", Sequence: 4, Content: "continue"})
	view := p.Snapshot()
	if view.Items[0].Ask.Active || view.Items[1].Approval.Active || view.Items[0].Ask.Answered || view.Items[1].Approval.Answered {
		t.Fatalf("closed interactions=%#v", view)
	}
}

func TestCompletedQuestionToolsRecoverReadOnlyHistoryWithoutDuplicateCards(t *testing.T) {
	for _, structured := range []bool{false, true} {
		p := New("session")
		for i, id := range []string{"first", "second"} {
			seq := int64(i*10 + 1)
			p.Apply(event.Event{Type: event.TypeToolCallStarted, EventID: id + "-start", RunID: "run", MessageID: id, Sequence: seq, ToolCallID: id, ToolName: "ask_questions", ToolArgs: `{"question":"same question","options":["A","B"],"multi_select":true}`})
			if structured {
				p.Apply(event.Event{Type: event.TypeAskRequested, EventID: id + "-ask", RunID: "run", Sequence: seq + 1, ToolCallID: id, Ask: &event.AskPayload{ID: id + "-ask", Question: "same question", Options: []string{"A", "B"}, MultiSelect: true}})
				p.Apply(event.Event{Type: event.TypeAskAnswered, EventID: id + "-answer", RunID: "run", Sequence: seq + 2, ToolCallID: id, Ask: &event.AskPayload{ID: id + "-ask", Selected: []string{"B"}, FreeText: "details"}})
			}
			p.Apply(event.Event{Type: event.TypeToolCallCompleted, EventID: id + "-end", RunID: "run", Sequence: seq + 3, ToolCallID: id, ToolName: "ask_questions", ToolResult: `{"selected":["B"],"free_text":"details"}`})
		}
		got := p.Snapshot().Items
		if len(got) != 2 {
			t.Fatalf("structured=%v items=%+v", structured, got)
		}
		for _, item := range got {
			if item.Kind != "ask" || item.Ask.Active || !item.Ask.Answered || !item.Ask.MultiSelect || len(item.Ask.Selected) != 1 || item.Ask.Selected[0] != "B" || item.Ask.FreeText != "details" {
				t.Fatalf("card=%+v", item)
			}
		}
	}
}

func TestQuestionRecoveryDoesNotInventAnswersForFailedTools(t *testing.T) {
	for _, result := range []string{`{}`, `{"error_message":"cancelled"}`, `invalid`} {
		p := New("session")
		p.Apply(event.Event{Type: event.TypeToolCallStarted, EventID: "start", Sequence: 1, ToolCallID: "ask", ToolName: "ask_questions", ToolArgs: `{"question":"choose?","options":["A","B"]}`})
		p.Apply(event.Event{Type: event.TypeToolCallCompleted, EventID: "end", Sequence: 2, ToolCallID: "ask", ToolName: "ask_questions", ToolResult: result})
		for _, item := range p.Snapshot().Items {
			if item.Ask != nil && (item.Ask.Answered || item.Ask.Active) {
				t.Fatalf("invented answer: %+v", item.Ask)
			}
		}
	}
}
