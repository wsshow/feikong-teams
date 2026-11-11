package memory

import (
	"errors"
	domainmemory "fkteams/internal/domain/memory"
	"fmt"
	"github.com/google/uuid"
	"strings"
	"time"
)

// ErrInvalidEntry 表示记忆字段不合法。
var ErrInvalidEntry = errors.New("invalid memory entry")

// ErrEntryNotFound 表示指定记忆不存在。
var ErrEntryNotFound = errors.New("memory entry not found")

// EditRequest 描述用户可编辑的记忆字段，ID 为空时新增。
type EditRequest struct {
	ID      string                  `json:"id"`
	Type    domainmemory.MemoryType `json:"type"`
	Summary string                  `json:"summary"`
	Detail  string                  `json:"detail"`
	Tags    []string                `json:"tags"`
}

// SaveEntry 校验并持久化用户编辑，失败时不发布内存变更。
func (m *Manager) SaveEntry(req EditRequest) (MemoryEntry, error) {
	req.Summary = strings.TrimSpace(req.Summary)
	if !domainmemory.AllMemoryTypes[req.Type] || req.Summary == "" || strings.ContainsAny(req.Summary, "\r\n") || len(req.Summary) > 4096 || len(req.Detail) > 65536 || len(req.Tags) > 64 {
		return MemoryEntry{}, ErrInvalidEntry
	}
	for _, tag := range req.Tags {
		if len(tag) > 256 || strings.ContainsAny(tag, "\r\n,") {
			return MemoryEntry{}, ErrInvalidEntry
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	next := append([]MemoryEntry(nil), m.entries...)
	index := -1
	for i := range next {
		if next[i].ID == req.ID {
			index = i
			break
		}
	}
	if req.ID != "" && index < 0 {
		return MemoryEntry{}, ErrEntryNotFound
	}
	if index < 0 {
		if len(next) >= m.maxEntries {
			return MemoryEntry{}, fmt.Errorf("%w: capacity reached", ErrInvalidEntry)
		}
		next = append(next, MemoryEntry{ID: uuid.NewString(), CreatedAt: time.Now()})
		index = len(next) - 1
	}
	entry := next[index]
	entry.Type = req.Type
	entry.Summary = req.Summary
	entry.Detail = req.Detail
	entry.Tags = append([]string(nil), req.Tags...)
	next[index] = entry
	if err := m.store.Save(next); err != nil {
		return MemoryEntry{}, fmt.Errorf("save memory: %w", err)
	}
	m.entries = next
	m.dirty = false
	m.rebuildIndex()
	return entry, nil
}

// DeleteEntry 以稳定 ID 删除一条记忆，保存失败时保留原状态。
func (m *Manager) DeleteEntry(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := make([]MemoryEntry, 0, len(m.entries))
	found := false
	for _, entry := range m.entries {
		if entry.ID == id {
			found = true
			continue
		}
		next = append(next, entry)
	}
	if !found {
		return ErrEntryNotFound
	}
	if err := m.store.Save(next); err != nil {
		return fmt.Errorf("delete memory: %w", err)
	}
	m.entries = next
	m.dirty = false
	m.rebuildIndex()
	return nil
}
