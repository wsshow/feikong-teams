package fkevent

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SessionMetadata 会话元数据，存储于 metadata.json
type SessionMetadata struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SaveMetadata 保存会话元数据到指定目录
func SaveMetadata(sessionDir string, meta *SessionMetadata) error {
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	return os.WriteFile(filepath.Join(sessionDir, "metadata.json"), data, 0644)
}

// LoadMetadata 从指定目录加载会话元数据
func LoadMetadata(sessionDir string) (*SessionMetadata, error) {
	data, err := os.ReadFile(filepath.Join(sessionDir, "metadata.json"))
	if err != nil {
		return nil, err
	}
	var meta SessionMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("unmarshal metadata: %w", err)
	}
	return &meta, nil
}

// SessionHistoryManager 按会话 ID 管理独立的 HistoryRecorder，支持并发安全
type SessionHistoryManager struct {
	mu       sync.RWMutex
	sessions map[string]*HistoryRecorder
}

func NewSessionHistoryManager() *SessionHistoryManager {
	return &SessionHistoryManager{
		sessions: make(map[string]*HistoryRecorder),
	}
}

// GetOrCreate 获取或创建会话的 HistoryRecorder，不存在时尝试从文件加载
func (m *SessionHistoryManager) GetOrCreate(sessionID, historyDir string) *HistoryRecorder {
	m.mu.RLock()
	if recorder, exists := m.sessions[sessionID]; exists {
		m.mu.RUnlock()
		return recorder
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	if recorder, exists := m.sessions[sessionID]; exists {
		return recorder
	}

	recorder := NewHistoryRecorder()
	filePath := filepath.Join(historyDir, sessionID, "history.json")
	if err := recorder.LoadFromFile(filePath); err == nil {
		log.Printf("[SessionManager] loaded session=%s from %s", sessionID, filePath)
	}

	m.sessions[sessionID] = recorder
	return recorder
}

func (m *SessionHistoryManager) Get(sessionID string) *HistoryRecorder {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[sessionID]
}

func (m *SessionHistoryManager) Remove(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, sessionID)
}

// Clear 清空指定会话的历史（不移除 Recorder 实例）
func (m *SessionHistoryManager) Clear(sessionID string) {
	m.mu.RLock()
	recorder, exists := m.sessions[sessionID]
	m.mu.RUnlock()

	if exists {
		recorder.Clear()
	}
}

// LoadForSession 从文件加载历史并替换指定会话的 Recorder
func (m *SessionHistoryManager) LoadForSession(sessionID, filePath string) (*HistoryRecorder, error) {
	recorder := NewHistoryRecorder()
	if err := recorder.LoadFromFile(filePath); err != nil {
		return nil, fmt.Errorf("load session %s: %w", sessionID, err)
	}

	m.mu.Lock()
	m.sessions[sessionID] = recorder
	m.mu.Unlock()

	return recorder, nil
}

func (m *SessionHistoryManager) SaveSession(sessionID, filePath string) error {
	m.mu.RLock()
	recorder, exists := m.sessions[sessionID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	return recorder.SaveToFile(filePath)
}

// GlobalSessionManager Web 和 CLI 共用的全局实例
var GlobalSessionManager = NewSessionHistoryManager()
