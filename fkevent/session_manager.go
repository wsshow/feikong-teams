package fkevent

import (
	"fmt"
	"log"
	"sync"
)

// SessionHistoryManager 按会话ID管理多个 HistoryRecorder 实例
// 解决全局单例 GlobalHistoryRecorder 在多会话场景下的数据污染和并发问题
type SessionHistoryManager struct {
	mu       sync.RWMutex
	sessions map[string]*HistoryRecorder
}

// NewSessionHistoryManager 创建会话历史管理器
func NewSessionHistoryManager() *SessionHistoryManager {
	return &SessionHistoryManager{
		sessions: make(map[string]*HistoryRecorder),
	}
}

// GetOrCreate 获取或创建指定会话的 HistoryRecorder
// 如果会话不存在，自动创建一个新的 Recorder 并尝试从文件加载历史
func (m *SessionHistoryManager) GetOrCreate(sessionID, historyDir string) *HistoryRecorder {
	m.mu.RLock()
	if recorder, exists := m.sessions[sessionID]; exists {
		m.mu.RUnlock()
		return recorder
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	// 双重检查
	if recorder, exists := m.sessions[sessionID]; exists {
		return recorder
	}

	recorder := NewHistoryRecorder()

	// 尝试从文件加载该会话的历史记录
	filePath := fmt.Sprintf("%sfkteams_chat_history_%s", historyDir, sessionID)
	if err := recorder.LoadFromFile(filePath); err == nil {
		log.Printf("[SessionManager] loaded history for session=%s from %s", sessionID, filePath)
	} else {
		log.Printf("[SessionManager] new session=%s (no history file)", sessionID)
	}

	m.sessions[sessionID] = recorder
	return recorder
}

// Get 获取指定会话的 HistoryRecorder，如果不存在返回 nil
func (m *SessionHistoryManager) Get(sessionID string) *HistoryRecorder {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[sessionID]
}

// Remove 移除指定会话的 HistoryRecorder
func (m *SessionHistoryManager) Remove(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, sessionID)
}

// Clear 清除指定会话的历史记录（不从 map 中移除，只清空内容）
func (m *SessionHistoryManager) Clear(sessionID string) {
	m.mu.RLock()
	recorder, exists := m.sessions[sessionID]
	m.mu.RUnlock()

	if exists {
		recorder.Clear()
		log.Printf("[SessionManager] cleared history for session=%s", sessionID)
	}
}

// ClearAll 清除所有会话记录
func (m *SessionHistoryManager) ClearAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions = make(map[string]*HistoryRecorder)
	log.Println("[SessionManager] cleared all session histories")
}

// LoadForSession 为指定会话从文件加载历史记录
// 如果该会话的 Recorder 已存在，会用文件数据替换内存数据
func (m *SessionHistoryManager) LoadForSession(sessionID, filePath string) (*HistoryRecorder, error) {
	recorder := NewHistoryRecorder()
	if err := recorder.LoadFromFile(filePath); err != nil {
		return nil, fmt.Errorf("load history for session %s: %w", sessionID, err)
	}

	m.mu.Lock()
	m.sessions[sessionID] = recorder
	m.mu.Unlock()

	log.Printf("[SessionManager] loaded history for session=%s from %s", sessionID, filePath)
	return recorder, nil
}

// SaveSession 保存指定会话的历史记录到文件
func (m *SessionHistoryManager) SaveSession(sessionID, filePath string) error {
	m.mu.RLock()
	recorder, exists := m.sessions[sessionID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	return recorder.SaveToFile(filePath)
}

// GlobalSessionManager 全局会话历史管理器实例（用于 Web 服务）
var GlobalSessionManager = NewSessionHistoryManager()
