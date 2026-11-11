package migration

import (
	"context"
	domainmigration "fkteams/internal/domain/migration"
	"sync/atomic"
	"testing"
)

type testSource struct {
	calls   atomic.Int32
	release chan struct{}
}

func (s *testSource) Prepare(ctx context.Context, _ func(string, int, int)) (domainmigration.Snapshot, error) {
	s.calls.Add(1)
	select {
	case <-s.release:
		return domainmigration.Snapshot{}, nil
	case <-ctx.Done():
		return domainmigration.Snapshot{}, ctx.Err()
	}
}

type testTarget struct{ complete atomic.Bool }

func (s *testTarget) Completed(context.Context) (bool, error) { return s.complete.Load(), nil }
func (s *testTarget) Import(context.Context, domainmigration.Snapshot, func(string, int, int)) error {
	s.complete.Store(true)
	return nil
}
func TestSingleJobAndCancelledWait(t *testing.T) {
	source := &testSource{release: make(chan struct{})}
	target := &testTarget{}
	service, err := NewService(context.Background(), source, target)
	if err != nil {
		t.Fatal(err)
	}
	if !service.Start(context.Background()) || service.Start(context.Background()) {
		t.Fatal("duplicate migration started")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.Wait(ctx); err == nil {
		t.Fatal("cancelled waiter succeeded")
	}
	close(source.release)
	if err := service.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if source.calls.Load() != 1 || service.Start(context.Background()) {
		t.Fatal("completed migration restarted")
	}
	restored, err := NewService(context.Background(), source, target)
	if err != nil || restored.Status().State != "completed" {
		t.Fatal("completion not restored")
	}
}
