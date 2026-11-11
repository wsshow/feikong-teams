package share

import (
	"encoding/json"
	"fkteams/internal/app/appdata"
	domainsession "fkteams/internal/domain/session"
	storageport "fkteams/internal/ports/storage"
	"fkteams/internal/runtime/log"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const sessionShareFileName = "session_share.json"

// SessionEntry 保存会话分享的授权与展示信息。
type SessionEntry struct {
	SessionID        string    `json:"session_id"`
	Title            string    `json:"title"`
	PasswordHash     string    `json:"password_hash,omitempty"`
	AllowToolDetails bool      `json:"allow_tool_details"`
	MessageCount     int       `json:"message_count"`
	ExpiresAt        time.Time `json:"expires_at,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	LastAccessedAt   time.Time `json:"last_accessed_at,omitempty"`
}

type sessionShareFileEntry struct {
	SessionID        string `json:"session_id"`
	Title            string `json:"title"`
	PasswordHash     string `json:"password_hash,omitempty"`
	AllowToolDetails bool   `json:"allow_tool_details"`
	MessageCount     int    `json:"message_count"`
	ExpiresAt        int64  `json:"expires_at"`
	CreatedAt        int64  `json:"created_at"`
	LastAccessedAt   int64  `json:"last_accessed_at,omitempty"`
}

// SessionShareStore 保存单个 HTTP runtime 的会话分享状态。
type SessionShareStore struct{ *entryStore[SessionEntry] }

// NewSessionShareStore 创建会话分享存储，并从持久化文件加载现有分享。
func NewSessionShareStore(filePath string, stores ...storageport.Documents) *SessionShareStore {
	if filePath == "" {
		filePath = sessionSharesFilePath()
	}
	store := &SessionShareStore{newEntryStore(filePath, cloneSessionEntry, validateSessionShareEntry, encodeSessionEntryMap)}
	if len(stores) > 0 {
		store.Documents = stores[0]
	}
	store.loadErr = store.Load()
	return store
}

func sessionSharesFilePath() string {
	return filepath.Join(appdata.ShareDir(), sessionShareFileName)
}

// Load 从持久化文件加载未过期的会话分享。
func (s *SessionShareStore) Load() error {
	if s == nil {
		return nil
	}
	entries, err := readSessionShareEntries(s.filePath, s.Documents)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("load session shares: %w", err)
	}
	loaded := make(map[string]*SessionEntry, len(entries))
	now := time.Now()
	for id, e := range entries {
		if e == nil {
			return fmt.Errorf("invalid session share entry: %s", id)
		}
		var expiresAt time.Time
		if e.ExpiresAt > 0 {
			expiresAt = time.Unix(e.ExpiresAt, 0)
			if now.After(expiresAt) {
				continue
			}
		}
		var lastAccessedAt time.Time
		if e.LastAccessedAt > 0 {
			lastAccessedAt = time.Unix(e.LastAccessedAt, 0)
		}
		entry := &SessionEntry{
			SessionID:        e.SessionID,
			Title:            e.Title,
			PasswordHash:     e.PasswordHash,
			AllowToolDetails: e.AllowToolDetails,
			MessageCount:     e.MessageCount,
			ExpiresAt:        expiresAt,
			CreatedAt:        time.Unix(e.CreatedAt, 0),
			LastAccessedAt:   lastAccessedAt,
		}
		if err := validateSessionShareEntry(id, entry); err != nil {
			return err
		}
		loaded[id] = entry
	}
	s.mu.Lock()
	s.m = loaded
	s.mu.Unlock()
	return nil
}

func readSessionShareEntries(filePath string, stores ...storageport.Documents) (map[string]*sessionShareFileEntry, error) {
	data, err := readPersistedShareStore(filePath, stores...)
	if err != nil {
		return nil, err
	}
	var entries map[string]*sessionShareFileEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	if len(entries) > maxPersistedShareEntries {
		return nil, errShareStoreFull
	}
	return entries, nil
}

func encodeSessionEntryMap(values map[string]*SessionEntry) ([]byte, error) {
	entries := make(map[string]*sessionShareFileEntry, len(values))
	for id, e := range values {
		entries[id] = &sessionShareFileEntry{
			SessionID:        e.SessionID,
			Title:            e.Title,
			PasswordHash:     e.PasswordHash,
			AllowToolDetails: e.AllowToolDetails,
			MessageCount:     e.MessageCount,
			ExpiresAt:        ExpiresAtUnix(e.ExpiresAt),
			CreatedAt:        e.CreatedAt.Unix(),
			LastAccessedAt:   ExpiresAtUnix(e.LastAccessedAt),
		}
	}
	return json.MarshalIndent(entries, "", "  ")
}

// Touch 按固定间隔持久化访问时间，失败时恢复原值。
func (s *SessionShareStore) Touch(id string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.m[id]
	if entry == nil {
		return nil
	}
	const persistInterval = 5 * time.Minute
	if !entry.LastAccessedAt.IsZero() && at.Sub(entry.LastAccessedAt) < persistInterval {
		return nil
	}
	previous := entry.LastAccessedAt
	entry.LastAccessedAt = at
	if err := s.saveLockedTo(s.filePath); err != nil {
		entry.LastAccessedAt = previous
		return err
	}
	return nil
}

func validateSessionShareEntry(id string, entry *SessionEntry) error {
	if id == "" || entry == nil || !domainsession.ValidID(entry.SessionID) {
		return fmt.Errorf("invalid session share entry")
	}
	if entry.MessageCount < 0 {
		return fmt.Errorf("invalid session share message count")
	}
	return nil
}

// EntryByID 返回独立条目副本，并清理已过期的授权。
func (s *SessionShareStore) EntryByID(id string) (*SessionEntry, bool, bool) {
	entry, exists := s.Lookup(id)
	if !exists {
		return nil, false, false
	}
	if !entry.ExpiresAt.IsZero() && time.Now().After(entry.ExpiresAt) {
		if _, err := s.Delete(id); err != nil {
			log.Printf("failed to persist expired session share cleanup: id=%s, err=%v", id, err)
		}
		return nil, false, true
	}
	return entry, true, false
}

func cloneSessionEntry(entry *SessionEntry) *SessionEntry { copied := *entry; return &copied }
