package chat

import (
	"context"
	"errors"
	"fkteams/internal/app/chat/taskstream"
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
