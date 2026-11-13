// Package process 管理会话所属命令的状态、配额、执行期限和完成投递。
package process

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"time"

	domain "fkteams/internal/domain/process"
	port "fkteams/internal/ports/process"
	"github.com/google/uuid"
)

const maxActive = 64
const maxRetained = 256
const retention = 7 * 24 * time.Hour

// Request 将等待预算与实际执行期限分开；Service 模式默认不设执行期限。
type Request struct {
	Owner      domain.Owner
	Command    string
	Mode       domain.Mode
	Timeout    time.Duration
	Yield      time.Duration
	Background bool
}

type task struct {
	mu      sync.Mutex
	record  domain.Record
	process port.Process
	cancel  context.CancelCauseFunc
	done    chan struct{}
	dirty   bool
}

// Service 是应用实例持有的任务注册表，进程操作通过端口实现。
type Service struct {
	mu         sync.Mutex
	tasks      map[string]*task
	runner     port.Runner
	store      port.Store
	ctx        context.Context
	cancel     context.CancelFunc
	closed     bool
	started    bool
	wg         sync.WaitGroup
	closeOnce  sync.Once
	closeErr   error
	closedDone chan struct{}
	sink       func(domain.Record) error
}

var errStopped = errors.New("process stopped")

// NewService 恢复记录；中断任务只更新状态，禁止重放副作用或按旧 PID 发信号。
func NewService(runner port.Runner, store port.Store) (*Service, error) {
	if runner == nil || store == nil {
		return nil, fmt.Errorf("process runner and store are required")
	}
	records, err := store.Load()
	if err != nil {
		return nil, fmt.Errorf("load process records: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{tasks: make(map[string]*task), runner: runner, store: store, ctx: ctx, cancel: cancel, closedDone: make(chan struct{})}
	for _, r := range records {
		if r.State == domain.Running {
			r.Background = true
			r.State, r.Error, r.FinishedAt = domain.Interrupted, "application stopped before command completion; verify effects before retrying", time.Now()
			if err := store.Save(r); err != nil {
				cancel()
				return nil, err
			}
		}
		done := make(chan struct{})
		close(done)
		s.tasks[r.ID] = &task{record: r, done: done}
	}
	if err := s.prune(); err != nil {
		cancel()
		return nil, err
	}
	return s, nil
}

// Name 返回生命周期服务名。
func (s *Service) Name() string { return "processes" }

// Start 将应用取消传递给所属进程，并启动持久化重试与保留期清理。
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("process service is closed")
	}
	if s.started {
		return nil
	}
	s.started = true
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				s.cancel()
				return
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				s.maintain()
			}
		}
	}()
	return nil
}

// SetCompletionSink 设置应用队列投递器；失败的投递保留记录并重试。
func (s *Service) SetCompletionSink(sink func(domain.Record) error) {
	s.mu.Lock()
	s.sink = sink
	s.mu.Unlock()
}

func normalizedOwner(owner domain.Owner) (domain.Owner, error) {
	if owner.Workspace == "" {
		return owner, fmt.Errorf("workspace is required")
	}
	dir, err := filepath.Abs(owner.Workspace)
	if err != nil {
		return owner, err
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return owner, err
	}
	owner.Workspace = filepath.Clean(dir)
	return owner, nil
}

