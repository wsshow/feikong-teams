package eventlog

import (
	"encoding/json"
	"errors"
	domainsession "fkteams/internal/domain/session"
	"fkteams/internal/runtime/atomicfile"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sync"
)

const defaultSessionRecorderCacheSize = 128

const metadataLockStripeCount = 64

var metadataLockStripes [metadataLockStripeCount]sync.RWMutex

type sessionRecorderEntry struct {
	recorder *HistoryRecorder
	lastUsed uint64
	refs     int
}

// SessionMetadata 保留历史存储包的兼容名称。
type SessionMetadata = domainsession.Metadata

// SaveMetadata 保存会话元数据到指定目录
func SaveMetadata(sessionDir string, meta *SessionMetadata) error {
	lock := metadataLock(sessionDir)
	lock.Lock()
	defer lock.Unlock()
	return saveMetadataUnlocked(sessionDir, meta)
}

func saveMetadataUnlocked(sessionDir string, meta *SessionMetadata) error {
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	return atomicfile.WriteFile(filepath.Join(sessionDir, "metadata.json"), data, 0644)
}

// LoadMetadata 从指定目录加载会话元数据
func LoadMetadata(sessionDir string) (*SessionMetadata, error) {
	lock := metadataLock(sessionDir)
	lock.RLock()
	defer lock.RUnlock()
	return loadMetadataUnlocked(sessionDir)
}

func loadMetadataUnlocked(sessionDir string) (*SessionMetadata, error) {
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

// UpdateMetadata 在同一进程的所有存储实例之间原子执行元数据读改写。
func UpdateMetadata(sessionDir string, createIfMissing bool, update func(*SessionMetadata) error) (*SessionMetadata, error) {
	lock := metadataLock(sessionDir)
	lock.Lock()
	defer lock.Unlock()

	meta, err := loadMetadataUnlocked(sessionDir)
	if errors.Is(err, os.ErrNotExist) && createIfMissing {
		meta = &SessionMetadata{}
		err = nil
	}
	if err != nil {
		return nil, err
	}
	if update != nil {
		if err := update(meta); err != nil {
			return nil, err
		}
	}
	if err := saveMetadataUnlocked(sessionDir, meta); err != nil {
		return nil, err
	}
	copy := *meta
	return &copy, nil
}

func deleteSessionDirectory(sessionDir string) error {
	lock := metadataLock(sessionDir)
	lock.Lock()
	defer lock.Unlock()

	if _, err := os.Stat(sessionDir); err != nil {
		return err
	}
	return os.RemoveAll(sessionDir)
}

func metadataLock(sessionDir string) *sync.RWMutex {
	absPath, err := filepath.Abs(sessionDir)
	if err == nil {
		sessionDir = filepath.Clean(absPath)
	}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(sessionDir))
	return &metadataLockStripes[hash.Sum32()%metadataLockStripeCount]
}

// SessionHistoryManager 按会话 ID 管理独立的 HistoryRecorder，支持并发安全
type SessionHistoryManager struct {
	mu         sync.Mutex
	sessions   map[string]*sessionRecorderEntry
	maxEntries int
	clock      uint64
}

func NewSessionHistoryManager() *SessionHistoryManager {
	return NewSessionHistoryManagerWithCapacity(defaultSessionRecorderCacheSize)
}

func NewSessionHistoryManagerWithCapacity(maxEntries int) *SessionHistoryManager {
	if maxEntries <= 0 {
		maxEntries = defaultSessionRecorderCacheSize
	}
	return &SessionHistoryManager{
		sessions:   make(map[string]*sessionRecorderEntry),
		maxEntries: maxEntries,
	}
}

