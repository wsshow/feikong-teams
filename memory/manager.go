package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Manager 记忆管理器
type Manager struct {
	mu        sync.RWMutex
	storePath string
	store     MemoryStore
	bm25      *BM25
	llm       LLMClient
}

// NewManager 创建记忆管理器
func NewManager(workspaceDir string, llmClient LLMClient) *Manager {
	m := &Manager{
		storePath: filepath.Join(workspaceDir, "memory", "index.json"),
		bm25:      &BM25{},
		llm:       llmClient,
	}
	m.load()
	m.rebuildIndex()
	return m
}

// ExtractAndStore 提取记忆并存储（设计为异步调用，由调用方 go 启动）
func (m *Manager) ExtractAndStore(ctx context.Context, messages []Message, sessionID string) {
	entries, err := Extract(ctx, messages, sessionID, m.llm)
	if err != nil {
		fmt.Printf("[memory] warn: extract failed: %v\n", err)
		return
	}
	if len(entries) == 0 {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	added := 0
	for _, entry := range entries {
		if !m.isDuplicate(entry) {
			m.store.Entries = append(m.store.Entries, entry)
			added++
		}
	}

	if added > 0 {
		m.rebuildIndex()
		if err := m.save(); err != nil {
			fmt.Printf("[memory] warn: save failed: %v\n", err)
		} else {
			fmt.Printf("[memory] saved %d new entries to %s\n", added, m.storePath)
		}
	}
}

// Search 检索记忆
func (m *Manager) Search(query string, topK int) []MemoryEntry {
	m.mu.RLock()
	results := m.bm25.Search(query, m.store.Entries, topK)
	entries := make([]MemoryEntry, len(results))
	hitIDs := make([]string, len(results))
	for i, r := range results {
		entries[i] = *r.Entry
		hitIDs[i] = r.Entry.ID
	}
	m.mu.RUnlock()

	// 异步更新命中统计
	if len(hitIDs) > 0 {
		go m.updateHitStats(hitIDs)
	}

	return entries
}

// updateHitStats 更新命中统计
func (m *Manager) updateHitStats(ids []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	hitSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		hitSet[id] = true
	}

	for i := range m.store.Entries {
		if hitSet[m.store.Entries[i].ID] {
			m.store.Entries[i].HitCount++
			m.store.Entries[i].LastHitAt = &now
		}
	}

	if err := m.save(); err != nil {
		fmt.Printf("[memory] warn: save hit stats failed: %v\n", err)
	}
}

// isDuplicate 判断新条目是否与已有条目重复（需在锁内调用）
func (m *Manager) isDuplicate(entry MemoryEntry) bool {
	for _, existing := range m.store.Entries {
		if existing.Type != entry.Type {
			continue
		}
		// Summary 包含关系
		if strings.Contains(existing.Summary, entry.Summary) || strings.Contains(entry.Summary, existing.Summary) {
			return true
		}
		// Tags 重叠比例超过 2/3
		if len(entry.Tags) > 0 && len(existing.Tags) > 0 {
			overlap := 0
			existingTags := make(map[string]bool, len(existing.Tags))
			for _, t := range existing.Tags {
				existingTags[strings.ToLower(t)] = true
			}
			for _, t := range entry.Tags {
				if existingTags[strings.ToLower(t)] {
					overlap++
				}
			}
			minLen := len(entry.Tags)
			if len(existing.Tags) < minLen {
				minLen = len(existing.Tags)
			}
			if float64(overlap)/float64(minLen) > 2.0/3.0 {
				return true
			}
		}
	}
	return false
}

// rebuildIndex 重建 BM25 索引（需在锁内调用）
func (m *Manager) rebuildIndex() {
	m.bm25 = &BM25{}
	m.bm25.Build(m.store.Entries)
}

// load 从文件加载
func (m *Manager) load() {
	data, err := os.ReadFile(m.storePath)
	if err != nil {
		m.store = MemoryStore{Version: "1", Entries: []MemoryEntry{}}
		return
	}
	if err := json.Unmarshal(data, &m.store); err != nil {
		fmt.Printf("[memory] warn: failed to parse store file: %v\n", err)
		m.store = MemoryStore{Version: "1", Entries: []MemoryEntry{}}
	}
}

// save 保存到文件（需在锁内调用）
func (m *Manager) save() error {
	dir := filepath.Dir(m.storePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	data, err := json.MarshalIndent(m.store, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal store: %w", err)
	}
	return os.WriteFile(m.storePath, data, 0644)
}
