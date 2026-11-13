package taskstream

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCleanupLifecycleCanBeReplacedAndStopped(t *testing.T) {
	manager := NewManager()
	manager.StartCleanup(context.Background(), time.Hour)
	manager.cleanupMu.Lock()
	first := manager.cleanupCancel
	firstDone := manager.cleanupDone
	manager.cleanupMu.Unlock()
	if first == nil {
		t.Fatal("cleanup was not started")
	}

	manager.StartCleanup(context.Background(), time.Hour)
	manager.cleanupMu.Lock()
	second := manager.cleanupCancel
	secondDone := manager.cleanupDone
	manager.cleanupMu.Unlock()
	if second == nil {
		t.Fatal("replacement cleanup was not started")
	}
	select {
	case <-firstDone:
	default:
		t.Fatal("replacement returned before old cleanup exited")
	}

	manager.StopCleanup()
	select {
	case <-secondDone:
	default:
		t.Fatal("stop returned before cleanup exited")
	}
	manager.cleanupMu.Lock()
	defer manager.cleanupMu.Unlock()
	if manager.cleanupCancel != nil {
		t.Fatal("cleanup cancel function was not cleared")
	}
}

func TestCleanupRemovesCompletedStreamWithImmediateTTL(t *testing.T) {
	manager := NewManager()
	stream := manager.Register(StreamConfig{SessionID: "session", CleanupTTL: 0})
	stream.Done()
	manager.cleanup()

	if got := manager.Get("session"); got != nil {
		t.Fatal("completed stream with zero TTL should be removed")
	}
}

func TestRegisterDoesNotRunCancelWhileHoldingManagerLock(t *testing.T) {
	manager := NewManager()
	cancelled := make(chan struct{})
	manager.Register(StreamConfig{
		SessionID: "session",
		Cancel: func() {
			_ = manager.Get("session")
			close(cancelled)
		},
	})

	registered := make(chan struct{})
	go func() {
		manager.Register(StreamConfig{SessionID: "session"})
		close(registered)
	}()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("replacement cancel callback deadlocked on manager lock")
	}
	select {
	case <-registered:
	case <-time.After(time.Second):
		t.Fatal("replacement registration did not finish")
	}
}

func TestRegisterIfIdleKeepsSingleActiveStream(t *testing.T) {
	manager := NewManager()
	const callers = 64
	start := make(chan struct{})
	results := make(chan *Stream, callers)
	var created atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			stream, ok := manager.RegisterIfIdle(StreamConfig{SessionID: "session"})
			if ok {
				created.Add(1)
			}
			results <- stream
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	if got := created.Load(); got != 1 {
		t.Fatalf("created streams = %d, want 1", got)
	}
	var active *Stream
	for stream := range results {
		if active == nil {
			active = stream
			continue
		}
		if stream != active {
			t.Fatal("concurrent registration returned different active streams")
		}
	}
	active.Done()
}

func TestRegisterIfIdleWaitsForPreviousStreamDone(t *testing.T) {
	manager := NewManager()
	first, created := manager.RegisterIfIdle(StreamConfig{SessionID: "session"})
	if !created {
		t.Fatal("expected first stream to be created")
	}
	first.SetStatus("completed")
	if got, created := manager.RegisterIfIdle(StreamConfig{SessionID: "session"}); created || got != first {
		t.Fatal("status change alone must not replace a stream that has not finished")
	}
	first.Done()
	second, created := manager.RegisterIfIdle(StreamConfig{SessionID: "session"})
	if !created || second == first {
		t.Fatal("finished stream should be replaced")
	}
	second.Done()
}