// GetOrCreate 获取或创建会话的 HistoryRecorder，不存在时尝试从 transcript 加载
func (m *SessionHistoryManager) GetOrCreate(sessionID, historyDir string) *HistoryRecorder {
	if !domainsession.ValidID(sessionID) {
		recorder := NewHistoryRecorder()
		recorder.persistErr = fmt.Errorf("invalid session ID")
		return recorder
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.getOrCreateLocked(sessionID, historyDir)
	m.evictLocked(sessionID)
	return entry.recorder
}

// Acquire 获取 recorder 并在使用期间阻止缓存淘汰。
func (m *SessionHistoryManager) Acquire(sessionID, historyDir string) (*HistoryRecorder, func()) {
	if !domainsession.ValidID(sessionID) {
		return m.GetOrCreate(sessionID, historyDir), func() {}
	}
	releaseLease := AcquireSessionLease(historyDir, sessionID)
	m.mu.Lock()
	entry := m.getOrCreateLocked(sessionID, historyDir)
	entry.refs++
	m.evictLocked("")
	m.mu.Unlock()

	var once sync.Once
	return entry.recorder, func() {
		once.Do(func() {
			m.release(sessionID, entry.recorder)
			releaseLease()
		})
	}
}

func (m *SessionHistoryManager) Get(sessionID string) *HistoryRecorder {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.sessions[sessionID]
	if entry == nil {
		return nil
	}
	m.touchLocked(entry)
	return entry.recorder
}

// Remove 删除空闲 recorder；正在使用时返回 false。
func (m *SessionHistoryManager) Remove(sessionID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if entry := m.sessions[sessionID]; entry != nil && entry.refs > 0 {
		return false
	}
	delete(m.sessions, sessionID)
	return true
}

// Clear 清空指定会话的历史（不移除 Recorder 实例）
func (m *SessionHistoryManager) Clear(sessionID string) {
	m.mu.Lock()
	entry := m.sessions[sessionID]
	if entry != nil {
		entry.refs++
		m.touchLocked(entry)
	}
	m.mu.Unlock()
	if entry != nil {
		entry.recorder.Clear()
		m.release(sessionID, entry.recorder)
	}
}

// LoadForSession 从 transcript 加载历史并替换指定会话的 Recorder
func (m *SessionHistoryManager) LoadForSession(sessionID, filePath string) (*HistoryRecorder, error) {
	if !domainsession.ValidID(sessionID) {
		return nil, fmt.Errorf("invalid session ID")
	}
	recorder := NewHistoryRecorder()
	recorder.SetSessionDir(filepath.Dir(filePath))
	if err := recorder.LoadFromFile(filePath); err != nil {
		return nil, fmt.Errorf("load session %s: %w", sessionID, err)
	}

	m.mu.Lock()
	if current := m.sessions[sessionID]; current != nil && current.refs > 0 {
		m.mu.Unlock()
		return nil, fmt.Errorf("session is active: %s", sessionID)
	}
	entry := &sessionRecorderEntry{recorder: recorder}
	m.touchLocked(entry)
	m.sessions[sessionID] = entry
	m.evictLocked(sessionID)
	m.mu.Unlock()

	return recorder, nil
}

func (m *SessionHistoryManager) SaveSession(sessionID, filePath string) error {
	if !domainsession.ValidID(sessionID) {
		return fmt.Errorf("invalid session ID")
	}
	m.mu.Lock()
	entry := m.sessions[sessionID]
	if entry != nil {
		entry.refs++
		m.touchLocked(entry)
	}
	m.mu.Unlock()
	if entry == nil {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	defer m.release(sessionID, entry.recorder)
	return entry.recorder.SaveToFile(filePath)
}

func (m *SessionHistoryManager) getOrCreateLocked(sessionID, historyDir string) *sessionRecorderEntry {
	if entry := m.sessions[sessionID]; entry != nil {
		m.touchLocked(entry)
		return entry
	}
	sessionDir := filepath.Join(historyDir, sessionID)
	recorder := NewHistoryRecorder()
	recorder.SetSessionDir(sessionDir)
	filePath := filepath.Join(sessionDir, TranscriptFileName)
	if err := recorder.LoadFromFile(filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		recorder.mu.Lock()
		recorder.persistErr = fmt.Errorf("load existing transcript: %w", err)
		recorder.mu.Unlock()
	}
	entry := &sessionRecorderEntry{recorder: recorder}
	m.touchLocked(entry)
	m.sessions[sessionID] = entry
	return entry
}

func (m *SessionHistoryManager) touchLocked(entry *sessionRecorderEntry) {
	m.clock++
	entry.lastUsed = m.clock
}

func (m *SessionHistoryManager) release(sessionID string, recorder *HistoryRecorder) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.sessions[sessionID]
	if entry == nil || entry.recorder != recorder || entry.refs == 0 {
		return
	}
	entry.refs--
	m.touchLocked(entry)
	m.evictLocked("")
}

func (m *SessionHistoryManager) evictLocked(protectedSessionID string) {
	for len(m.sessions) > m.maxEntries {
		var oldestID string
		var oldestUsed uint64
		for sessionID, entry := range m.sessions {
			if sessionID == protectedSessionID || entry.refs > 0 {
				continue
			}
			if oldestID == "" || entry.lastUsed < oldestUsed {
				oldestID = sessionID
				oldestUsed = entry.lastUsed
			}
		}
		if oldestID == "" {
			return
		}
		delete(m.sessions, oldestID)
	}
}