// Execute 等待命令完成或耗尽等待预算；转后台后归应用所有，不继承回合取消。
func (s *Service) Execute(ctx context.Context, req Request) (domain.Record, error) {
	if err := ctx.Err(); err != nil {
		return domain.Record{}, err
	}
	owner, err := normalizedOwner(req.Owner)
	if err != nil {
		return domain.Record{}, err
	}
	if req.Command == "" || len(req.Command) > 16<<10 || (req.Mode != domain.ModeJob && req.Mode != domain.ModeService) || req.Timeout < 0 || req.Timeout > 24*time.Hour || req.Yield < 0 || req.Yield > 30*time.Second {
		return domain.Record{}, fmt.Errorf("invalid process request")
	}
	if req.Timeout == 0 && req.Mode == domain.ModeJob {
		req.Timeout = 120 * time.Second
	}
	if err := s.prune(); err != nil {
		return domain.Record{}, err
	}
	s.mu.Lock()
	if s.closed || s.ctx.Err() != nil {
		s.mu.Unlock()
		return domain.Record{}, fmt.Errorf("process service is closed")
	}
	active, owned := 0, 0
	for _, t := range s.tasks {
		t.mu.Lock()
		if t.record.State == domain.Running {
			active++
			if t.record.Owner == owner {
				owned++
			}
		}
		t.mu.Unlock()
	}
	if active >= maxActive || owned >= 8 || len(s.tasks) >= maxRetained+maxActive {
		s.mu.Unlock()
		return domain.Record{}, fmt.Errorf("process quota reached; stop active tasks or acknowledge completed results")
	}
	runCtx, cancel := context.WithCancelCause(s.ctx)
	var timeoutCancel context.CancelFunc = func() {}
	if req.Timeout > 0 {
		runCtx, timeoutCancel = context.WithTimeout(runCtx, req.Timeout)
	}
	r := domain.Record{ID: "proc_" + uuid.NewString(), Owner: owner, Command: req.Command, Mode: req.Mode, State: domain.Running, StartedAt: time.Now()}
	t := &task{record: r, cancel: cancel, done: make(chan struct{})}
	// 先提交意图，再启动进程，避免返回一个无法追踪的副作用。
	if err := s.store.Save(r); err != nil {
		s.mu.Unlock()
		cancel(err)
		timeoutCancel()
		return domain.Record{}, err
	}
	if err := ctx.Err(); err != nil {
		cancel(err)
		timeoutCancel()
		r.State, r.Error, r.FinishedAt, r.Delivered = domain.Cancelled, err.Error(), time.Now(), true
		t.record = r
		t.dirty = s.store.Save(r) != nil
		close(t.done)
		s.tasks[r.ID] = t
		s.mu.Unlock()
		return r, nil
	}
	p, startErr := s.runner.Start(runCtx, port.Spec{ID: r.ID, Command: r.Command, Directory: owner.Workspace})
	if startErr != nil {
		cancel(startErr)
		timeoutCancel()
		r.State, r.Error, r.FinishedAt, r.Delivered = domain.Failed, startErr.Error(), time.Now(), true
		t.record, t.dirty = r, true
		t.dirty = s.store.Save(r) != nil
		close(t.done)
		s.tasks[r.ID] = t
		s.mu.Unlock()
		return r, nil
	}
	r.PID = p.PID()
	t.record, t.process = r, p
	t.dirty = s.store.Save(r) != nil
	s.tasks[r.ID] = t
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		defer timeoutCancel()
		defer cancel(nil)
		result := p.Wait()
		t.mu.Lock()
		r := t.record
		r.OutputBytes, r.Stdout, r.Stderr = result.OutputBytes, result.Stdout, result.Stderr
		r.ExitCode, r.Error, r.FinishedAt = &result.ExitCode, result.Error, time.Now()
		switch {
		case errors.Is(runCtx.Err(), context.DeadlineExceeded):
			r.State, r.Error = domain.TimedOut, "command execution deadline exceeded"
		case errors.Is(context.Cause(runCtx), errStopped):
			r.State, r.Error = domain.Cancelled, "command stopped"
		case runCtx.Err() != nil:
			r.State, r.Error = domain.Interrupted, "application stopped command"
		case result.ExitCode == 0 && result.Error == "":
			r.State = domain.Succeeded
		default:
			r.State = domain.Failed
		}
		t.record, t.process = r, nil
		t.dirty = s.store.Save(r) != nil
		t.mu.Unlock()
		close(t.done)
	}()
	if ctx.Err() != nil {
		return s.cancelWait(t)
	}
	if req.Background || req.Yield == 0 {
		return s.detach(t)
	}
	timer := time.NewTimer(req.Yield)
	defer timer.Stop()
	select {
	case <-t.done:
		t.mu.Lock()
		defer t.mu.Unlock()
		t.record.Delivered = true
		t.dirty = s.store.Save(t.record) != nil
		return clone(t.record), nil
	case <-timer.C:
		if ctx.Err() != nil {
			return s.cancelWait(t)
		}
		return s.detach(t)
	case <-ctx.Done():
		return s.cancelWait(t)
	}
}

func (s *Service) cancelWait(t *task) (domain.Record, error) {
	t.cancel(errStopped)
	<-t.done
	t.mu.Lock()
	defer t.mu.Unlock()
	t.record.Delivered = true
	t.dirty = s.store.Save(t.record) != nil
	return clone(t.record), nil
}

func clone(r domain.Record) domain.Record {
	if r.ExitCode != nil {
		code := *r.ExitCode
		r.ExitCode = &code
	}
	return r
}

func (s *Service) detach(t *task) (domain.Record, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.record.Background = true
	if t.record.State != domain.Running {
		t.record.Delivered = true
	}
	t.dirty = s.store.Save(t.record) != nil
	return clone(t.record), nil
}

