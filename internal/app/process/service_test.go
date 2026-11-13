package process

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	domain "fkteams/internal/domain/process"
	port "fkteams/internal/ports/process"
)

type memoryStore struct {
	mu      sync.Mutex
	records map[string]domain.Record
	fail    bool
}

func (s *memoryStore) Load() ([]domain.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.Record
	for _, r := range s.records {
		out = append(out, clone(r))
	}
	return out, nil
}
func (s *memoryStore) Save(r domain.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return errors.New("disk unavailable")
	}
	s.records[r.ID] = clone(r)
	return nil
}
func (s *memoryStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, id)
	return nil
}
func (s *memoryStore) setFail(fail bool) { s.mu.Lock(); s.fail = fail; s.mu.Unlock() }

type fakeRunner struct {
	mu      sync.Mutex
	starts  int
	release <-chan struct{}
}
type fakeProcess struct {
	ctx     context.Context
	release <-chan struct{}
}

func (r *fakeRunner) Start(ctx context.Context, _ port.Spec) (port.Process, error) {
	r.mu.Lock()
	r.starts++
	r.mu.Unlock()
	return &fakeProcess{ctx: ctx, release: r.release}, nil
}
func (r *fakeRunner) Read(_ string, _ int64, _ int64, _ int) (port.Output, error) {
	return port.Output{Text: "saved", Total: 5, Next: 5}, nil
}
func (r *fakeRunner) Remove(string) error { return nil }
func (p *fakeProcess) PID() int           { return 123 }
func (p *fakeProcess) Read(_ int64, _ int) (port.Output, error) {
	return port.Output{Text: "live", Total: 4, Next: 4}, nil
}
func (p *fakeProcess) Wait() port.Result {
	<-p.ctx.Done()
	if p.release != nil {
		<-p.release
	}
	return port.Result{ExitCode: -1, Error: p.ctx.Err().Error(), OutputBytes: 5, Stdout: "saved"}
}

