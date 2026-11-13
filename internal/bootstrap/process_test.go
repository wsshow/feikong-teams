package bootstrap

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	localprocess "fkteams/internal/adapters/process/local"
	commandtool "fkteams/internal/adapters/tools/builtin/command"
	cliruntime "fkteams/internal/adapters/transport/cli/runtime"
	httpserver "fkteams/internal/adapters/transport/http"
	"fkteams/internal/adapters/transport/http/handler"
	"fkteams/internal/app/chat/taskstream"
	"fkteams/internal/app/execution"
	appprocess "fkteams/internal/app/process"
	"fkteams/internal/domain/event"
	"fkteams/internal/domain/message"
	domainprocess "fkteams/internal/domain/process"
	"fkteams/internal/domain/session"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/events"
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

type processNotificationRunner func(context.Context, message.TurnInput, runtimeport.RunOptions) (*runtimeport.RunResult, error)

func (f processNotificationRunner) Run(ctx context.Context, input message.TurnInput, opts runtimeport.RunOptions) (*runtimeport.RunResult, error) {
	return f(ctx, input, opts)
}

func TestCLIReceivesRealCommandCompletionWithoutHTTP(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell fixture")
	}
	t.Setenv("FEIKONG_APP_DIR", t.TempDir())
	dependencies, err := NewExecutionDependencies()
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := dependencies.prepareStorage(context.Background(), httpserver.ModeAPI, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := dependencies.closeStorage(); err != nil {
			t.Error(err)
		}
	})
	if err := dependencies.Processes.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	s := cliruntime.NewSession(cliruntime.ModeTeam, nil, nil)
	s.SetResumeSessionID("cli-process-session")
	s.SetDocuments(dependencies.Documents)
	s.SetCallbackBuilder(func() func(event.Event) error { return func(event.Event) error { return nil } })
	tools := commandtool.NewCommandTools(t.TempDir(), commandtool.WithService(dependencies.Processes))
	ready := make(chan struct{}, 1)
	unsubscribe := dependencies.Queues.Subscribe(func(_ *taskstream.Stream, items []taskstream.QueuedMessage) {
		for _, item := range items {
			if strings.Contains(item.Text, "durable CLI output") {
				select {
				case ready <- struct{}{}:
				default:
				}
			}
		}
	})
	defer unsubscribe()
	var inputs []message.TurnInput
	taskID := ""
	runner := processNotificationRunner(func(ctx context.Context, input message.TurnInput, opts runtimeport.RunOptions) (*runtimeport.RunResult, error) {
		inputs = append(inputs, input)
		if len(inputs) == 1 {
			r, err := tools.SmartExecute(ctx, &commandtool.SmartExecuteRequest{Command: "sleep 0.05; printf 'durable CLI output'", Background: true})
			if err != nil {
				return nil, err
			}
			if !r.Success {
				t.Fatalf("command: %+v", r)
			}
			taskID = r.TaskID
			select {
			case <-ready:
			case <-time.After(4 * time.Second):
				t.Fatal("application did not deliver command completion to CLI")
			}
		}
		turnID := events.TurnID(opts.RunID, 1)
		if err := opts.Sink(events.UserMessage(opts.RunID, turnID, turnID+":user", input.Message)); err != nil {
			return nil, err
		}
		answer := "started"
		if len(inputs) > 1 {
			answer = "completion received"
		}
		return &runtimeport.RunResult{}, opts.Sink(event.Event{Type: event.TypeAssistantCompleted, RunID: opts.RunID, TurnID: turnID, MessageID: turnID + ":answer", Role: message.RoleAssistant, Content: answer, Message: &message.Message{Role: message.RoleAssistant, Content: answer}})
	})
	if err := s.HandleDirect(ctx, runner, make(chan os.Signal, 1), "run command"); err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 2 || !strings.Contains(inputs[1].Message.Content, taskID) || !strings.Contains(inputs[1].Message.Content, "durable CLI output") {
		t.Fatalf("CLI completion: %+v", inputs)
	}
	items, err := dependencies.Queues.Snapshot("cli-process-session")
	if err != nil || len(items) != 0 {
		t.Fatalf("notification left pending: %+v %v", items, err)
	}
}
