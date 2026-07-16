package handler

import (
	"testing"
	"time"

	"fkteams/internal/app/chat/taskstream"
)

func TestSessionManagerReplacesDuplicateSubscription(t *testing.T) {
	manager := taskstream.NewManager()
	stream := manager.Register(taskstream.StreamConfig{SessionID: "session"})
	firstOK, firstID := stream.Subscribe(taskstream.FuncSubscriber(func(taskstream.Event) error { return nil }), 0)
	secondOK, secondID := stream.Subscribe(taskstream.FuncSubscriber(func(taskstream.Event) error { return nil }), 0)
	if !firstOK || !secondOK {
		t.Fatal("expected subscriptions to succeed")
	}
	sm := &sessionManager{tasks: make(map[string]*sessionTask)}
	sm.attachSubscription("session", stream, firstID)
	sm.attachSubscription("session", stream, secondID)

	if count := stream.SubscriptionCount(); count != 1 {
		t.Fatalf("subscription count = %d, want 1", count)
	}
	sm.detachAll(manager)
	if count := stream.SubscriptionCount(); count != 0 {
		t.Fatalf("subscription count after detach = %d, want 0", count)
	}
}

func TestSessionManagerStartTaskDetachesAndCancelsPreviousTask(t *testing.T) {
	manager := taskstream.NewManager()
	first := manager.Register(taskstream.StreamConfig{SessionID: "first"})
	ok, firstSubID := first.Subscribe(taskstream.FuncSubscriber(func(taskstream.Event) error { return nil }), 0)
	if !ok {
		t.Fatal("expected first subscription to succeed")
	}
	cancelled := 0
	sm := &sessionManager{tasks: make(map[string]*sessionTask)}
	sm.startTask("session", func() { cancelled++ }, first, firstSubID)

	second := manager.Register(taskstream.StreamConfig{SessionID: "second"})
	ok, secondSubID := second.Subscribe(taskstream.FuncSubscriber(func(taskstream.Event) error { return nil }), 0)
	if !ok {
		t.Fatal("expected second subscription to succeed")
	}
	secondTaskID := sm.startTask("session", func() {}, second, secondSubID)

	if cancelled != 1 {
		t.Fatalf("previous task cancel calls = %d, want 1", cancelled)
	}
	if count := first.SubscriptionCount(); count != 0 {
		t.Fatalf("previous subscription count = %d, want 0", count)
	}
	if count := second.SubscriptionCount(); count != 1 {
		t.Fatalf("replacement subscription count = %d, want 1", count)
	}
	sm.removeTask("session", secondTaskID)
	second.Done()
}

func TestSessionManagerCancelTaskDoesNotHoldManagerLock(t *testing.T) {
	sm := &sessionManager{tasks: make(map[string]*sessionTask)}
	cancelled := make(chan struct{})
	sm.tasks["session"] = &sessionTask{
		cancel: func() {
			sm.removeTask("session", 1)
			close(cancelled)
		},
		id: 1,
	}

	go sm.cancelTask("session")
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("cancel callback deadlocked on session manager lock")
	}
}
