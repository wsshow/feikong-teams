package todo

import (
	"context"
	sqlitestore "fkteams/internal/adapters/storage/sqlite"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"fkteams/internal/domain/session"
)

func newTestTodoTools(t *testing.T) (*TodoTools, context.Context) {
	t.Helper()
	tools, err := NewTodoTools(t.TempDir())
	if err != nil {
		t.Fatalf("NewTodoTools failed: %v", err)
	}
	return tools, session.WithID(context.Background(), "session-a")
}

func strPtr(s string) *string { return &s }

func TestTodoAddListUpdateDelete(t *testing.T) {
	tools, ctx := newTestTodoTools(t)

	addResp, err := tools.TodoAdd(ctx, &TodoAddRequest{
		Title:       "补测试",
		Description: "覆盖待办工具",
		Priority:    "high",
	})
	if err != nil {
		t.Fatalf("TodoAdd returned error: %v", err)
	}
	if !addResp.Success || addResp.Todo == nil {
		t.Fatalf("TodoAdd failed: %#v", addResp)
	}
	id := addResp.Todo.ID

	listResp, err := tools.TodoListFunc(ctx, &TodoListRequest{Priority: "high"})
	if err != nil {
		t.Fatalf("TodoListFunc returned error: %v", err)
	}
	if !listResp.Success || listResp.TotalCount != 1 || listResp.Todos[0].ID != id {
		t.Fatalf("TodoListFunc = %#v, want one high priority todo %s", listResp, id)
	}

	updateResp, err := tools.TodoUpdate(ctx, &TodoUpdateRequest{
		ID:       id,
		Status:   strPtr("completed"),
		Priority: strPtr("urgent"),
	})
	if err != nil {
		t.Fatalf("TodoUpdate returned error: %v", err)
	}
	if !updateResp.Success || updateResp.Todo.Status != "completed" || updateResp.Todo.CompletedAt == nil {
		t.Fatalf("TodoUpdate = %#v, want completed todo", updateResp)
	}

	deleteResp, err := tools.TodoDelete(ctx, &TodoDeleteRequest{ID: id})
	if err != nil {
		t.Fatalf("TodoDelete returned error: %v", err)
	}
	if !deleteResp.Success {
		t.Fatalf("TodoDelete failed: %#v", deleteResp)
	}

	emptyResp, err := tools.TodoListFunc(ctx, &TodoListRequest{})
	if err != nil {
		t.Fatalf("TodoListFunc returned error: %v", err)
	}
	if emptyResp.TotalCount != 0 {
		t.Fatalf("TotalCount = %d, want 0", emptyResp.TotalCount)
	}
}

func TestTodoValidationAndSessionRequired(t *testing.T) {
	tools, ctx := newTestTodoTools(t)

	tests := []struct {
		name string
		run  func() string
		want string
	}{
		{
			name: "empty title",
			run: func() string {
				resp, _ := tools.TodoAdd(ctx, &TodoAddRequest{})
				return resp.ErrorMessage
			},
			want: "标题不能为空",
		},
		{
			name: "bad priority",
			run: func() string {
				resp, _ := tools.TodoAdd(ctx, &TodoAddRequest{Title: "x", Priority: "bad"})
				return resp.ErrorMessage
			},
			want: "优先级必须是",
		},
		{
			name: "bad status",
			run: func() string {
				resp, _ := tools.TodoUpdate(ctx, &TodoUpdateRequest{ID: "missing", Status: strPtr("bad")})
				return resp.ErrorMessage
			},
			want: "状态必须是",
		},
		{
			name: "missing session",
			run: func() string {
				resp, _ := tools.TodoListFunc(context.Background(), &TodoListRequest{})
				return resp.ErrorMessage
			},
			want: "会话 ID 未设置",
		},
		{
			name: "invalid session",
			run: func() string {
				invalidCtx := session.WithID(context.Background(), "..")
				resp, _ := tools.TodoListFunc(invalidCtx, &TodoListRequest{})
				return resp.ErrorMessage
			},
			want: "会话 ID 未设置",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.run(); !strings.Contains(got, tt.want) {
				t.Fatalf("error = %q, want containing %q", got, tt.want)
			}
		})
	}
}

func TestTodoConcurrentFirstWritesPreserveAllItems(t *testing.T) {
	tools, ctx := newTestTodoTools(t)
	const count = 32

	var wg sync.WaitGroup
	for i := range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := tools.TodoAdd(ctx, &TodoAddRequest{Title: "item"})
			if err != nil || !resp.Success {
				t.Errorf("TodoAdd(%d) resp=%#v err=%v", i, resp, err)
			}
		}()
	}
	wg.Wait()

	resp, err := tools.TodoListFunc(ctx, &TodoListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.TotalCount != count {
		t.Fatalf("todo count = %d, want %d", resp.TotalCount, count)
	}
}

