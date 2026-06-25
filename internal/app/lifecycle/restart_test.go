package lifecycle

import (
	"sync"
	"testing"
	"time"
)

func resetRestartState() {
	shutdownMu.Lock()
	shutdownFn = nil
	shutdownMu.Unlock()
	shutdownOnce = sync.Once{}
	pendingRestart.Store(false)
}

func TestShutdownRegistrationAndTrigger(t *testing.T) {
	resetRestartState()
	t.Cleanup(resetRestartState)

	if IsShutdownAvailable() {
		t.Fatal("shutdown should not be available before registration")
	}

	called := make(chan struct{}, 2)
	SetShutdownFunc(func() {
		called <- struct{}{}
	})
	if !IsShutdownAvailable() {
		t.Fatal("shutdown should be available after registration")
	}

	TriggerShutdown()
	TriggerShutdown()

	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("shutdown callback was not called")
	}
	select {
	case <-called:
		t.Fatal("shutdown callback should only be called once")
	case <-time.After(250 * time.Millisecond):
	}
}

func TestTriggerRestartAndExecutePendingRestart(t *testing.T) {
	resetRestartState()
	t.Cleanup(resetRestartState)

	if err := ExecutePendingRestart(); err != nil {
		t.Fatalf("ExecutePendingRestart without pending restart returned error: %v", err)
	}

	t.Setenv("FEIKONG_NO_SELF_RESTART", "1")
	TriggerRestart()
	if !pendingRestart.Load() {
		t.Fatal("TriggerRestart should mark pending restart")
	}
	if err := ExecutePendingRestart(); err != nil {
		t.Fatalf("ExecutePendingRestart with no-self-restart returned error: %v", err)
	}
}
