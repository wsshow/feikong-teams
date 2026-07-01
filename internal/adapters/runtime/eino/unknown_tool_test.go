package eino

import (
	"context"
	"testing"

	domainevent "fkteams/internal/domain/event"
	domainmessage "fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	runtimeevents "fkteams/internal/runtime/events"
)

func TestRecordUnknownToolResultKeepsMemberScopeFromContext(t *testing.T) {
	recorder := newUnknownToolRecorder()
	ctx := withUnknownToolRecorder(context.Background(), recorder)
	ctx = runtimeport.WithInterruptMetadata(ctx, runtimeport.InterruptMetadata{
		MemberCallID:   "member-call-1",
		MemberToolName: "ask_fkagent_researcher",
		MemberName:     "研究员",
	})

	recordUnknownToolResult(ctx, unknownToolReport{
		AgentName:  "researcher",
		ToolName:   "search",
		ToolResult: "ok",
	})

	reports := recorder.take()
	if len(reports) != 1 {
		t.Fatalf("reports = %d, want 1", len(reports))
	}
	if reports[0].Scope.CallID != "member-call-1" || reports[0].Scope.ToolName != "ask_fkagent_researcher" || reports[0].Scope.Name != "研究员" {
		t.Fatalf("scope = %#v", reports[0].Scope)
	}
}

func TestFlushUnknownToolReportsEmitsMemberScopedToolResult(t *testing.T) {
	var got []domainevent.Event
	emitter := runtimeevents.NewEmitter("run-1", "turn-1", func(event domainevent.Event) error {
		got = append(got, event)
		return nil
	})
	recorder := newUnknownToolRecorder()
	converter := newConverter(emitter, recorder)
	scope := MemberScope{
		CallID:   "member-call-1",
		ToolName: "ask_fkagent_researcher",
		Name:     "研究员",
	}
	toolCall := domainmessage.ToolCall{
		ID:    "tool-call-1",
		Index: intPtr(0),
		Type:  "function",
		Function: domainmessage.FunctionCall{
			Name:      "search",
			Arguments: `{"query":"ai"}`,
		},
	}
	ref := converter.identities.ensure("msg-1", 0, scope, &toolCall)
	converter.identities.rememberResult("search", "tool-call-1", scope)
	recorder.add(unknownToolReport{
		AgentName:  "researcher",
		ToolCallID: "tool-call-1",
		ToolName:   "search",
		ToolArgs:   `{"query":"ai"}`,
		ToolResult: "result",
		Scope:      scope,
	})

	if err := converter.flushUnknownToolReports(); err != nil {
		t.Fatalf("flush reports: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("events = %#v", got)
	}
	event := got[0]
	if event.Type != domainevent.TypeToolCallCompleted || event.ToolCallRef != ref || event.ToolCallID != "tool-call-1" {
		t.Fatalf("tool identity event = %#v, want ref %q", event, ref)
	}
	if event.MemberCallID != "member-call-1" || event.ParentToolCallID != "member-call-1" || event.MemberName != "研究员" {
		t.Fatalf("member scope event = %#v", event)
	}
}

func TestFlushUnknownToolReportsKeepsParallelMemberScopes(t *testing.T) {
	var got []domainevent.Event
	emitter := runtimeevents.NewEmitter("run-1", "turn-1", func(event domainevent.Event) error {
		got = append(got, event)
		return nil
	})
	recorder := newUnknownToolRecorder()
	converter := newConverter(emitter, recorder)
	scopes := []MemberScope{
		{CallID: "member-call-ai", ToolName: "ask_fkagent_researcher", Name: "研究员"},
		{CallID: "member-call-tech", ToolName: "ask_fkagent_researcher", Name: "研究员"},
	}
	for i, scope := range scopes {
		toolID := "tool-call-" + scope.CallID
		toolCall := domainmessage.ToolCall{
			ID:    toolID,
			Index: intPtr(0),
			Type:  "function",
			Function: domainmessage.FunctionCall{
				Name:      "search",
				Arguments: `{"query":"x"}`,
			},
		}
		converter.identities.ensure("msg-"+scope.CallID, i, scope, &toolCall)
		converter.identities.rememberResult("search", toolID, scope)
		recorder.add(unknownToolReport{
			AgentName:  "researcher",
			ToolCallID: toolID,
			ToolName:   "search",
			ToolArgs:   `{"query":"x"}`,
			ToolResult: "result",
			Scope:      scope,
		})
	}

	if err := converter.flushUnknownToolReports(); err != nil {
		t.Fatalf("flush reports: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("events = %#v", got)
	}
	seen := map[string]bool{}
	for _, event := range got {
		seen[event.MemberCallID] = true
		if event.MemberCallID == "" || event.ParentToolCallID != event.MemberCallID || event.ToolName != "search" {
			t.Fatalf("parallel scoped event = %#v", event)
		}
	}
	for _, scope := range scopes {
		if !seen[scope.CallID] {
			t.Fatalf("missing event for scope %q: %#v", scope.CallID, got)
		}
	}
}
