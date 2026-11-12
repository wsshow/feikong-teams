// Package memory 管理由用户和助手主动维护的长期记忆。
package memory

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	memoryport "fkteams/internal/ports/memory"
)

const (
	defaultMaxEntries = 500
	defaultMaxChars   = 24000
	contextMaxChars   = 4000
)

// Config 限制记忆容量；达到上限时要求显式整理，不自动淘汰。
type Config struct {
	MaxEntries int
	MaxChars   int
}

// Usage 描述真实存储容量，便于助手主动整理。
type Usage struct {
	Entries       int `json:"entries"`
	Characters    int `json:"characters"`
	MaxEntries    int `json:"max_entries"`
	MaxCharacters int `json:"max_characters"`
}

// Manager 持有应用实例内唯一的记忆快照；每次变更先持久化再发布。
// 不启动定时器、后台提取或额外模型调用。
type Manager struct {
	enabled    atomic.Bool
	mu         sync.RWMutex
	store      memoryport.Store
	entries    []MemoryEntry
	bm25       *BM25
	maxEntries int
	maxChars   int
}

// NewManager 加载存储并建立只读检索索引，不依赖模型。
func NewManager(store memoryport.Store, cfg *Config) (*Manager, error) {
	if store == nil {
		return nil, fmt.Errorf("memory store is required")
	}
	entries, err := store.Load(math.MaxInt)
	if err != nil {
		return nil, fmt.Errorf("load memory: %w", err)
	}
	m := &Manager{store: store, entries: cloneEntries(entries), maxEntries: defaultMaxEntries, maxChars: defaultMaxChars}
	if cfg != nil {
		if cfg.MaxEntries > 0 {
			m.maxEntries = cfg.MaxEntries
		}
		if cfg.MaxChars > 0 {
			m.maxChars = cfg.MaxChars
		}
	}
	m.enabled.Store(true)
	m.rebuildIndex()
	return m, nil
}

// SetEnabled 控制模型记忆读写及上下文注入，管理界面仍可维护已有数据。
func (m *Manager) SetEnabled(enabled bool) { m.enabled.Store(enabled) }

// Enabled 返回当前是否允许模型使用记忆。
func (m *Manager) Enabled() bool { return m.enabled.Load() }

// List 返回独立的记忆快照。
func (m *Manager) List() []MemoryEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneEntries(m.entries)
}

// Count 返回已保存的记忆条数。
func (m *Manager) Count() int { m.mu.RLock(); defer m.mu.RUnlock(); return len(m.entries) }

// Usage 返回真实存储用量。
func (m *Manager) Usage() Usage {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return Usage{len(m.entries), entryCharacters(m.entries), m.maxEntries, m.maxChars}
}

// Search 构造有界背景：全局偏好优先，其余按相关性检索。
// 场景记忆需显式 Recall，不会自动扩散到其他项目。
func (m *Manager) Search(query string, topK int) []MemoryEntry {
	if !m.Enabled() {
		return nil
	}
	if topK <= 0 {
		topK = 5
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]MemoryEntry, 0)
	seen := make(map[string]bool)
	budget := contextMaxChars
	add := func(e MemoryEntry) {
		size := entryCharacters([]MemoryEntry{e})
		if e.Scope != "" || seen[e.ID] || len(result) >= topK || size > budget {
			return
		}
		result = append(result, cloneEntry(e))
		seen[e.ID] = true
		budget -= size
	}
	for i := len(m.entries) - 1; i >= 0; i-- {
		e := m.entries[i]
		if e.Type == Preference || e.Type == Feedback {
			add(e)
		}
	}
	for _, match := range m.bm25.Search(query, m.entries, 0) {
		add(*match.Entry)
	}
	return result
}

// Recall 分页查询可管理的记忆；scope 非空时仅返回该范围与全局记录。
func (m *Manager) Recall(query, scope string, offset, limit int) ([]MemoryEntry, int, bool) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	query, scope = strings.TrimSpace(query), strings.TrimSpace(scope)
	m.mu.RLock()
	defer m.mu.RUnlock()
	candidates := make([]MemoryEntry, 0)
	add := func(e MemoryEntry) {
		if scope == "" || e.Scope == "" || e.Scope == scope {
			candidates = append(candidates, e)
		}
	}
	if query == "" {
		for i := len(m.entries) - 1; i >= 0; i-- {
			add(m.entries[i])
		}
	} else {
		for _, match := range m.bm25.Search(query, m.entries, 0) {
			add(*match.Entry)
		}
	}
	offset = min(offset, len(candidates))
	end := offset + min(limit, len(candidates)-offset)
	return cloneEntries(candidates[offset:end]), end, end < len(candidates)
}

// Delete 精确删除摘要匹配项，持久化失败时返回错误且保留原数据。
func (m *Manager) Delete(summary string) (int, error) {
	summary = strings.TrimSpace(summary)
	m.mu.Lock()
	defer m.mu.Unlock()
	next := make([]MemoryEntry, 0, len(m.entries))
	for _, e := range m.entries {
		if e.Summary != summary {
			next = append(next, e)
		}
	}
	count := len(m.entries) - len(next)
	if count == 0 {
		return 0, nil
	}
	if err := m.commit(next); err != nil {
		return 0, err
	}
	return count, nil
}

// Clear 原子清空记忆；没有后台提取会重新填充历史信息。
func (m *Manager) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.commit([]MemoryEntry{})
}

func (m *Manager) commit(next []MemoryEntry) error {
	if err := m.store.Save(next); err != nil {
		return fmt.Errorf("save memory: %w", err)
	}
	m.entries = cloneEntries(next)
	m.rebuildIndex()
	return nil
}

func (m *Manager) rebuildIndex() { m.bm25 = &BM25{}; m.bm25.Build(m.entries) }

func cloneEntry(e MemoryEntry) MemoryEntry {
	e.Tags = slices.Clone(e.Tags)
	if e.LastHitAt != nil {
		value := *e.LastHitAt
		e.LastHitAt = &value
	}
	return e
}
func cloneEntries(entries []MemoryEntry) []MemoryEntry {
	result := make([]MemoryEntry, len(entries))
	for i, e := range entries {
		result[i] = cloneEntry(e)
	}
	return result
}
func entryCharacters(entries []MemoryEntry) int {
	size := 0
	for _, e := range entries {
		size += utf8.RuneCountInString(e.Scope + e.Summary + e.Detail + strings.Join(e.Tags, ""))
	}
	return size
}
