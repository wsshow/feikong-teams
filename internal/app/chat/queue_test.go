package chat

import (
	"context"
	"errors"
	"fkteams/internal/app/chat/taskstream"
	"fkteams/internal/domain/message"
	domainprocess "fkteams/internal/domain/process"
	"sync"
	"testing"
	"time"
)

func TestRunQueuedDoesNotConsumeAfterFailure(t *testing.T) {
	stream := taskstream.NewManager().Register(taskstream.StreamConfig{SessionID: "session"})
	defer stream.Done()
	for _, text := range []string{"next", "later"} {
		if _, err := stream.EnqueueMessageIfProcessing(taskstream.QueuedMessage{Text: text}); err != nil {
			t.Fatal(err)
		}
	}
	buildErr := errors.New("cannot build queued input")
	var finished error
	err := NewService().RunQueued(context.Background(), QueueRequest{
		Stream: stream, First: TurnRequest{SessionID: "session", Runner: &fakeRunner{}},
		Next:   func(taskstream.QueuedMessage) (TurnRequest, error) { return TurnRequest{}, buildErr },
		Finish: func(err error) error { finished = err; return nil },
	})
	if !errors.Is(err, buildErr) || !errors.Is(finished, buildErr) || stream.Status() != "error" || stream.QueuedCount() != 1 {
		t.Fatalf("run=%v, finish=%v, status=%s, queued=%d", err, finished, stream.Status(), stream.QueuedCount())
	}
}

func TestRunQueuedReturnsFinalPersistenceFailure(t *testing.T) {
	stream := taskstream.NewManager().Register(taskstream.StreamConfig{SessionID: "session"})
	defer stream.Done()
	diskErr := errors.New("disk full")
	err := NewService().RunQueued(context.Background(), QueueRequest{
		Stream: stream, First: TurnRequest{SessionID: "session", Runner: &fakeRunner{}},
		Next: func(taskstream.QueuedMessage) (TurnRequest, error) {
			t.Fatal("empty queue consumed")
			return TurnRequest{}, nil
		},
		Finish: func(error) error {
			if stream.Status() != "processing" {
				t.Fatalf("completed before persistence: %s", stream.Status())
			}
			if _, err := stream.EnqueueMessageIfProcessing(taskstream.QueuedMessage{Text: "late"}); !errors.Is(err, taskstream.ErrQueueNotProcessing) {
				t.Fatalf("queue not sealed during persistence: %v", err)
			}
			return diskErr
		},
	})
	if !errors.Is(err, diskErr) || stream.Status() != "error" {
		t.Fatalf("run=%v, status=%s", err, stream.Status())
	}
	if _, err := stream.EnqueueMessageIfProcessing(taskstream.QueuedMessage{Text: "late"}); !errors.Is(err, taskstream.ErrQueueNotProcessing) {
		t.Fatalf("queue admission after failure: %v", err)
	}
}

type queueTestStore struct {
	items                []taskstream.QueuedMessage
	loadErr, errorOnSave error
	beforeSave           func()
}

func (s *queueTestStore) Load(string) ([]taskstream.QueuedMessage, error) {
	return append([]taskstream.QueuedMessage(nil), s.items...), s.loadErr
}
func (s *queueTestStore) Save(_ string, items []taskstream.QueuedMessage) error {
	if s.beforeSave != nil {
		s.beforeSave()
	}
	if s.errorOnSave != nil {
		return s.errorOnSave
	}
	s.items = append([]taskstream.QueuedMessage(nil), items...)
	return nil
}