func (s *Service) find(owner domain.Owner, id string) (*task, error) {
	owner, err := normalizedOwner(owner)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.tasks[id]
	if t == nil {
		return nil, fmt.Errorf("task not found in current session and workspace")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.record.Owner != owner {
		return nil, fmt.Errorf("task not found in current session and workspace")
	}
	return t, nil
}

// Read 返回任务状态和一页日志；wait 只等待终态，不改变进程的执行期限。
func (s *Service) Read(ctx context.Context, owner domain.Owner, id string, cursor int64, limit int, wait time.Duration) (domain.Record, port.Output, error) {
	if cursor < -1 || limit < 1 || limit > 64<<10 || wait < 0 || wait > 30*time.Second {
		return domain.Record{}, port.Output{}, fmt.Errorf("invalid read cursor, limit or wait")
	}
	t, err := s.find(owner, id)
	if err != nil {
		return domain.Record{}, port.Output{}, err
	}
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return domain.Record{}, port.Output{}, ctx.Err()
		case <-t.done:
		case <-timer.C:
		}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	r := clone(t.record)
	var out port.Output
	if t.process != nil {
		out, err = t.process.Read(cursor, limit)
	} else {
		out, err = s.runner.Read(id, r.OutputBytes, cursor, limit)
	}
	if err != nil && r.PID == 0 {
		err = nil
	}
	if err == nil && r.State != domain.Running && !t.record.Delivered {
		t.record.Delivered = true
		t.dirty = s.store.Save(t.record) != nil
	}
	return r, out, err
}

// StopTask 幂等请求停止进程，不按调用方传入的 PID 操作。
func (s *Service) StopTask(ctx context.Context, owner domain.Owner, id string) (domain.Record, error) {
	t, err := s.find(owner, id)
	if err != nil {
		return domain.Record{}, err
	}
	t.mu.Lock()
	if t.cancel != nil && t.record.State == domain.Running {
		t.cancel(errStopped)
	}
	t.mu.Unlock()
	select {
	case <-ctx.Done():
		return domain.Record{}, ctx.Err()
	case <-t.done:
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return clone(t.record), nil
}

// List 仅列出同一会话和工作区的任务，按开始时间排序。
func (s *Service) List(owner domain.Owner) ([]domain.Record, error) {
	owner, err := normalizedOwner(owner)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var records []domain.Record
	for _, t := range s.tasks {
		t.mu.Lock()
		if t.record.Owner == owner {
			records = append(records, clone(t.record))
		}
		t.mu.Unlock()
	}
	sort.Slice(records, func(i, j int) bool { return records[i].StartedAt.Before(records[j].StartedAt) })
	return records, nil
}

func (s *Service) maintain() {
	s.mu.Lock()
	tasks := make([]*task, 0, len(s.tasks))
	for _, t := range s.tasks {
		tasks = append(tasks, t)
	}
	sink := s.sink
	s.mu.Unlock()
	for _, t := range tasks {
		t.mu.Lock()
		if t.dirty {
			t.dirty = s.store.Save(t.record) != nil
		}
		r := clone(t.record)
		ready := !t.dirty && r.Background && r.State != domain.Running && !r.Delivered && r.Owner.SessionID != "" && sink != nil
		t.mu.Unlock()
		if ready && sink(r) == nil {
			t.mu.Lock()
			t.record.Delivered = true
			t.dirty = s.store.Save(t.record) != nil
			t.mu.Unlock()
		}
	}
	_ = s.prune()
}

func (s *Service) prune() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var eligible []domain.Record
	for _, t := range s.tasks {
		t.mu.Lock()
		if t.record.State != domain.Running && t.record.Delivered && !t.dirty {
			eligible = append(eligible, clone(t.record))
		}
		t.mu.Unlock()
	}
	sort.Slice(eligible, func(i, j int) bool { return eligible[i].FinishedAt.Before(eligible[j].FinishedAt) })
	for _, r := range eligible {
		if len(s.tasks) <= maxRetained && time.Since(r.FinishedAt) < retention {
			continue
		}
		t := s.tasks[r.ID]
		t.mu.Lock()
		err := s.store.Delete(r.ID)
		if err == nil {
			err = s.runner.Remove(r.ID)
		}
		t.mu.Unlock()
		if err != nil {
			return err
		}
		delete(s.tasks, r.ID)
	}

	return nil
}

// Stop 只限制调用方等待；后台仍继续取消、排空和提交终态。
func (s *Service) Stop(ctx context.Context) error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.cancel()
		s.mu.Unlock()
		go func() {
			s.wg.Wait()
			s.mu.Lock()
			defer s.mu.Unlock()
			for _, t := range s.tasks {
				t.mu.Lock()
				if t.dirty {
					s.closeErr = errors.Join(s.closeErr, s.store.Save(t.record))
				}
				t.mu.Unlock()
			}
			close(s.closedDone)
		}()
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.closedDone:
		return s.closeErr
	}
}

// Close 等待所有进程和收尾真实结束，日志保留到正常保留期清理。
func (s *Service) Close() error { return s.Stop(context.Background()) }
