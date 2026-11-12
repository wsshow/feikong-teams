package session

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"fkteams/internal/domain/apperror"
	domainhistory "fkteams/internal/domain/history"
	domainsession "fkteams/internal/domain/session"
)

type queryReader struct {
	messages map[string][]domainhistory.AgentMessage
	err      error
	calls    int
}

func (r *queryReader) LoadSessionMessages(_ context.Context, id string) ([]domainhistory.AgentMessage, error) {
	r.calls++
	return r.messages[id], r.err
}

func TestQuerySearchesBodyAndPaginatesScannedSessions(t *testing.T) {
	repo := &memoryRepository{items: map[string]domainsession.Metadata{
		"new": {ID: "new", Title: "最近会话", UpdatedAt: time.Unix(3, 0)},
		"old": {ID: "old", Title: "项目讨论", UpdatedAt: time.Unix(2, 0)},
	}}
	reader := &queryReader{messages: map[string][]domainhistory.AgentMessage{
		"old": {{AgentName: "user", Events: []domainhistory.MessageEvent{{Type: domainhistory.MsgTypeText, Content: strings.Repeat("文", 500) + "BUDGET预算"}}}},
	}}
	svc := NewQueryService(repo, reader)
	first, err := svc.Query(context.Background(), QueryRequest{Query: "budget", Limit: 1})
	if err != nil || len(first.Sessions) != 0 || !first.HasMore || first.NextOffset != 1 {
		t.Fatalf("first = %#v, %v", first, err)
	}
	second, err := svc.Query(context.Background(), QueryRequest{Query: "budget", Offset: first.NextOffset, Limit: 1})
	if err != nil || len(second.Sessions) != 1 || second.HasMore {
		t.Fatalf("second = %#v, %v", second, err)
	}
	hit := second.Sessions[0]
	if hit.SessionID != "old" || hit.MessageIndex == nil || *hit.MessageIndex != 0 || !utf8.ValidString(hit.Snippet) || !strings.Contains(hit.Snippet, "BUDGET预算") {
		t.Fatalf("hit = %#v", hit)
	}
	reader.calls = 0
	listed, err := svc.Query(context.Background(), QueryRequest{})
	if err != nil || len(listed.Sessions) != 2 || listed.Sessions[0].SessionID != "new" || reader.calls != 0 {
		t.Fatalf("list = %#v, %v, reads=%d", listed, err, reader.calls)
	}
}

func TestReadBoundsContentAndExcludesInternalPayloads(t *testing.T) {
	repo := &memoryRepository{items: map[string]domainsession.Metadata{"s": {ID: "s", Title: "标题"}}}
	reader := &queryReader{messages: map[string][]domainhistory.AgentMessage{"s": {{AgentName: "user", Events: []domainhistory.MessageEvent{
		{Type: domainhistory.MsgTypeText, Content: strings.Repeat("中", 4100)},
		{Type: domainhistory.MsgTypeReasoning, Content: "internal reasoning"},
		{Type: domainhistory.MsgTypeToolCall, ToolCall: &domainhistory.ToolCallRecord{Result: "raw result"}},
	}}}}}
	svc := NewQueryService(repo, reader)
	page, err := svc.Read(context.Background(), ReadRequest{SessionID: "s", Limit: 1})
	if err != nil || len(page.Messages) != 1 {
		t.Fatalf("page = %#v, %v", page, err)
	}
	m := page.Messages[0]
	if !m.Truncated || len([]rune(m.Content)) != 4000 || m.NextContentOffset != 4000 {
		t.Fatalf("invalid chunk: %#v", m)
	}
	tail, err := svc.Read(context.Background(), ReadRequest{SessionID: "s", Offset: m.Index, ContentOffset: m.NextContentOffset, Limit: 1})
	if err != nil || tail.Messages[0].Truncated || tail.Messages[0].Content != strings.Repeat("中", 100) {
		t.Fatalf("tail = %#v, %v", tail, err)
	}
	for _, id := range []string{"../outside", "/absolute", "a\\b", "missing"} {
		before := reader.calls
		if _, err := svc.Read(context.Background(), ReadRequest{SessionID: id}); err == nil {
			t.Fatalf("accepted %q", id)
		}
		if reader.calls != before {
			t.Fatalf("invalid ID %q reached reader", id)
		}
	}
	if _, err := svc.Read(context.Background(), ReadRequest{SessionID: "s", Offset: -1}); !apperror.IsCode(err, apperror.CodeInvalidArgument) {
		t.Fatalf("negative offset: %v", err)
	}
	reader.err = errors.New("storage unavailable")
	if _, err := svc.Query(context.Background(), QueryRequest{Query: "anything"}); !errors.Is(err, reader.err) {
		t.Fatalf("storage failure: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := svc.Read(ctx, ReadRequest{SessionID: "s"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read: %v", err)
	}
}
