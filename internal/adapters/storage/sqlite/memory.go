package sqlite

import (
	"context"
	"encoding/json"
	domainmemory "fkteams/internal/domain/memory"
	"fmt"
)

// MemoryStore 保存完整记忆快照，事务失败时保留上次提交。
type MemoryStore struct{ database *Database }

// NewMemoryStore 创建共享应用数据库的记忆存储。
func NewMemoryStore(database *Database) *MemoryStore { return &MemoryStore{database: database} }

// Load 按原顺序恢复记忆，并保留稳定身份、来源及命中统计。
func (s *MemoryStore) Load(limit int) ([]domainmemory.MemoryEntry, error) {
	entries := []domainmemory.MemoryEntry{}
	if limit <= 0 {
		return entries, nil
	}
	rows, err := s.database.db.Query("SELECT payload FROM memories ORDER BY position LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var entry domainmemory.MemoryEntry
		if err := json.Unmarshal([]byte(payload), &entry); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// Save 在一个事务中替换记忆，重复或缺失 ID 会使整个快照回滚。
func (s *MemoryStore) Save(entries []domainmemory.MemoryEntry) error {
	tx, err := s.database.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM memories"); err != nil {
		return err
	}
	for position, entry := range entries {
		if entry.ID == "" {
			return fmt.Errorf("memory ID is required")
		}
		if !domainmemory.AllMemoryTypes[entry.Type] {
			return fmt.Errorf("invalid memory type %q", entry.Type)
		}
		payload, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		if _, err = tx.Exec("INSERT INTO memories(id,position,payload) VALUES(?,?,?)", entry.ID, position, string(payload)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