func TestTodoRejectsSymlinkStore(t *testing.T) {
	tools, ctx := newTestTodoTools(t)
	sessionDir := filepath.Join(tools.sessionsDir, "session-a")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{"todos":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(sessionDir, "todos.json")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	resp, err := tools.TodoListFunc(ctx, &TodoListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Success || !strings.Contains(resp.ErrorMessage, "路径无效") {
		t.Fatalf("response = %#v, want symlink rejection", resp)
	}
}

func TestTodoEnforcesStorageLimits(t *testing.T) {
	tools, ctx := newTestTodoTools(t)
	filePath, err := tools.getFilePath(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := tools.saveTo(filePath, &TodoList{Todos: make([]Todo, maxTodoItems+1)}); err == nil {
		t.Fatal("saveTo accepted too many todo items")
	}
	if err := os.WriteFile(filePath, []byte(strings.Repeat(" ", int(maxTodoStoreBytes+1))), 0644); err != nil {
		t.Fatal(err)
	}
	resp, err := tools.TodoListFunc(ctx, &TodoListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Success || !strings.Contains(resp.ErrorMessage, "超过") {
		t.Fatalf("response = %#v, want size limit rejection", resp)
	}
}

func TestTodoBatchAndClearKeepSessionsIsolated(t *testing.T) {
	tools, ctxA := newTestTodoTools(t)
	ctxB := session.WithID(context.Background(), "session-b")

	batchResp, err := tools.TodoBatchAdd(ctxA, &TodoBatchAddRequest{Todos: []struct {
		Title       string `json:"title" jsonschema:"description=待办事项标题,required"`
		Description string `json:"description,omitempty" jsonschema:"description=待办事项详细描述"`
		Priority    string `json:"priority,omitempty" jsonschema:"description=优先级: low(低), medium(中), high(高), urgent(紧急)"`
	}{
		{Title: "A1", Priority: "low"},
		{Title: "A2", Priority: "urgent"},
	}})
	if err != nil {
		t.Fatalf("TodoBatchAdd returned error: %v", err)
	}
	if !batchResp.Success || batchResp.AddedCount != 2 {
		t.Fatalf("TodoBatchAdd = %#v, want two added", batchResp)
	}

	if _, err := tools.TodoAdd(ctxB, &TodoAddRequest{Title: "B1", Priority: "urgent"}); err != nil {
		t.Fatalf("TodoAdd session B returned error: %v", err)
	}

	clearResp, err := tools.TodoClear(ctxA, &TodoClearRequest{Status: "pending"})
	if err != nil {
		t.Fatalf("TodoClear returned error: %v", err)
	}
	if !clearResp.Success || clearResp.ClearedCount != 2 {
		t.Fatalf("TodoClear = %#v, want two cleared", clearResp)
	}

	listA, err := tools.TodoListFunc(ctxA, &TodoListRequest{})
	if err != nil {
		t.Fatalf("TodoListFunc session A returned error: %v", err)
	}
	if listA.TotalCount != 0 {
		t.Fatalf("session A count = %d, want 0", listA.TotalCount)
	}

	listB, err := tools.TodoListFunc(ctxB, &TodoListRequest{})
	if err != nil {
		t.Fatalf("TodoListFunc session B returned error: %v", err)
	}
	if listB.TotalCount != 1 || listB.Todos[0].Title != "B1" {
		t.Fatalf("session B list = %#v, want B1 preserved", listB)
	}
}

func TestDatabaseTodoRejectsSymlinkAndCrossSessionPath(t *testing.T) {
	root := t.TempDir()
	db, err := sqlitestore.Open(context.Background(), filepath.Join(root, "data", "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	documents, err := sqlitestore.NewDocuments(db, root)
	if err != nil {
		t.Fatal(err)
	}
	tools, err := NewTodoTools(filepath.Join(root, "sessions"), documents)
	if err != nil {
		t.Fatal(err)
	}
	ctx := session.WithID(context.Background(), "one")
	path, err := tools.getFilePath(ctx)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{"todos":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Skip(err)
	}
	response, err := tools.TodoAdd(ctx, &TodoAddRequest{Title: "blocked"})
	if err != nil || response.Success || response.ErrorMessage == "" {
		t.Fatalf("symlink was accepted: %v %v", response, err)
	}
	if err := tools.saveTo(filepath.Join(root, "other.json"), &TodoList{}); err == nil {
		t.Fatal("path outside sessions accepted")
	}
	data, _ := os.ReadFile(outside)
	if string(data) != `{"todos":[]}` {
		t.Fatal("outside target modified")
	}
}