func TestQueueFailedSaveKeepsCommittedState(t *testing.T) {
	manager := taskstream.NewManager()
	store := &queueTestStore{}
	notifications := 0
	service := NewQueueService(manager, store, func(*taskstream.Stream, []taskstream.QueuedMessage) { notifications++ })
	stream, _, err := service.RegisterIfIdle(taskstream.StreamConfig{SessionID: "session"})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Done()
	item, err := service.Enqueue(stream, taskstream.QueuedMessage{Text: "before", Kind: taskstream.QueueSteering})
	if err != nil {
		t.Fatal(err)
	}
	diskErr := errors.New("disk full")
	store.errorOnSave = diskErr
	_, err = service.Edit("session", func(q *taskstream.Queue) error {
		_, err := q.UpdateQueuedMessage(item.ID, "after", nil, "")
		return err
	})
	if !errors.Is(err, diskErr) || notifications != 1 || stream.QueueSnapshot()[0].Text != "before" {
		t.Fatalf("edit=%v, notifications=%d, queue=%v", err, notifications, stream.QueueSnapshot())
	}
	if items, err := service.TakeSteering(stream, 1); !errors.Is(err, diskErr) || len(items) != 0 {
		t.Fatalf("steering=%v, err=%v", items, err)
	}
	if _, ok, err := service.TakeNext(stream); !errors.Is(err, diskErr) || ok {
		t.Fatalf("next=%v, err=%v", ok, err)
	}
	if stream.QueuedCount() != 1 || notifications != 1 {
		t.Fatal("failed consumption changed state")
	}
}

func TestQueueConcurrentOfflineEditsDoNotLoseUpdates(t *testing.T) {
	store := &queueTestStore{items: []taskstream.QueuedMessage{{ID: "item", Text: ""}}}
	service := NewQueueService(taskstream.NewManager(), store, nil)
	var wg sync.WaitGroup
	for range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := service.Edit("session", func(q *taskstream.Queue) error {
				item := q.QueueSnapshot()[0]
				_, err := q.UpdateQueuedMessage(item.ID, item.Text+"x", nil, "")
				return err
			})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	items, err := service.Snapshot("session")
	if err != nil || len(items[0].Text) != 30 {
		t.Fatalf("items=%v, err=%v", items, err)
	}
	stream, _, err := service.RegisterIfIdle(taskstream.StreamConfig{SessionID: "session"})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Done()
	if stream.QueueSnapshot()[0].Text != items[0].Text {
		t.Fatal("start did not recover latest queue")
	}
}

func TestQueueLoadFailurePreventsStartAndSnapshot(t *testing.T) {
	manager := taskstream.NewManager()
	store := &queueTestStore{loadErr: errors.New("invalid snapshot")}
	service := NewQueueService(manager, store, nil)
	if _, _, err := service.RegisterIfIdle(taskstream.StreamConfig{SessionID: "session"}); !errors.Is(err, store.loadErr) {
		t.Fatal(err)
	}
	if manager.Get("session") != nil {
		t.Fatal("failed restore registered a task")
	}
	if _, err := service.Snapshot("session"); !errors.Is(err, store.loadErr) {
		t.Fatal(err)
	}
}

