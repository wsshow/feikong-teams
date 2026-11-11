package lifecycle

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestControlsHaveIndependentShutdownState(t *testing.T) {
	var count atomic.Int32
	done := make(chan struct{}, 2)
	first := NewControl(func() { count.Add(1); done <- struct{}{} }, nil)
	second := NewControl(func() { count.Add(1); done <- struct{}{} }, nil)
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); first.Shutdown(); second.Shutdown() }()
	}
	wg.Wait()
	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("shutdown was not called")
		}
	}
	if count.Load() != 2 {
		t.Fatalf("shutdown calls = %d", count.Load())
	}
}

func TestRestartRunsOnceAfterCleanup(t *testing.T) {
	expected := errors.New("restart failed")
	done := make(chan struct{})
	calls := 0
	control := NewControl(func() { close(done) }, func() error { calls++; return expected })
	if err := control.ExecutePendingRestart(); err != nil {
		t.Fatal(err)
	}
	control.Restart()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown was not called")
	}
	if err := control.ExecutePendingRestart(); !errors.Is(err, expected) {
		t.Fatalf("restart = %v", err)
	}
	if err := control.ExecutePendingRestart(); err != nil || calls != 1 {
		t.Fatalf("restart = %v, calls = %d", err, calls)
	}
}

func TestUnconfiguredControlCannotScheduleRestart(t *testing.T) {
	var nilControl *Control
	if nilControl.Available() {
		t.Fatal("nil control is available")
	}
	nilControl.Shutdown()
	nilControl.Restart()
	calls := 0
	control := NewControl(nil, func() error { calls++; return nil })
	control.Restart()
	if err := control.ExecutePendingRestart(); err != nil || calls != 0 {
		t.Fatalf("restart = %v, calls = %d", err, calls)
	}
}
