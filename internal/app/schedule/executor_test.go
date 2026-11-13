package schedule

import (
	"context"
	"strings"
	"testing"

	appchat "fkteams/internal/app/chat"
	"fkteams/internal/app/chat/taskstream"
	"fkteams/internal/domain/message"
	domainprocess "fkteams/internal/domain/process"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/events"
)

type notificationStore struct {
	items    []message.QueuedMessage
	receipts map[string]bool
}

func (s *notificationStore) Load(string) ([]message.QueuedMessage, error) {
	return append([]message.QueuedMessage(nil), s.items...), nil
}
func (s *notificationStore) Save(_ string, items []message.QueuedMessage) error {
	s.items = append([]message.QueuedMessage(nil), items...)
	return nil
}
func (s *notificationStore) HasReceipt(_, id string) (bool, error) { return s.receipts[id], nil }
func (s *notificationStore) SaveWithReceipt(sessionID string, items []message.QueuedMessage, id string) error {
	s.Save(sessionID, items)
	s.receipts[id] = true
	return nil
}

type resultStore struct{ result string }

func (s *resultStore) SaveResult(_, _, result string) error { s.result = result; return nil }

type notificationRunner func(context.Context, message.TurnInput, runtimeport.RunOptions) (*runtimeport.RunResult, error)

func (f notificationRunner) Run(ctx context.Context, input message.TurnInput, opts runtimeport.RunOptions) (*runtimeport.RunResult, error) {
	return f(ctx, input, opts)
}

func TestScheduledExecutionConsumesCompletionAndIncludesResponseInReport(t *testing.T) {
	queues := appchat.NewQueueService(taskstream.NewManager(), &notificationStore{receipts: make(map[string]bool)}, nil)
	store := &resultStore{}
	var inputs []message.TurnInput
	runner := notificationRunner(func(_ context.Context, input message.TurnInput, opts runtimeport.RunOptions) (*runtimeport.RunResult, error) {
		inputs = append(inputs, input)
		answer := "started"
		if len(inputs) == 1 {
			if err := queues.DeliverProcess(domainprocess.Record{ID: "process-scheduled", Owner: domainprocess.Owner{SessionID: "fkteams_scheduler_job"}, State: domainprocess.Succeeded, Stdout: "scheduled result"}); err != nil {
				return nil, err
			}
		} else {
			answer = "completion received"
		}
		emitter := events.NewEmitter(opts.RunID, events.TurnID(opts.RunID, 1), opts.Sink)
		meta := events.MessageEvent{MessageID: opts.RunID + ":answer", Role: message.RoleAssistant, Content: answer}
		if err := emitter.Emit(events.AssistantDelta(meta, answer)); err != nil {
			return nil, err
		}
		return &runtimeport.RunResult{}, emitter.Emit(events.AssistantCompleted(meta))
	})
	executor, err := NewBackgroundExecutor(func(context.Context) (runtimeport.Runner, error) { return runner, nil }, store)
	if err != nil {
		t.Fatal(err)
	}
	output, err := executor.Execute(appchat.WithQueues(context.Background(), queues), "job", "run background work")
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 2 || !strings.Contains(inputs[1].Message.Content, "scheduled result") {
		t.Fatalf("scheduler completion: %+v", inputs)
	}
	if len(inputs[1].Context) != 2 || inputs[1].Context[0].Content != "run background work" || !strings.Contains(inputs[1].Context[1].Content, "started") {
		t.Fatalf("continuation lost context: %+v", inputs[1].Context)
	}
	if store.result != output || !strings.Contains(output, "completion received") {
		t.Fatalf("report omitted completion: %q", output)
	}
}
