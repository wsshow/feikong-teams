package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"

	filequeue "fkteams/internal/adapters/storage/file/queue"
	appchat "fkteams/internal/app/chat"
	"fkteams/internal/app/chat/taskstream"
	"fkteams/internal/app/execution"
	"fkteams/internal/domain/event"
	"fkteams/internal/domain/message"
	domainprocess "fkteams/internal/domain/process"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/approval"
	"fkteams/internal/runtime/events"
)

type notificationRunnerFunc func(context.Context, message.TurnInput, runtimeport.RunOptions) (*runtimeport.RunResult, error)

func (f notificationRunnerFunc) Run(ctx context.Context, input message.TurnInput, opts runtimeport.RunOptions) (*runtimeport.RunResult, error) {
	return f(ctx, input, opts)
}

func notificationFixture(t *testing.T) (*Session, *appchat.QueueService, context.Context) {
	t.Helper()
	t.Setenv("FEIKONG_APP_DIR", t.TempDir())
	s := NewSession(ModeTeam, nil, nil)
	s.activateSession(false)
	queues := appchat.NewQueueService(taskstream.NewManager(), filequeue.NewStore(s.historyDir), nil)
	return s, queues, (execution.Dependencies{Queues: queues}).Context(context.Background())
}

func TestCLIProcessesCompletionWithHistoryAndOriginalApprovalPolicy(t *testing.T) {
	s, queues, ctx := notificationFixture(t)
	record := domainprocess.Record{ID: "process-one", Owner: domainprocess.Owner{SessionID: s.sessionID()}, State: domainprocess.Succeeded, Stdout: "completed output"}
	var inputs []message.TurnInput
	var runIDs []string
	runner := notificationRunnerFunc(func(ctx context.Context, input message.TurnInput, opts runtimeport.RunOptions) (*runtimeport.RunResult, error) {
		inputs = append(inputs, input)
		runIDs = append(runIDs, opts.RunID)
		turnID := events.TurnID(opts.RunID, 1)
		if err := opts.Sink(events.UserMessage(opts.RunID, turnID, turnID+":user", input.Message)); err != nil {
			return nil, err
		}
		if len(inputs) == 1 {
			if err := queues.DeliverProcess(record); err != nil {
				return nil, err
			}
		} else {
			decisions, err := opts.InterruptHandler(ctx, []runtimeport.Interrupt{{ID: "risk", IsRootCause: true}})
			if err != nil || decisions["risk"] != approval.Reject {
				t.Fatalf("notification changed approval policy: %+v %v", decisions, err)
			}
		}
		answer := "started"
		if len(inputs) > 1 {
			answer = "completion received"
		}
		if err := opts.Sink(event.Event{Type: event.TypeAssistantCompleted, RunID: opts.RunID, TurnID: turnID, MessageID: turnID + ":answer", Role: message.RoleAssistant, Content: answer, Message: &message.Message{Role: message.RoleAssistant, Content: answer}}); err != nil {
			return nil, err
		}
		return &runtimeport.RunResult{}, nil
	})
	executor := NewQueryExecutor(runner, s.GetQueryState())
	executor.SetSession(s)
	executor.SetAutoReject(true)
	var displayed []event.Event
	executor.SetCallbackBuilder(func() func(event.Event) error {
		return func(e event.Event) error { displayed = append(displayed, e); return nil }
	})
	if err := executor.Execute(ctx, "start work"); err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 2 || !strings.Contains(inputs[1].Message.Content, record.Stdout) {
		t.Fatalf("completion was not consumed: %+v", inputs)
	}
	if len(inputs[1].Context) < 2 {
		t.Fatalf("completion lost previous turn history: %+v", inputs[1])
	}
	if runIDs[0] == runIDs[1] || runIDs[1] != appchat.QueuedRunID(s.sessionID(), message.QueuedMessage{ID: record.ID}) {
		t.Fatalf("run IDs: %+v", runIDs)
	}
	found := false
	for _, e := range displayed {
		if e.Content == "completion received" {
			found = true
		}
	}
	if !found {
		t.Fatal("completion response was not sent through CLI view")
	}
	if s.GetQueryState().IsRunning() {
		t.Fatal("CLI did not finish after completion")
	}
	if err := queues.DeliverProcess(record); err != nil {
		t.Fatal(err)
	}
	items, err := queues.Snapshot(s.sessionID())
	if err != nil || len(items) != 0 {
		t.Fatalf("duplicate notification: %+v %v", items, err)
	}
}

func TestCLIPreservesCompletionAfterCancellationAndConsumesOnResume(t *testing.T) {
	s, queues, root := notificationFixture(t)
	record := domainprocess.Record{ID: "process-cancelled-turn", Owner: domainprocess.Owner{SessionID: s.sessionID()}, State: domainprocess.Succeeded, Stdout: "finished during cancellation"}
	ctx, cancel := context.WithCancel(root)
	defer cancel()
	runner := notificationRunnerFunc(func(_ context.Context, _ message.TurnInput, _ runtimeport.RunOptions) (*runtimeport.RunResult, error) {
		if err := queues.DeliverProcess(record); err != nil {
			return nil, err
		}
		cancel()
		return nil, context.Canceled
	})
	executor := NewQueryExecutor(runner, s.GetQueryState())
	executor.SetSession(s)
	executor.SetView(callbackQueryView{})
	if err := executor.Execute(ctx, "start"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	items, err := queues.Snapshot(s.sessionID())
	if err != nil || len(items) != 1 {
		t.Fatalf("completion lost on cancel: %+v %v", items, err)
	}
	// 模拟新 CLI 会话实例恢复，队列与回执来自持久化存储。
	resumed := NewSession(ModeTeam, nil, nil)
	resumed.activeSessionID = s.sessionID()
	newQueues := appchat.NewQueueService(taskstream.NewManager(), filequeue.NewStore(s.historyDir), nil)
	var inputs []message.TurnInput
	executor = NewQueryExecutor(notificationRunnerFunc(func(_ context.Context, input message.TurnInput, _ runtimeport.RunOptions) (*runtimeport.RunResult, error) {
		inputs = append(inputs, input)
		return &runtimeport.RunResult{}, nil
	}), resumed.GetQueryState())
	executor.SetSession(resumed)
	executor.SetView(callbackQueryView{})
	if err := executor.Execute(appchat.WithQueues(context.Background(), newQueues), "continue"); err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 2 || !strings.Contains(inputs[1].Message.Content, record.Stdout) {
		t.Fatalf("resume did not consume completion: %+v", inputs)
	}
	if err := newQueues.DeliverProcess(record); err != nil {
		t.Fatal(err)
	}
	items, err = newQueues.Snapshot(s.sessionID())
	if err != nil || len(items) != 0 {
		t.Fatalf("replayed consumed completion: %+v %v", items, err)
	}
}
