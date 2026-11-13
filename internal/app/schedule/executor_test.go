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

type conversationHistory struct {
	facts     []events.Event
	finalized bool
}

func (h *conversationHistory) RecordAndEmit(e events.Event, sink func(events.Event) error) error {
	h.facts = append(h.facts, e)
	if sink != nil {
		return sink(e)
	}
	return nil
}
func (h *conversationHistory) ReplayEvents() ([]events.Event, error) {
	return append([]events.Event(nil), h.facts...), nil
}
func (h *conversationHistory) FinalizeCurrent()       { h.finalized = true }
func (h *conversationHistory) RecordCancelled(string) {}
func (h *conversationHistory) Flush() error           { return nil }

type conversationMetadata struct{ updates []appchat.MetadataUpdate }

func (m *conversationMetadata) UpdateMetadata(_ context.Context, u appchat.MetadataUpdate) error {
	m.updates = append(m.updates, u)
	return nil
}

func TestScheduledExecutionUsesSharedConversationHistoryAndWebPresentation(t *testing.T) {
	queues := appchat.NewQueueService(taskstream.NewManager(), &notificationStore{receipts: make(map[string]bool)}, nil)
	history := &conversationHistory{}
	metadata := &conversationMetadata{}
	released := 0
	runner := notificationRunner(func(_ context.Context, _ message.TurnInput, opts runtimeport.RunOptions) (*runtimeport.RunResult, error) {
		emitter := events.NewEmitter(opts.RunID, events.TurnID(opts.RunID, 1), opts.Sink)
		return &runtimeport.RunResult{}, emitter.Emit(events.AssistantCompleted(events.MessageEvent{MessageID: opts.RunID + ":answer", Role: message.RoleAssistant, Content: "scheduled answer"}))
	})
	executor, err := NewBackgroundExecutor(func(context.Context) (runtimeport.Runner, error) { return runner, nil }, &resultStore{})
	if err != nil {
		t.Fatal(err)
	}
	executor.WithConversationHistory(func(id string) (appchat.SessionHistory, func(), error) {
		if id != "fkteams_scheduler_job" {
			t.Fatalf("session=%s", id)
		}
		return history, func() { released++ }, nil
	}, metadata)
	for range 2 {
		if _, err := executor.Execute(appchat.WithQueues(context.Background(), queues), "job", "run task"); err != nil {
			t.Fatal(err)
		}
	}
	view, _ := queues.Streams().Get("fkteams_scheduler_job").PresentationSnapshot()
	if len(view.Items) != 2 || view.Items[0].Node.Message.Content != "scheduled answer" || !history.finalized || released != 2 || len(metadata.updates) != 4 {
		t.Fatalf("view=%+v released=%d history=%+v metadata=%+v", view, released, history, metadata)
	}
}
