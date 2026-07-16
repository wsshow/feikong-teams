package taskstream

import (
	"context"
	"testing"
	"time"
)

func TestCleanupLifecycleCanBeReplacedAndStopped(t *testing.T) {
	manager := NewManager()
	manager.StartCleanup(context.Background(), time.Hour)
	manager.cleanupMu.Lock()
	first := manager.cleanupCancel
	manager.cleanupMu.Unlock()
	if first == nil {
		t.Fatal("cleanup was not started")
	}

	manager.StartCleanup(context.Background(), time.Hour)
	manager.cleanupMu.Lock()
	second := manager.cleanupCancel
	manager.cleanupMu.Unlock()
	if second == nil {
		t.Fatal("replacement cleanup was not started")
	}

	manager.StopCleanup()
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
