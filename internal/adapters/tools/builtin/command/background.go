package command

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"fkteams/internal/domain/session"
	"fkteams/internal/runtime/executil"

	"github.com/google/uuid"
)

const (
	backgroundThreshold = 15 * time.Second
	bgTaskTTL           = time.Hour
	maxBackgroundTasks  = 64
)

// Manager 统一持有前台、显式后台和自动转后台命令，不共享进程级状态。
type Manager struct {
	mu        sync.Mutex
	tasks     map[string]*backgroundTask
	files     []string
	ctx       context.Context
	cancel    context.CancelFunc
	closed    bool
	wg        sync.WaitGroup
	closeOnce sync.Once
	closeErr  error
}

type backgroundTask struct {
	mu       sync.Mutex
	id       string
	scope    string
	done     bool
	resp     *SmartExecuteResponse
	command  string
	startAt  time.Time
	doneAt   time.Time
	cancel   context.CancelFunc
	finished chan struct{}
	pid      int
}

// NewManager 创建独立命令任务管理器，调用 Close 排空其所属任务。
func NewManager() *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{tasks: make(map[string]*backgroundTask), ctx: ctx, cancel: cancel}
}

// Closed 报告管理器是否已进入关闭流程。
func (m *Manager) Closed() bool { m.mu.Lock(); defer m.mu.Unlock(); return m.closed }

// Close 拒绝新命令、取消并等待所有子进程退出，再清理本实例拥有的文件。
func (m *Manager) Close() error {
	m.closeOnce.Do(func() {
		m.mu.Lock()
		m.closed = true
		m.cancel()
		m.mu.Unlock()
		m.wg.Wait()
		m.mu.Lock()
		files := append([]string(nil), m.files...)
		m.tasks = make(map[string]*backgroundTask)
		m.mu.Unlock()
		for _, path := range files {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				m.closeErr = errors.Join(m.closeErr, err)
			}
		}
	})
	return m.closeErr
}

func scopeFor(ctx context.Context, workDir string) string {
	id, _ := session.IDFromContext(ctx)
	return filepath.Clean(workDir) + "\x00" + id
}

func (m *Manager) reserveLocked() bool {
	now := time.Now()
	oldest := ""
	var oldestTime time.Time
	for id, task := range m.tasks {
		task.mu.Lock()
		done, at := task.done, task.doneAt
		task.mu.Unlock()
		if done && now.Sub(at) > bgTaskTTL {
			delete(m.tasks, id)
			continue
		}
		if done && (oldest == "" || at.Before(oldestTime)) {
			oldest, oldestTime = id, at
		}
	}
	if len(m.tasks) < maxBackgroundTasks {
		return true
	}
	if oldest != "" {
		delete(m.tasks, oldest)
		return true
	}
	return false
}

func (m *Manager) start(ctx context.Context, workDir string, req *SmartExecuteRequest, eval SecurityEvaluation, timeout time.Duration) (*backgroundTask, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// 拷贝请求，后台任务不保留调用方可变参数。
	request := *req
	ec := newExecutionContext(m.ctx, &request, eval, timeout, workDir)
	shell, args := buildShellCommand(request.Command)
	cmd := exec.CommandContext(ec.cmdCtx, shell, args...)
	cmd.Dir = workDir
	executil.ConfigureProcessGroup(cmd)
	cmd.Stdout, cmd.Stderr = ec.stdoutLW, ec.stderrLW
	ec.startTime = time.Now()
	task := &backgroundTask{id: "bg_" + uuid.NewString(), scope: scopeFor(ctx, workDir), command: request.Command, startAt: ec.startTime, cancel: ec.cancel, finished: make(chan struct{})}
	m.mu.Lock()
	if m.closed || !m.reserveLocked() {
		m.mu.Unlock()
		ec.cancel()
		return nil, fmt.Errorf("command manager is closed or too many background tasks are running")
	}
	if err := ctx.Err(); err != nil {
		m.mu.Unlock()
		ec.cancel()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		m.mu.Unlock()
		ec.cancel()
		return nil, fmt.Errorf("start command: %w", err)
	}
	task.pid = cmd.Process.Pid
	m.tasks[task.id] = task
	m.wg.Add(1)
	m.mu.Unlock()
	go func() {
		defer m.wg.Done()
		defer ec.cancel()
		err := cmd.Wait()
		resp := ec.buildResponse(err)
		resp.TaskID = task.id
		resp.PID = task.pid
		resp.IsBackground = request.Background
		if resp.OutputFilePath != "" {
			m.mu.Lock()
			m.files = append(m.files, filepath.Join(workDir, resp.OutputFilePath))
			m.mu.Unlock()
		}
		task.mu.Lock()
		task.done = true
		task.doneAt = time.Now()
		task.resp = resp
		task.mu.Unlock()
		close(task.finished)
	}()
	return task, nil
}

func (task *backgroundTask) result() *SmartExecuteResponse {
	task.mu.Lock()
	defer task.mu.Unlock()
	copy := *task.resp
	return &copy
}
func (task *backgroundTask) runningResponse() *SmartExecuteResponse {
	task.mu.Lock()
	defer task.mu.Unlock()
	if task.done {
		copy := *task.resp
		return &copy
	}
	return &SmartExecuteResponse{Success: true, Command: task.command, IsBackground: true, TaskID: task.id, PID: task.pid, WarningMessage: "命令正在后台运行，使用 task_id 查询状态或终止任务"}
}

func (t *CommandTools) handleTaskOperation(ctx context.Context, req *SmartExecuteRequest) (*SmartExecuteResponse, error) {
	scope := scopeFor(ctx, t.workDir)
	m := t.manager
	m.mu.Lock()
	if req.TaskAction == "list" {
		tasks := make([]BackgroundTaskInfo, 0)
		for id, task := range m.tasks {
			if task.scope != scope {
				continue
			}
			task.mu.Lock()
			status := "running"
			elapsed := time.Since(task.startAt)
			if task.done {
				elapsed = task.doneAt.Sub(task.startAt)
				status = "failed"
				if task.resp != nil && task.resp.Success {
					status = "completed"
				}
			}
			tasks = append(tasks, BackgroundTaskInfo{TaskID: id, Command: task.command, Status: status, ElapsedTime: elapsed.Round(time.Millisecond).String()})
			task.mu.Unlock()
		}
		m.mu.Unlock()
		sort.Slice(tasks, func(i, j int) bool { return tasks[i].TaskID < tasks[j].TaskID })
		return &SmartExecuteResponse{Success: true, Tasks: tasks}, nil
	}
	task := m.tasks[req.TaskID]
	if task == nil || task.scope != scope {
		m.mu.Unlock()
		return &SmartExecuteResponse{ErrorMessage: "task not found in current session"}, nil
	}
	m.mu.Unlock()
	switch req.TaskAction {
	case "terminate":
		task.cancel()
		return &SmartExecuteResponse{Success: true, TaskID: task.id, WarningMessage: "已发送终止信号，使用 status 确认退出"}, nil
	case "status", "":
		return task.runningResponse(), nil
	default:
		return &SmartExecuteResponse{ErrorMessage: "invalid task operation"}, nil
	}
}