func TestQueueUncommittedMutationIsNotVisible(t *testing.T) {
	manager := taskstream.NewManager()
	store := &queueTestStore{}
	service := NewQueueService(manager, store, nil)
	stream, _, err := service.RegisterIfIdle(taskstream.StreamConfig{SessionID: "session"})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Done()
	entered, release := make(chan struct{}), make(chan struct{})
	store.beforeSave = func() { close(entered); <-release }
	done := make(chan error, 1)
	go func() { _, err := service.Enqueue(stream, taskstream.QueuedMessage{Text: "next"}); done <- err }()
	<-entered
	read := make(chan []taskstream.QueuedMessage, 1)
	go func() { read <- stream.QueueSnapshot() }()
	select {
	case <-read:
		t.Fatal("uncommitted queue visible")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if items := <-read; len(items) != 1 || items[0].Text != "next" {
		t.Fatal(items)
	}
}

func TestCancelledQueueRetainsSteeringForNextRun(t *testing.T) {
	manager := taskstream.NewManager()
	store := &queueTestStore{}
	service := NewQueueService(manager, store, nil)
	stream, _, err := service.RegisterIfIdle(taskstream.StreamConfig{SessionID: "session"})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Done()
	if _, err := service.Enqueue(stream, taskstream.QueuedMessage{Kind: taskstream.QueueSteering, Text: "pending"}); err != nil {
		t.Fatal(err)
	}
	stream.CancelIfProcessing()
	items, err := service.TakeSteering(stream, 1)
	if err != nil || len(items) != 0 || stream.QueuedCount() != 1 || len(store.items) != 1 {
		t.Fatalf("consumed=%v, err=%v, queued=%v", items, err, store.items)
	}
}

type receiptedTestStore struct {
	queueTestStore
	receipts map[string]bool
}

func (s *receiptedTestStore) HasReceipt(_, receipt string) (bool, error) {
	return s.receipts[receipt], nil
}
func (s *receiptedTestStore) SaveWithReceipt(id string, items []taskstream.QueuedMessage, receipt string) error {
	if err := s.Save(id, items); err != nil {
		return err
	}
	s.receipts[receipt] = true
	return nil
}

func TestProcessCompletionDeliveryRetriesAndDeduplicatesAfterConsumption(t *testing.T) {
	manager := taskstream.NewManager()
	store := &receiptedTestStore{receipts: make(map[string]bool)}
	published := 0
	queues := NewQueueService(manager, store, func(*taskstream.Stream, []taskstream.QueuedMessage) { published++ })
	stream, _, err := queues.RegisterIfIdle(taskstream.StreamConfig{SessionID: "session"})
	if err != nil {
		t.Fatal(err)
	}
	record := domainprocess.Record{ID: "process-one", Owner: domainprocess.Owner{SessionID: "session"}, State: domainprocess.Succeeded, Stdout: "completed"}
	store.errorOnSave = errors.New("disk unavailable")
	if err := queues.DeliverProcess(record); err == nil {
		t.Fatal("acknowledged failed delivery")
	}
	if stream.QueuedCount() != 0 || published != 0 || store.receipts[record.ID] {
		t.Fatal("failed delivery became visible")
	}
	store.errorOnSave = nil
	if err := queues.DeliverProcess(record); err != nil {
		t.Fatal(err)
	}
	item, ok, err := queues.TakeNext(stream)
	if err != nil || !ok || item.ID != record.ID {
		t.Fatalf("completion: %+v %v", item, err)
	}
	if err := queues.DeliverProcess(record); err != nil {
		t.Fatal(err)
	}
	if stream.QueuedCount() != 0 {
		t.Fatal("duplicated consumed completion")
	}
	// 重建用例后仍使用同一持久化回执，不依赖内存去重表。
	restarted := NewQueueService(taskstream.NewManager(), store, nil)
	if err := restarted.DeliverProcess(record); err != nil {
		t.Fatal(err)
	}
	items, err := restarted.Snapshot("session")
	if err != nil || len(items) != 0 {
		t.Fatalf("duplicate after restart: %+v %v", items, err)
	}
}

func TestRunQueuedBindsApplicationQueuesAndRestoresPendingResults(t *testing.T) {
	store := &receiptedTestStore{receipts: make(map[string]bool)}
	queues := NewQueueService(taskstream.NewManager(), store, nil)
	record := domainprocess.Record{ID: "completed-one", Owner: domainprocess.Owner{SessionID: "session"}, State: domainprocess.Succeeded, Stdout: "finished"}
	if err := queues.DeliverProcess(record); err != nil {
		t.Fatal(err)
	}
	ctx := WithQueues(context.Background(), queues)
	consumed := 0
	err := NewService().RunQueued(ctx, QueueRequest{
		First: TurnRequest{SessionID: "session", Runner: &fakeRunner{}},
		Next: func(item taskstream.QueuedMessage) (TurnRequest, error) {
			consumed++
			if item.ID != record.ID {
				t.Fatalf("item: %+v", item)
			}
			return TurnRequest{SessionID: "session", RunID: QueuedRunID("session", item), Runner: &fakeRunner{}}, nil
		},
	})
	if err != nil || consumed != 1 {
		t.Fatalf("run=%v consumed=%d", err, consumed)
	}
	stream := queues.Streams().Get("session")
	if stream == nil || !stream.IsDone() || stream.Status() != "completed" {
		t.Fatal("application did not own and finalize stream")
	}
	if err := queues.DeliverProcess(record); err != nil {
		t.Fatal(err)
	}
	items, err := queues.Snapshot("session")
	if err != nil || len(items) != 0 {
		t.Fatalf("completion repeated: %+v %v", items, err)
	}
}

func TestRunQueuedDoesNotAdmitConcurrentEntryOrConsumeOnSaveFailure(t *testing.T) {
	store := &queueTestStore{}
	queues := NewQueueService(taskstream.NewManager(), store, nil)
	stream, _, err := queues.RegisterIfIdle(taskstream.StreamConfig{SessionID: "session"})
	if err != nil {
		t.Fatal(err)
	}
	builder := func(taskstream.QueuedMessage) (TurnRequest, error) {
		t.Fatal("unexpected next turn")
		return TurnRequest{}, nil
	}
	ctx := WithQueues(context.Background(), queues)
	err = NewService().RunQueued(ctx, QueueRequest{First: TurnRequest{SessionID: "session", Runner: &fakeRunner{}}, Next: builder})
	if err == nil {
		t.Fatal("admitted a second entry for active session")
	}
	stream.Done()
	store.items = []taskstream.QueuedMessage{{ID: "pending", Text: "completion"}}
	store.errorOnSave = errors.New("disk full")
	err = NewService().RunQueued(ctx, QueueRequest{First: TurnRequest{SessionID: "session", Runner: &fakeRunner{}}, Next: builder})
	if !errors.Is(err, ErrQueuePersistence) {
		t.Fatalf("persistence failure hidden: %v", err)
	}
	items, err := queues.Snapshot("session")
	if err != nil || len(items) != 1 || items[0].ID != "pending" {
		t.Fatalf("pending result lost: %+v %v", items, err)
	}
}

func TestQueueObserversCannotReplaceDeliveryOrMutateOtherSnapshots(t *testing.T) {
	queues := NewQueueService(taskstream.NewManager(), &queueTestStore{}, nil)
	stream, _, err := queues.RegisterIfIdle(taskstream.StreamConfig{SessionID: "session"})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Done()
	first, second := 0, 0
	off := queues.Subscribe(func(_ *taskstream.Stream, items []taskstream.QueuedMessage) { first++; items[0].Text = "mutated" })
	queues.Subscribe(func(_ *taskstream.Stream, items []taskstream.QueuedMessage) {
		second++
		if items[0].Text != "original" {
			t.Fatal("observer received another observer's mutation")
		}
	})
	item, err := queues.Enqueue(stream, taskstream.QueuedMessage{Text: "original"})
	if err != nil {
		t.Fatal(err)
	}
	off()
	off()
	if _, err := queues.Edit("session", func(q *taskstream.Queue) error {
		_, err := q.UpdateQueuedMessage(item.ID, "original", nil, "")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if first != 1 || second != 2 {
		t.Fatalf("observers: %d %d", first, second)
	}
}

func TestRunQueuedRejectsSessionAndApplicationMismatch(t *testing.T) {
	first := NewQueueService(taskstream.NewManager(), &queueTestStore{}, nil)
	second := NewQueueService(taskstream.NewManager(), &queueTestStore{}, nil)
	stream, _, err := first.RegisterIfIdle(taskstream.StreamConfig{SessionID: "session"})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Done()
	for _, sessionID := range []string{"other-session", "session"} {
		runner := &fakeRunner{}
		err := NewService().RunQueued(WithQueues(context.Background(), second), QueueRequest{
			Stream: stream, First: TurnRequest{SessionID: sessionID, Runner: runner, Input: message.TurnInput{Message: message.Message{Content: "must not run"}}},
			Next: func(taskstream.QueuedMessage) (TurnRequest, error) {
				t.Fatal("foreign queue consumed")
				return TurnRequest{}, nil
			},
		})
		if err == nil || !runner.input.Message.IsEmpty() {
			t.Fatalf("foreign context admitted: input=%+v err=%v", runner.input, err)
		}
	}
}
