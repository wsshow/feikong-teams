package session

import (
	"context"
	"strings"
	"testing"

	domainsession "fkteams/internal/domain/session"
)

func TestAssistantOpenAndRename(t *testing.T) {
	repo := &memoryRepository{items: map[string]domainsession.Metadata{"other": {ID: "other", Title: "other"}}}
	svc := NewService(repo)
	ctx := context.Background()
	meta, err := svc.OpenAssistant(ctx)
	if err != nil || meta.ID != domainsession.AssistantID || meta.Title != "助手" || meta.CurrentAgent != domainsession.AssistantAgent {
		t.Fatalf("open: %#v, %v", meta, err)
	}
	for _, name := range []string{"", "  ", strings.Repeat("名", 41), "bad\nname", "bad\x00name"} {
		if _, err := svc.RenameAssistant(ctx, name); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	if _, err := svc.RenameAssistant(ctx, " 小空 "); err != nil {
		t.Fatal(err)
	}
	meta, err = NewService(repo).OpenAssistant(ctx)
	if err != nil || meta.Title != "小空" || len(repo.items) != 2 || repo.items["other"].Title != "other" {
		t.Fatalf("reopen: %#v, %v", meta, err)
	}
	agent := "coder"
	if _, err := svc.Update(ctx, UpdateRequest{SessionID: domainsession.AssistantID, CurrentAgent: &agent}); err == nil {
		t.Fatal("changed assistant target")
	}
}

type contextClearRepository struct {
	*memoryRepository
	cleared string
}

func (r *contextClearRepository) ClearSessionHistory(_ context.Context, id string) error {
	r.cleared = id
	return nil
}

func TestClearAssistantContextUsesFixedIdentity(t *testing.T) {
	repo := &contextClearRepository{memoryRepository: &memoryRepository{items: map[string]domainsession.Metadata{}}}
	if err := NewService(repo).ClearAssistantContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if repo.cleared != domainsession.AssistantID {
		t.Fatalf("cleared = %q", repo.cleared)
	}
	if err := NewService(nil).ClearAssistantContext(context.Background()); err == nil {
		t.Fatal("accepted missing storage")
	}
}
