package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"

	eventlog "fkteams/internal/adapters/storage/file/history"
	"fkteams/internal/adapters/storage/sqlite"
	appchat "fkteams/internal/app/chat"
	appsession "fkteams/internal/app/session"
	assistanttools "fkteams/internal/app/tools/assistant"
	domainsession "fkteams/internal/domain/session"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/toolpolicy"
)

func TestAssistantToolsPersistNameAndEnforceSessionScope(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	db, err := sqlite.Open(ctx, filepath.Join(root, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	documents, err := sqlite.NewDocuments(db, root)
	if err != nil {
		t.Fatal(err)
	}
	repo := eventlog.NewSessionRepository(filepath.Join(root, "sessions"), documents)
	svc := appsession.NewService(repo)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := svc.OpenAssistant(ctx); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	toolList, err := assistanttools.GetTools(svc)
	if err != nil {
		t.Fatal(err)
	}
	if err := toolpolicy.ClassifyTools(toolList); err != nil {
		t.Fatal(err)
	}
	info, _ := toolList[1].Info(ctx)
	if !info.Policy.Serialize || !info.Policy.Destructive {
		t.Fatal("rename policy is missing")
	}
	invoke := func(ctx context.Context, tool runtimeport.Tool, args string) assistanttools.ProfileResponse {
		t.Helper()
		result, err := tool.Invoke(ctx, runtimeport.ToolInvocation{Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		var response assistanttools.ProfileResponse
		if err := json.Unmarshal([]byte(result.Content), &response); err != nil {
			t.Fatal(err)
		}
		return response
	}
	for _, denied := range []context.Context{ctx, domainsession.WithID(ctx, "other")} {
		if result := invoke(denied, toolList[1], `{"name":"wrong"}`); result.ErrorMessage == "" {
			t.Fatal("rename allowed outside assistant session")
		}
	}
	assistantCtx := domainsession.WithID(ctx, domainsession.AssistantID)
	if result := invoke(assistantCtx, toolList[1], `{"name":"未命名会话"}`); result.Name != "未命名会话" || result.ErrorMessage != "" {
		t.Fatalf("rename: %#v", result)
	}
	store := eventlog.NewChatSessionStore(filepath.Join(root, "sessions"), documents)
	if err := store.UpdateMetadata(ctx, appchat.MetadataUpdate{SessionID: domainsession.AssistantID, TitleSource: "do not replace my name", UpdateDefaultTitle: true}); err != nil {
		t.Fatal(err)
	}
	if result := invoke(assistantCtx, toolList[0], `{}`); result.Name != "未命名会话" {
		t.Fatalf("lifecycle replaced name: %#v", result)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sqlite.Open(ctx, filepath.Join(root, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	documents, err = sqlite.NewDocuments(db, root)
	if err != nil {
		t.Fatal(err)
	}
	reopened := appsession.NewService(eventlog.NewSessionRepository(filepath.Join(root, "sessions"), documents))
	meta, err := reopened.OpenAssistant(ctx)
	if err != nil || meta.Title != "未命名会话" {
		t.Fatalf("restarted: %#v, %v", meta, err)
	}
}
