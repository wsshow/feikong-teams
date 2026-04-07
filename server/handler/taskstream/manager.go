package taskstream

import (
	"log"
	"sync"
	"time"
)

// Manager 全局任务流注册表，管理所有活跃和已完成的任务流。
type Manager struct {
	mu      sync.Mutex
	streams map[string]*Stream
}

// NewManager 创建新的 Manager
func NewManager() *Manager {
	return &Manager{streams: make(map[string]*Stream)}
}

// Register 注册新的任务流。如果同一 session 已有活跃流，先取消旧流。
func (m *Manager) Register(cfg StreamConfig) *Stream {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 取消同一 session 的旧流
	if old, exists := m.streams[cfg.SessionID]; exists {
		old.mu.Lock()
		if old.graceTimer != nil {
			old.graceTimer.Stop()
			old.graceTimer = nil
		}
		old.mu.Unlock()
		old.Cancel()
	}

	s := &Stream{
		config:      cfg,
		status:      "processing",
		createdAt:   time.Now(),
		interruptCh: make(chan any, 1),
		manager:     m,
	}
	m.streams[cfg.SessionID] = s
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
// 调用方需提供每个 session 在 Subscribe 时获得的 epoch，确保不会误解绑新连接的订阅者。
func (m *Manager) UnsubscribeAll(items []UnsubscribeItem) {
	for _, item := range items {
		item.Stream.Unsubscribe(item.Epoch)
	}
}

// UnsubscribeItem 描述一次 Unsubscribe 操作所需的信息
type UnsubscribeItem struct {
	Stream *Stream
	Epoch  uint64
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
		s.mu.Lock()
		if s.graceTimer != nil {
			s.graceTimer.Stop()
			s.graceTimer = nil
		}
		s.mu.Unlock()
		s.Cancel()
	}
}

// StartCleanup 启动后台清理协程，定期移除已完成且超过 TTL 的流。
func (m *Manager) StartCleanup(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			m.cleanup()
		}
	}()
}

func (m *Manager) cleanup() {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()

	for sid, s := range m.streams {
		s.mu.Lock()
		shouldRemove := s.done && s.config.CleanupTTL > 0 && now.Sub(s.doneAt) > s.config.CleanupTTL
		s.mu.Unlock()
		if shouldRemove {
			log.Printf("[taskstream] cleanup expired stream: session=%s", sid)
			delete(m.streams, sid)
		}
	}
}
