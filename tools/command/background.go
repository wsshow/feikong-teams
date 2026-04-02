package command

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	backgroundBudget = 15 * time.Second
	bgTaskTTL        = 1 * time.Hour // 后台任务结果保留时间
)

// backgroundTask 后台任务
type backgroundTask struct {
	mu      sync.Mutex
	done    bool
	resp    *SmartExecuteResponse
	command string
	startAt time.Time
	doneAt  time.Time
	cancel  context.CancelFunc
}

var (
	bgTasks   = make(map[string]*backgroundTask)
	bgTasksMu sync.Mutex
)

// cleanStaleTasks 清理过期的后台任务，需在持有 bgTasksMu 时调用
func cleanStaleTasks() {
	now := time.Now()
	for id, t := range bgTasks {
		t.mu.Lock()
		if t.done && now.Sub(t.doneAt) > bgTaskTTL {
			t.mu.Unlock()
			delete(bgTasks, id)
			continue
		}
		t.mu.Unlock()
	}
}

// handleTaskOperation 后台任务管理路由
func (t *CommandTools) handleTaskOperation(req *SmartExecuteRequest) (*SmartExecuteResponse, error) {
	switch req.TaskAction {
	case "list":
		return t.listBackgroundTasks()
	case "terminate":
		if req.TaskID == "" {
			return &SmartExecuteResponse{ErrorMessage: "task_id is required for terminate action"}, nil
		}
		return t.terminateBackgroundTask(req.TaskID)
	default:
		if req.TaskID != "" {
			return t.queryBackgroundTask(req.TaskID)
		}
		return &SmartExecuteResponse{ErrorMessage: "invalid task operation"}, nil
	}
}

// listBackgroundTasks 列出所有后台任务
func (t *CommandTools) listBackgroundTasks() (*SmartExecuteResponse, error) {
	bgTasksMu.Lock()
	cleanStaleTasks()
	tasks := make([]BackgroundTaskInfo, 0, len(bgTasks))
	for id, task := range bgTasks {
		task.mu.Lock()
		info := BackgroundTaskInfo{
			TaskID:      id,
			Command:     task.command,
			ElapsedTime: time.Since(task.startAt).Round(time.Millisecond).String(),
		}
		if task.done {
			if task.resp != nil && task.resp.Success {
				info.Status = "completed"
			} else {
				info.Status = "failed"
			}
		} else {
			info.Status = "running"
		}
		task.mu.Unlock()
		tasks = append(tasks, info)
	}
	bgTasksMu.Unlock()

	resp := &SmartExecuteResponse{Success: true, Tasks: tasks}
	if len(tasks) == 0 {
		resp.WarningMessage = "当前没有后台任务"
	}
	return resp, nil
}

// terminateBackgroundTask 终止后台任务
func (t *CommandTools) terminateBackgroundTask(taskID string) (*SmartExecuteResponse, error) {
	bgTasksMu.Lock()
	task, ok := bgTasks[taskID]
	bgTasksMu.Unlock()

	if !ok {
		return &SmartExecuteResponse{ErrorMessage: fmt.Sprintf("task %q not found", taskID)}, nil
	}

	task.mu.Lock()
	if task.done {
		task.mu.Unlock()
		return &SmartExecuteResponse{
			Success:        true,
			Command:        task.command,
			WarningMessage: "任务已结束，无需终止",
		}, nil
	}
	cancelFn := task.cancel
	task.mu.Unlock()

	// 取消 context 触发 cmd.Cancel 杀死进程组
	if cancelFn != nil {
		cancelFn()
	}

	return &SmartExecuteResponse{
		Success:        true,
		Command:        task.command,
		TaskID:         taskID,
		WarningMessage: "已发送终止信号，任务正在停止",
	}, nil
}

// queryBackgroundTask 查询后台任务结果
func (t *CommandTools) queryBackgroundTask(taskID string) (*SmartExecuteResponse, error) {
	bgTasksMu.Lock()
	task, ok := bgTasks[taskID]
	bgTasksMu.Unlock()

	if !ok {
		return &SmartExecuteResponse{ErrorMessage: fmt.Sprintf("task %q not found", taskID)}, nil
	}

	task.mu.Lock()
	if !task.done {
		elapsed := time.Since(task.startAt).Round(time.Millisecond)
		task.mu.Unlock()
		return &SmartExecuteResponse{
			Success:        true,
			Command:        task.command,
			IsBackground:   true,
			TaskID:         taskID,
			WarningMessage: fmt.Sprintf("任务仍在执行中（已运行 %s），请稍后再查询", elapsed),
		}, nil
	}
	resp := task.resp
	task.mu.Unlock()

	// 任务完成，返回结果并清理
	bgTasksMu.Lock()
	delete(bgTasks, taskID)
	bgTasksMu.Unlock()

	return resp, nil
}

// registerAndWaitBackground 注册后台任务并启动等待协程
func (ec *executionContext) registerAndWaitBackground() (string, *backgroundTask) {
	taskID := fmt.Sprintf("bg_%d", time.Now().UnixNano())

	task := &backgroundTask{
		command: ec.req.Command,
		startAt: ec.startTime,
		cancel:  ec.cancel,
	}

	bgTasksMu.Lock()
	cleanStaleTasks()
	bgTasks[taskID] = task
	bgTasksMu.Unlock()

	go func() {
		defer ec.cancel()
		err := <-ec.done
		resp := ec.buildResponse(err)
		resp.IsBackground = true
		resp.TaskID = taskID

		task.mu.Lock()
		task.resp = resp
		task.done = true
		task.doneAt = time.Now()
		task.mu.Unlock()
	}()

	return taskID, task
}
