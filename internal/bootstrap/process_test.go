package bootstrap

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	localprocess "fkteams/internal/adapters/process/local"
	commandtool "fkteams/internal/adapters/tools/builtin/command"
	"fkteams/internal/adapters/transport/http/handler"
	"fkteams/internal/app/execution"
	appprocess "fkteams/internal/app/process"
	domainprocess "fkteams/internal/domain/process"
	"fkteams/internal/domain/session"
)

func TestProcessCompletionQueuesOnceAndRecoversLogs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell fixture")
	}
	directory := t.TempDir()
	backend, err := localprocess.New(directory)
	if err != nil {
		t.Fatal(err)
	}
	service, err := appprocess.NewService(backend, backend)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Close(); backend.Close() })
	rt := handler.NewRuntime(handler.RuntimeOptions{HistoryDir: t.TempDir(), Dependencies: execution.Dependencies{Processes: service}})
	t.Cleanup(rt.Close)
	delivered := make(chan domainprocess.Record, 1)
	service.SetCompletionSink(func(r domainprocess.Record) error {
		if err := rt.Queues.DeliverProcess(r); err != nil {
			return err
		}
		delivered <- r
		return nil
	})
	if err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	tools := commandtool.NewCommandTools(t.TempDir(), commandtool.WithService(service))
	ctx := session.WithID(context.Background(), "process-session")
	r, err := tools.SmartExecute(ctx, &commandtool.SmartExecuteRequest{Command: "sleep 0.1; printf durable", Background: true})
	if err != nil || !r.Success {
		t.Fatalf("execute: %+v %v", r, err)
	}
	select {
	case completion := <-delivered:
		if completion.ID != r.TaskID || completion.State != domainprocess.Succeeded {
			t.Fatalf("completion: %+v", completion)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("completion was not delivered")
	}
	items, err := rt.Queues.Snapshot("process-session")
	if err != nil || len(items) != 1 || items[0].ID != r.TaskID || !strings.Contains(items[0].Text, "durable") {
		t.Fatalf("queue: %+v %v", items, err)
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	// 重新构造注册表模拟重启，保留终态和日志，但不再执行命令。
	recovered, err := appprocess.NewService(backend, backend)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	records, err := backend.Load()
	if err != nil || len(records) != 1 {
		t.Fatalf("records: %+v %v", records, err)
	}
	record, out, err := recovered.Read(context.Background(), records[0].Owner, r.TaskID, 0, 100, 0)
	if err != nil || record.State != domainprocess.Succeeded || out.Text != "durable" {
		t.Fatalf("recovered: %+v %+v %v", record, out, err)
	}
}
