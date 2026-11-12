package resources

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestKeyedLocksSerializeAndReleaseEntries(t *testing.T) {
	locks := NewKeyedLocks()
	var wg sync.WaitGroup
	count := 0
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := locks.Acquire(context.Background(), "same")
			if err != nil {
				t.Error(err)
				return
			}
			defer release()
			count++
		}()
	}
	wg.Wait()
	if count != 64 {
		t.Fatalf("lost updates: %d", count)
	}
	locks.mu.Lock()
	defer locks.mu.Unlock()
	if len(locks.entries) != 0 {
		t.Fatal("unused lock entries leaked")
	}
}
func TestKeyedLocksCancellationAndIndependentKeys(t *testing.T) {
	locks := NewKeyedLocks()
	release, err := locks.Acquire(context.Background(), "one")
	if err != nil {
		t.Fatal(err)
	}
	other, err := locks.Acquire(context.Background(), "two")
	if err != nil {
		t.Fatal(err)
	}
	other()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := locks.Acquire(ctx, "one"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	release()
	release()
	if len(locks.entries) != 0 {
		t.Fatal("cancelled waiter leaked lock entry")
	}
}

func TestSharedReadsRemainParallel(t *testing.T) {
	locks := NewKeyedLocks()
	first, err := locks.AcquireRead(context.Background(), "file")
	if err != nil {
		t.Fatal(err)
	}
	defer first()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	second, err := locks.AcquireRead(ctx, "file")
	if err != nil {
		t.Fatalf("readers serialized: %v", err)
	}
	second()
	cancelled, cancelWait := context.WithCancel(context.Background())
	cancelWait()
	if _, err := locks.Acquire(cancelled, "file"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
