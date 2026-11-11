package bootstrap

import (
	"context"
	"fkteams/internal/adapters/tools/builtin/todo"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fkteams/internal/adapters/scheduler/filecron"
	eventlog "fkteams/internal/adapters/storage/file/history"
	filememory "fkteams/internal/adapters/storage/file/memory"
	filequeue "fkteams/internal/adapters/storage/file/queue"
	fileshare "fkteams/internal/adapters/storage/file/share"
	"fkteams/internal/adapters/tools/javascript"
	httpserver "fkteams/internal/adapters/transport/http"
	domainmemory "fkteams/internal/domain/memory"
	"fkteams/internal/domain/message"
	domainsession "fkteams/internal/domain/session"
)

func TestStorageUpgradeAllAdaptersAndRestart(t *testing.T) {
	root := t.TempDir()
	t.Setenv("FEIKONG_APP_DIR", root)
	ctx := context.Background()
	write := func(path, data string) {
		t.Helper()
		path = filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("sessions/one/metadata.json", `{"id":"one","title":"before"}`)
	// 无尾换行的历史在数据库追加后仍必须能恢复。
	write("sessions/one/transcript.jsonl", `{"type":"user_message","id":"old","content":"old message"}`)
	write("scheduler/scheduled_tasks.json", `{"tasks":[{"id":"task-one","task":"remember","status":"completed"}]}`)
	write("javascript/state/test.json", `{"count":3}`)
	write("sessions/one/attachments/metadata.json", `asset bytes`)
	write("sessions/one/todos.json", `{"todos":[{"id":"todo-one","title":"existing task","status":"pending"}]}`)
	write("sessions/orphan/transcript.jsonl", `{ "type":"user_message","content":"without metadata" }`)
	queues := filequeue.NewStore(filepath.Join(root, "sessions"))
	if err := queues.Save("one", []message.QueuedMessage{{ID: "queued-one", Kind: message.QueueFollowUp, Text: "next", Parts: []message.ContentPart{{Type: message.ContentPartImageURL, Base64Data: "aW1hZ2U=", MIMEType: "image/png"}}}}); err != nil {
		t.Fatal(err)
	}
	sharePath := filepath.Join(root, "share", "session_share.json")
	share := fileshare.NewSessionShareStore(sharePath)
	if err := share.Put("share-one", &fileshare.SessionEntry{SessionID: "one", Title: "shared", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	memories := []domainmemory.MemoryEntry{{ID: "memory-one", Type: domainmemory.Fact, Summary: "original memory", Detail: "line one\nline two", HitCount: 8, CreatedAt: time.Now()}}
	if err := filememory.NewStore(filepath.Join(root, "workspace", "memory")).Save(memories); err != nil {
		t.Fatal(err)
	}
	dependencies, err := NewExecutionDependencies()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dependencies.prepareStorage(ctx, httpserver.ModeAPI, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dependencies.closeStorage() })
	repo := eventlog.NewSessionRepository(filepath.Join(root, "sessions"), dependencies.Documents)
	if records, err := repo.ListSessions(ctx); err != nil || len(records) != 2 {
		t.Fatalf("orphan session lost: %v %v", records, err)
	}
	if _, err := repo.UpdateSession(ctx, "one", func(meta *domainsession.Metadata) error { meta.Title = "after"; return nil }); err != nil {
		t.Fatal(err)
	}
	queue, err := filequeue.NewStore(filepath.Join(root, "sessions"), dependencies.Documents).Load("one")
	if err != nil || len(queue) != 1 || queue[0].Parts[0].Base64Data != "aW1hZ2U=" {
		t.Fatalf("queue: %v %v", queue, err)
	}
	if _, ok := fileshare.NewSessionShareStore(sharePath, dependencies.Documents).Lookup("share-one"); !ok {
		t.Fatal("share missing")
	}
	scheduler, err := filecron.NewScheduler(filepath.Join(root, "scheduler"), dependencies.Documents)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := scheduler.ListTasks(ctx, "")
	if err != nil || len(tasks) != 1 {
		t.Fatalf("scheduler: %v %v", tasks, err)
	}
	state, err := javascript.NewFileStateStore(filepath.Join(root, "javascript", "state"), dependencies.Documents)
	if err != nil {
		t.Fatal(err)
	}
	if value, ok, err := state.Get("test", "count"); err != nil || !ok || value != float64(3) {
		t.Fatalf("script state: %v %v", value, err)
	}
	if err := state.Set("test", "count", 4); err != nil {
		t.Fatal(err)
	}
	tasksTool, err := todo.NewTodoTools(filepath.Join(root, "sessions"), dependencies.Documents)
	if err != nil {
		t.Fatal(err)
	}
	todoCtx := domainsession.WithID(ctx, "one")
	if result, err := tasksTool.TodoListFunc(todoCtx, &todo.TodoListRequest{}); err != nil || !result.Success || result.TotalCount != 1 {
		t.Fatalf("todo migration: %v %v", result, err)
	}
	if result, err := tasksTool.TodoAdd(todoCtx, &todo.TodoAddRequest{Title: "new database task"}); err != nil || !result.Success {
		t.Fatalf("todo write: %v %v", result, err)
	}
	originalTodo, _ := os.ReadFile(filepath.Join(root, "sessions", "one", "todos.json"))
	if string(originalTodo) != `{"todos":[{"id":"todo-one","title":"existing task","status":"pending"}]}` {
		t.Fatal("legacy todos changed")
	}
	loaded, err := dependencies.MemoryStore.Load(100)
	if err != nil || len(loaded) != 1 || loaded[0].ID != "memory-one" || loaded[0].Detail != memories[0].Detail || loaded[0].HitCount != 8 {
		t.Fatalf("memories: %v %v", loaded, err)
	}
	histories := eventlog.NewSessionHistoryManager()
	histories.Documents = dependencies.Documents
	recorder := histories.GetOrCreate("one", filepath.Join(root, "sessions"))
	recorder.RecordEvent(eventlog.Event{Type: eventlog.EventSystemNotice, Content: "after migration"})
	if err := recorder.Flush(); err != nil {
		t.Fatal(err)
	}
	if records, err := eventlog.LoadTranscriptFromFile(filepath.Join(root, "sessions", "one", "transcript.jsonl"), dependencies.Documents); err != nil || len(records) != 2 {
		t.Fatalf("history: %v %v", records, err)
	}
	original, err := os.ReadFile(filepath.Join(root, "sessions", "one", "metadata.json"))
	if err != nil || string(original) != `{"id":"one","title":"before"}` {
		t.Fatal("legacy source was overwritten")
	}
	backups, err := filepath.Glob(filepath.Join(root, "backups", "*", "manifest.json"))
	if err != nil || len(backups) != 1 {
		t.Fatal("missing verified backup")
	}
	if err := dependencies.closeStorage(); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewExecutionDependencies()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.prepareStorage(ctx, httpserver.ModeAPI, nil); err != nil {
		t.Fatal(err)
	}
	defer restarted.closeStorage()
	meta, err := eventlog.NewSessionRepository(filepath.Join(root, "sessions"), restarted.Documents).LoadSession(ctx, "one")
	if err != nil || meta.Title != "after" {
		t.Fatalf("restart read legacy data: %v %v", meta, err)
	}
	backups, _ = filepath.Glob(filepath.Join(root, "backups", "*", "manifest.json"))
	if len(backups) != 1 {
		t.Fatal("completed migration reran")
	}
}
