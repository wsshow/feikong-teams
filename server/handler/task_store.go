package handler

import (
	"log"
	"sync"
	"time"
)

const (
	// taskGracePeriod 断连后任务继续运行的宽限期
	taskGracePeriod = 60 * time.Second
	// maxEventBuffer 断连期间最大缓冲事件数
	maxEventBuffer = 1000
)

// activeTask 全局活跃任务，生命周期独立于 WebSocket 连接
type activeTask struct {
	sessionID string
	cancel    func() // 任务取消函数

	mu          sync.Mutex
	writer      func(any) error  // 当前 WS writer，断连时为 nil
	writerEpoch uint64           // writer 版本号，每次绑定新 writer 递增
	eventBuffer []map[string]any // 断连期间缓冲的事件
	approvalCh  chan any         // 审批/ask 通道
	graceTimer  *time.Timer      // 断连宽限期计时器
	done        bool             // 任务已完成
}

// Write 线程安全地写入事件：有连接时直接推送，断连时缓冲
func (t *activeTask) Write(event any) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.writer != nil {
		return t.writer(event)
	}
	// 断连中，缓冲事件
	if m, ok := event.(map[string]any); ok && len(t.eventBuffer) < maxEventBuffer {
		t.eventBuffer = append(t.eventBuffer, m)
	}
	return nil
}

// Detach 分离 writer（连接断开时调用），启动宽限期计时器
// 传入 expectedEpoch 以防止新连接已 Reattach 后被旧连接的清理覆盖
func (t *activeTask) Detach(expectedEpoch uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.writerEpoch != expectedEpoch {
		return // writer 已被 Reattach 更新，不再 detach
	}
	t.writer = nil
	if t.done {
		return
	}
	t.graceTimer = time.AfterFunc(taskGracePeriod, func() {
		t.mu.Lock()
		if t.writerEpoch != expectedEpoch {
			t.mu.Unlock()
			return
		}
		// 在锁内标记完成，防止并发的 Reattach 在锁释放后成功
		t.done = true
		t.mu.Unlock()
		log.Printf("task grace period expired, cancelling: session=%s", t.sessionID)
		t.cancel()
		globalTaskStore.RemoveIfMatch(t.sessionID, t)
	})
}

// Reattach 重新绑定 writer（连接恢复时调用），回放缓冲事件
// 持锁完成回放以保证事件顺序。返回 false 表示任务已完成/过期无法恢复
func (t *activeTask) Reattach(writer func(any) error) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.done {
		return false
	}

	// 停止宽限期计时器
	if t.graceTimer != nil {
		t.graceTimer.Stop()
		t.graceTimer = nil
	}

	// 递增 epoch，使旧连接的 Detach 成为空操作
	t.writerEpoch++

	// 先回放缓冲事件，再设置 writer
	// 这样并发 Write 在回放完成前只会继续缓冲，不会插入
	for _, event := range t.eventBuffer {
		_ = writer(event)
	}
	t.eventBuffer = nil
	t.writer = writer
	return true
}

// MarkDone 标记任务完成
func (t *activeTask) MarkDone() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.done = true
	if t.graceTimer != nil {
		t.graceTimer.Stop()
		t.graceTimer = nil
	}
}

// IsDone 线程安全地检查任务是否已完成
func (t *activeTask) IsDone() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.done
}

// GetApprovalCh 获取审批通道
func (t *activeTask) GetApprovalCh() chan any {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.approvalCh
}

// taskStore 全局任务注册表
type taskStore struct {
	mu    sync.Mutex
	tasks map[string]*activeTask
}

var globalTaskStore = &taskStore{tasks: make(map[string]*activeTask)}

// Register 注册新任务。如果同一 session 已有运行中的任务，先取消旧任务。
func (s *taskStore) Register(sessionID string, cancel func(), writer func(any) error) *activeTask {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 取消同一 session 的旧任务
	if old, exists := s.tasks[sessionID]; exists {
		old.cancel()
		old.mu.Lock()
		if old.graceTimer != nil {
			old.graceTimer.Stop()
		}
		old.mu.Unlock()
	}

	task := &activeTask{
		sessionID:  sessionID,
		cancel:     cancel,
		writer:     writer,
		approvalCh: make(chan any, 1),
	}
	s.tasks[sessionID] = task
	return task
}

// Get 获取活跃任务
func (s *taskStore) Get(sessionID string) *activeTask {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tasks[sessionID]
}

// Remove 移除任务
func (s *taskStore) Remove(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tasks, sessionID)
}

// RemoveIfMatch 仅当存储的 task 与给定指针一致时才移除（防止误删新任务）
func (s *taskStore) RemoveIfMatch(sessionID string, task *activeTask) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tasks[sessionID] == task {
		delete(s.tasks, sessionID)
	}
}

// CancelAll 取消所有活跃任务（服务关闭时调用）
func (s *taskStore) CancelAll() {
	s.mu.Lock()
	tasks := make([]*activeTask, 0, len(s.tasks))
	for _, t := range s.tasks {
		tasks = append(tasks, t)
	}
	s.tasks = make(map[string]*activeTask)
	s.mu.Unlock()
	for _, t := range tasks {
		t.cancel()
	}
}

// DetachAll 分离指定 session 列表的所有任务（连接断开时调用）
func (s *taskStore) DetachAll(sessionIDs []string) {
	type taskWithEpoch struct {
		task  *activeTask
		epoch uint64
	}

	s.mu.Lock()
	items := make([]taskWithEpoch, 0, len(sessionIDs))
	for _, sid := range sessionIDs {
		if t, exists := s.tasks[sid]; exists {
			t.mu.Lock()
			items = append(items, taskWithEpoch{task: t, epoch: t.writerEpoch})
			t.mu.Unlock()
		}
	}
	s.mu.Unlock()

	for _, item := range items {
		item.task.Detach(item.epoch)
	}
}