func newService(t *testing.T) (*Service, *memoryStore, *fakeRunner, domain.Owner) {
	t.Helper()
	store := &memoryStore{records: make(map[string]domain.Record)}
	runner := &fakeRunner{}
	s, err := NewService(runner, store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	owner := domain.Owner{Workspace: t.TempDir(), SessionID: "session"}
	owner, err = normalizedOwner(owner)
	if err != nil {
		t.Fatal(err)
	}
	return s, store, runner, owner
}

func TestYieldSeparatesCallerCancellationFromExecutionDeadline(t *testing.T) {
	s, _, _, owner := newService(t)
	ctx, cancel := context.WithCancel(context.Background())
	r, err := s.Execute(ctx, Request{Owner: owner, Command: "fixture", Mode: "service", Yield: time.Millisecond})
	if err != nil || r.State != domain.Running || !r.Background {
		t.Fatalf("yield: %+v %v", r, err)
	}
	cancel()
	r, _, err = s.Read(context.Background(), owner, r.ID, 0, 32, 5*time.Millisecond)
	if err != nil || r.State != domain.Running {
		t.Fatalf("caller cancellation killed service: %+v %v", r, err)
	}
	r, err = s.StopTask(context.Background(), owner, r.ID)
	if err != nil || r.State != domain.Cancelled {
		t.Fatalf("stop: %+v %v", r, err)
	}
	r, err = s.Execute(context.Background(), Request{Owner: owner, Command: "fixture", Mode: "job", Timeout: 10 * time.Millisecond, Background: true})
	if err != nil {
		t.Fatal(err)
	}
	r, _, err = s.Read(context.Background(), owner, r.ID, 0, 32, time.Second)
	if err != nil || r.State != domain.TimedOut {
		t.Fatalf("deadline: %+v %v", r, err)
	}
}

func TestAdmissionPersistsBeforeStartingAndCountsOnlyActiveTasks(t *testing.T) {
	s, store, runner, owner := newService(t)
	store.setFail(true)
	if _, err := s.Execute(context.Background(), Request{Owner: owner, Command: "fixture", Mode: "job", Background: true}); err == nil {
		t.Fatal("started without durable intent")
	}
	if runner.starts != 0 {
		t.Fatal("side effect before persistence")
	}
	store.setFail(false)
	for i := range maxRetained {
		r := domain.Record{ID: fmt.Sprint(i), Owner: owner, State: domain.Succeeded, Delivered: true, FinishedAt: time.Now()}
		done := make(chan struct{})
		close(done)
		s.tasks[r.ID] = &task{record: r, done: done}
	}
	for range 8 {
		if _, err := s.Execute(context.Background(), Request{Owner: owner, Command: "fixture", Mode: "service", Background: true}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Execute(context.Background(), Request{Owner: owner, Command: "rejected", Mode: "service", Background: true}); err == nil {
		t.Fatal("session active quota not enforced")
	}
	if runner.starts != 8 {
		t.Fatalf("started %d processes", runner.starts)
	}
}

func TestStopTimeoutKeepsDrainingAndRejectsNewWork(t *testing.T) {
	s, _, runner, owner := newService(t)
	release := make(chan struct{})
	runner.release = release
	r, err := s.Execute(context.Background(), Request{Owner: owner, Command: "fixture", Mode: "service", Background: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if err := s.Stop(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("stop returned %v", err)
	}
	select {
	case <-s.closedDone:
		t.Fatal("cleanup finished before process drained")
	default:
	}
	if _, err := s.Execute(context.Background(), Request{Owner: owner, Command: "forbidden", Mode: "job"}); err == nil {
		t.Fatal("admitted after stop")
	}
	close(release)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	r, _, err = s.Read(context.Background(), owner, r.ID, 0, 32, 0)
	if err != nil || r.State != domain.Interrupted {
		t.Fatalf("shutdown: %+v %v", r, err)
	}
}

func TestCompletionRetriesPersistenceAndDelivery(t *testing.T) {
	s, store, _, owner := newService(t)
	r, err := s.Execute(context.Background(), Request{Owner: owner, Command: "fixture", Mode: "service", Background: true})
	if err != nil {
		t.Fatal(err)
	}
	store.setFail(true)
	r, err = s.StopTask(context.Background(), owner, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	attempts := 0
	s.SetCompletionSink(func(got domain.Record) error {
		attempts++
		if got.ID != r.ID || got.State != domain.Cancelled {
			t.Fatalf("completion: %+v", got)
		}
		if attempts == 1 {
			return errors.New("queue full")
		}
		return nil
	})
	s.maintain()
	if attempts != 0 {
		t.Fatal("delivered before terminal state persisted")
	}
	store.setFail(false)
	s.maintain()
	s.maintain()
	s.maintain()
	if attempts != 2 {
		t.Fatalf("delivery attempts=%d", attempts)
	}
	loaded, _ := store.Load()
	if !loaded[0].Delivered {
		t.Fatal("delivery acknowledgement not persisted")
	}
}

func TestRecoveryMarksInterruptedWithoutReplaying(t *testing.T) {
	owner := domain.Owner{Workspace: t.TempDir(), SessionID: "session"}
	owner, _ = normalizedOwner(owner)
	store := &memoryStore{records: map[string]domain.Record{"old": {ID: "old", Owner: owner, State: domain.Running, PID: 9999, StartedAt: time.Now()}}}
	runner := &fakeRunner{}
	s, err := NewService(runner, store)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r, _, err := s.Read(context.Background(), owner, "old", 0, 32, 0)
	if err != nil || r.State != domain.Interrupted || runner.starts != 0 {
		t.Fatalf("recovery: %+v %v starts=%d", r, err, runner.starts)
	}
}

func TestConcurrentReadListAndStop(t *testing.T) {
	s, _, _, owner := newService(t)
	r, err := s.Execute(context.Background(), Request{Owner: owner, Command: "fixture", Mode: "service", Background: true})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 20 {
				_, _, _ = s.Read(context.Background(), owner, r.ID, 0, 32, 0)
				_, _ = s.List(owner)
			}
		})
	}
	wg.Go(func() { _, _ = s.StopTask(context.Background(), owner, r.ID) })
	wg.Wait()
}

func TestShutdownReturnsTerminalPersistenceFailure(t *testing.T) {
	store := &memoryStore{records: make(map[string]domain.Record)}
	s, err := NewService(&fakeRunner{}, store)
	if err != nil {
		t.Fatal(err)
	}
	owner := domain.Owner{Workspace: t.TempDir(), SessionID: "session"}
	if _, err := s.Execute(context.Background(), Request{Owner: owner, Command: "fixture", Mode: domain.ModeService, Background: true}); err != nil {
		t.Fatal(err)
	}
	store.setFail(true)
	if err := s.Close(); err == nil {
		t.Fatal("shutdown hid terminal persistence failure")
	}
}
