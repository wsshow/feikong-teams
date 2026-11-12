package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"fkteams/internal/adapters/storage/sqlite"
	"fkteams/internal/app/appstate"
	appmemory "fkteams/internal/app/memory"
	memorytools "fkteams/internal/app/tools/memory"
	domainsession "fkteams/internal/domain/session"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/toolpolicy"
)

func TestMemoryToolsPersistRecallCorrectAndForget(t *testing.T) {
	db, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := sqlite.NewMemoryStore(db)
	manager, err := appmemory.NewManager(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	state := appstate.New()
	state.SetMemory(manager)
	ctx := domainsession.WithID(appstate.WithState(context.Background(), state), "user-session")
	toolList, err := memorytools.GetTools()
	if err != nil {
		t.Fatal(err)
	}
	if err := toolpolicy.MarkPolicyRequired(toolList); err != nil {
		t.Fatal(err)
	}
	if err := toolpolicy.ClassifyTools(toolList); err != nil {
		t.Fatal(err)
	}
	invoke := func(ctx context.Context, index int, args string) memorytools.Response {
		t.Helper()
		result, err := toolList[index].Invoke(ctx, runtimeport.ToolInvocation{Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		var response memorytools.Response
		if err := json.Unmarshal([]byte(result.Content), &response); err != nil {
			t.Fatal(err)
		}
		return response
	}
	if response := invoke(context.Background(), 1, `{"type":"preference","summary":"简短回复"}`); response.ErrorMessage == "" {
		t.Fatal("missing instance allowed")
	}
	saved := invoke(ctx, 1, `{"type":"preference","summary":"简短回复"}`)
	if saved.Saved == nil || saved.Saved.Version != 1 || saved.Saved.SessionID != "user-session" {
		t.Fatalf("save: %+v", saved)
	}
	// 新实例和新会话读取同一持久记录；不需要提取模型或更多用户消息。
	manager, err = appmemory.NewManager(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	state.SetMemory(manager)
	ctx = domainsession.WithID(ctx, "new-session")
	recalled := invoke(ctx, 0, `{}`)
	if len(recalled.Entries) != 1 || recalled.Entries[0].ID != saved.Saved.ID {
		t.Fatalf("recall: %+v", recalled)
	}
	args := fmt.Sprintf(`{"id":%q,"version":1,"type":"preference","summary":"简短回复","detail":"必要时展开"}`, saved.Saved.ID)
	corrected := invoke(ctx, 1, args)
	if corrected.Saved == nil || corrected.Saved.Version != 2 {
		t.Fatalf("correction: %+v", corrected)
	}
	if response := invoke(ctx, 2, fmt.Sprintf(`{"id":%q,"version":1}`, saved.Saved.ID)); response.ErrorMessage == "" {
		t.Fatal("stale forget accepted")
	}
	manager.SetEnabled(false)
	if response := invoke(ctx, 0, `{}`); response.ErrorMessage == "" {
		t.Fatal("disabled tools returned memory")
	}
	manager.SetEnabled(true)
	if response := invoke(ctx, 2, fmt.Sprintf(`{"id":%q,"version":2}`, saved.Saved.ID)); !response.Deleted {
		t.Fatalf("forget: %+v", response)
	}
	reloaded, err := appmemory.NewManager(store, nil)
	if err != nil || reloaded.Count() != 0 {
		t.Fatalf("deleted memory reappeared: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if response := invoke(ctx, 1, `{"type":"fact","summary":"must not publish"}`); response.ErrorMessage == "" || response.Saved != nil {
		t.Fatal("storage failure reported success")
	}
	if manager.Count() != 0 {
		t.Fatal("failed write published memory")
	}
}
