package memory

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	domainmemory "fkteams/internal/domain/memory"
	"github.com/google/uuid"
)

// ErrInvalidEntry 表示字段无效或超出容量。
var ErrInvalidEntry = errors.New("invalid memory entry")

// ErrEntryNotFound 表示指定记忆不存在。
var ErrEntryNotFound = errors.New("memory entry not found")

// ErrConflict 表示记忆已经被其他对话修改，需要重新读取。
var ErrConflict = errors.New("memory changed; recall the current entry before editing")

// EditRequest 保存完整单条记忆，更新必须携带读取到的版本。
type EditRequest struct {
	ID        string                  `json:"id"`
	Version   uint64                  `json:"version"`
	Type      domainmemory.MemoryType `json:"type"`
	Scope     string                  `json:"scope,omitempty"`
	Summary   string                  `json:"summary"`
	Detail    string                  `json:"detail"`
	Tags      []string                `json:"tags"`
	SessionID string                  `json:"-"`
}

// SaveEntry 精确新增或更新，不使用相似度覆盖，也不自动淘汰其他事实。
func (m *Manager) SaveEntry(req EditRequest) (MemoryEntry, error) {
	req.Summary, req.Scope = strings.TrimSpace(req.Summary), strings.TrimSpace(req.Scope)
	if !domainmemory.AllMemoryTypes[req.Type] || req.Summary == "" || len(req.Summary) > 1024 || len(req.Detail) > 8192 || len(req.Scope) > 256 || len(req.Tags) > 32 || strings.ContainsAny(req.Summary+req.Scope, "\r\n") {
		return MemoryEntry{}, ErrInvalidEntry
	}
	for _, value := range append([]string{req.Summary, req.Scope, req.Detail}, req.Tags...) {
		if strings.IndexFunc(value, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\t' }) >= 0 {
			return MemoryEntry{}, ErrInvalidEntry
		}
	}
	for _, tag := range req.Tags {
		if len(tag) > 128 || strings.ContainsAny(tag, "\r\n,") {
			return MemoryEntry{}, ErrInvalidEntry
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	next := cloneEntries(m.entries)
	index := -1
	for i, e := range next {
		if req.ID != "" && e.ID == req.ID {
			index = i
		}
		if req.ID == "" && e.Scope == req.Scope && e.Type == req.Type && e.Summary == req.Summary {
			if e.Detail == req.Detail && strings.Join(e.Tags, "\x00") == strings.Join(req.Tags, "\x00") {
				return cloneEntry(e), nil
			}
			return MemoryEntry{}, fmt.Errorf("%w: summary already exists with ID %s", ErrConflict, e.ID)
		}
	}
	if req.ID != "" && index < 0 {
		return MemoryEntry{}, ErrEntryNotFound
	}
	if index >= 0 && (req.Version == 0 || req.Version != next[index].Version) {
		return MemoryEntry{}, ErrConflict
	}
	if index < 0 {
		if req.Version != 0 {
			return MemoryEntry{}, ErrConflict
		}
		next = append(next, MemoryEntry{ID: uuid.NewString(), CreatedAt: time.Now()})
		index = len(next) - 1
	}
	entry := next[index]
	entry.Version++
	entry.UpdatedAt = time.Now()
	entry.Type, entry.Scope, entry.Summary, entry.Detail = req.Type, req.Scope, req.Summary, req.Detail
	entry.Tags = append([]string(nil), req.Tags...)
	if req.SessionID != "" {
		entry.SessionID = req.SessionID
	}
	next[index] = entry
	// 容量满时仍允许缩小现有内容，以便整理，而不悄悄删除用户记忆。
	if (len(next) > m.maxEntries && len(next) > len(m.entries)) || (entryCharacters(next) > m.maxChars && entryCharacters(next) >= entryCharacters(m.entries)) {
		return MemoryEntry{}, fmt.Errorf("%w: memory capacity reached; consolidate or remove entries first", ErrInvalidEntry)
	}
	if err := m.commit(next); err != nil {
		return MemoryEntry{}, err
	}
	return cloneEntry(entry), nil
}

// DeleteEntry 由管理入口按稳定 ID 删除。
func (m *Manager) DeleteEntry(id string) error { return m.deleteEntry(id, 0) }

// Forget 删除模型读到的特定版本，拒绝丢失并发更新。
func (m *Manager) Forget(id string, version uint64) error {
	if version == 0 {
		return ErrConflict
	}
	return m.deleteEntry(id, version)
}
func (m *Manager) deleteEntry(id string, version uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := make([]MemoryEntry, 0, len(m.entries))
	found := false
	for _, e := range m.entries {
		if e.ID == id {
			if version != 0 && e.Version != version {
				return ErrConflict
			}
			found = true
		} else {
			next = append(next, e)
		}
	}
	if !found {
		return ErrEntryNotFound
	}
	return m.commit(next)
}
