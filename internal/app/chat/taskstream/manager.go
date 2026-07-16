package taskstream

import (
	"context"
	"fkteams/internal/runtime/log"
	"sync"
	"time"
)

// Manager 全局任务流注册表，管理所有活跃和已完成的任务流。
type Manager struct {
	mu            sync.Mutex
	streams       map[string]*Stream
	cleanupMu     sync.Mutex
	cleanupCancel context.CancelFunc
}

// NewManager 创建新的 Manager
func NewManager() *Manager {
	return &Manager{streams: make(map[string]*Stream)}
}

// Register 注册新的任务流。如果同一 session 已有活跃流，先取消旧流。
func (m *Manager) Register(cfg StreamConfig) *Stream {
	m.mu.Lock()
	old := m.streams[cfg.SessionID]
	s := &Stream{
		config:      cfg,
		status:      "processing",
		createdAt:   time.Now(),
		interruptCh: make(chan any, 1),
		manager:     m,
	}
	m.streams[cfg.SessionID] = s
	m.mu.Unlock()

	// Cancel 可能触发外部回调，不能在注册表锁内执行。
	if old != nil {
		old.Cancel()
	}
	return s
}

// Get 获取指定 session 的流（可能是活跃或已完成的）
func (m *Manager) Get(sessionID string) *Stream {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.streams[sessionID]
}

// RemoveIfMatch 仅当存储的流与给定指针一致时才移除（防止误删新流）
func (m *Manager) RemoveIfMatch(sessionID string, stream *Stream) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.streams[sessionID] == stream {
		delete(m.streams, sessionID)
	}
}

// Remove 无条件移除指定 session 的流
func (m *Manager) Remove(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.streams, sessionID)
}

// UnsubscribeAll 批量解绑指定 session 的 Push 订阅者。
func (m *Manager) UnsubscribeAll(items []UnsubscribeItem) {
	for _, item := range items {
		item.Stream.Unsubscribe(item.ID)
	}
}

// UnsubscribeItem 描述一次 Unsubscribe 操作所需的信息
type UnsubscribeItem struct {
	Stream *Stream
	ID     SubscriptionID
}

// CancelAll 取消所有活跃流（服务关闭时调用）
func (m *Manager) CancelAll() {
	m.mu.Lock()
	streams := make([]*Stream, 0, len(m.streams))
	for _, s := range m.streams {
		streams = append(streams, s)
	}
	m.streams = make(map[string]*Stream)
	m.mu.Unlock()

	for _, s := range streams {
		s.Cancel()
	}
}

// CancelAndRemove 取消并移除指定 session 的流（会话删除时调用）
func (m *Manager) CancelAndRemove(sessionID string) {
	m.mu.Lock()
	s := m.streams[sessionID]
	if s != nil {
		delete(m.streams, sessionID)
	}
	m.mu.Unlock()

	if s != nil {
		s.Cancel()
	}
}

// StartCleanup 启动后台清理协程，定期移除已完成且超过 TTL 的流。
// 重复调用会替换已有清理协程。
func (m *Manager) StartCleanup(ctx context.Context, interval time.Duration) {
	if m == nil || interval <= 0 {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	cleanupCtx, cancel := context.WithCancel(ctx)
	m.cleanupMu.Lock()
	previous := m.cleanupCancel
	m.cleanupCancel = cancel
	m.cleanupMu.Unlock()
	if previous != nil {
		previous()
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-cleanupCtx.Done():
				return
			case <-ticker.C:
				m.cleanup()
			}
		}
	}()
}

// StopCleanup 停止后台清理协程。
func (m *Manager) StopCleanup() {
	if m == nil {
		return
	}
	m.cleanupMu.Lock()
	cancel := m.cleanupCancel
	m.cleanupCancel = nil
	m.cleanupMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (m *Manager) cleanup() {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()

	for sid, s := range m.streams {
		s.mu.Lock()
		shouldRemove := s.done && (s.config.CleanupTTL <= 0 || now.Sub(s.doneAt) >= s.config.CleanupTTL)
		s.mu.Unlock()
		if shouldRemove {
			log.Printf("[taskstream] cleanup expired stream: session=%s", sid)
			delete(m.streams, sid)
		}
	}
}
