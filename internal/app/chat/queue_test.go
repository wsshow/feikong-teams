package chat

import (
	"context"
	"errors"
	"fkteams/internal/app/chat/taskstream"
	"testing"
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
