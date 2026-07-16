package eventlog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"fkteams/internal/domain/apperror"
	domainsession "fkteams/internal/domain/session"
)

// SessionRepository 将文件会话目录适配为会话存储端口。
type SessionRepository struct {
	root string
	mu   sync.RWMutex
}

func NewSessionRepository(root string) *SessionRepository {
	return &SessionRepository{root: root}
}

func (r *SessionRepository) ListSessions(_ context.Context) ([]domainsession.Record, error) {
	if r == nil || r.root == "" {
		return nil, apperror.New(apperror.CodeUnavailable, "session storage is not configured")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	entries, err := os.ReadDir(r.root)
	if errors.Is(err, os.ErrNotExist) {
		return []domainsession.Record{}, nil
	}
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeUnavailable, "session storage unavailable", err)
	}
	records := make([]domainsession.Record, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !domainsession.ValidID(entry.Name()) {
			continue
		}
		sessionDir := filepath.Join(r.root, entry.Name())
		metadata, loadErr := LoadMetadata(sessionDir)
		if loadErr != nil {
			metadata = &domainsession.Metadata{
				ID:     entry.Name(),
				Title:  entry.Name(),
				Status: domainsession.StatusActive,
			}
		}
		record := domainsession.Record{Metadata: *metadata}
		if info, statErr := os.Stat(filepath.Join(sessionDir, TranscriptFileName)); statErr == nil {
			record.Size = info.Size()
			record.ModTime = info.ModTime()
		}
		if record.ModTime.IsZero() {
			record.ModTime = metadata.UpdatedAt
		}
		records = append(records, record)
	}
	return records, nil
}

func (r *SessionRepository) CreateSession(_ context.Context, metadata domainsession.Metadata) (domainsession.Metadata, bool, error) {
	if !domainsession.ValidID(metadata.ID) {
		return domainsession.Metadata{}, false, apperror.New(apperror.CodeInvalidArgument, "invalid session ID")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	sessionDir := filepath.Join(r.root, metadata.ID)
	existing, err := LoadMetadata(sessionDir)
	if err == nil {
		return *existing, false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return domainsession.Metadata{}, false, apperror.Wrap(apperror.CodeUnavailable, "session storage unavailable", err)
	}
	if err := SaveMetadata(sessionDir, &metadata); err != nil {
		return domainsession.Metadata{}, false, apperror.Wrap(apperror.CodeUnavailable, "session storage unavailable", err)
	}
	return metadata, true, nil
}

func (r *SessionRepository) LoadSession(_ context.Context, sessionID string) (domainsession.Metadata, error) {
	if !domainsession.ValidID(sessionID) {
		return domainsession.Metadata{}, apperror.New(apperror.CodeInvalidArgument, "invalid session ID")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	metadata, err := LoadMetadata(filepath.Join(r.root, sessionID))
	if errors.Is(err, os.ErrNotExist) {
		return domainsession.Metadata{}, apperror.New(apperror.CodeNotFound, "session not found")
	}
	if err != nil {
		return domainsession.Metadata{}, apperror.Wrap(apperror.CodeUnavailable, "session storage unavailable", err)
	}
	return *metadata, nil
}

func (r *SessionRepository) UpdateSession(_ context.Context, sessionID string, update func(*domainsession.Metadata) error) (domainsession.Metadata, error) {
	if !domainsession.ValidID(sessionID) {
		return domainsession.Metadata{}, apperror.New(apperror.CodeInvalidArgument, "invalid session ID")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	metadata, err := UpdateMetadata(filepath.Join(r.root, sessionID), false, update)
	if errors.Is(err, os.ErrNotExist) {
		return domainsession.Metadata{}, apperror.New(apperror.CodeNotFound, "session not found")
	}
	if err != nil {
		return domainsession.Metadata{}, apperror.Wrap(apperror.CodeUnavailable, "session storage unavailable", err)
	}
	return *metadata, nil
}

func (r *SessionRepository) DeleteSession(_ context.Context, sessionID string) error {
	if !domainsession.ValidID(sessionID) {
		return apperror.New(apperror.CodeInvalidArgument, "invalid session ID")
	}
	finishDelete, ok := beginSessionDelete(r.root, sessionID)
	if !ok {
		return apperror.New(apperror.CodeConflict, "session is active")
	}
	defer finishDelete()
	r.mu.Lock()
	defer r.mu.Unlock()
	sessionDir := filepath.Join(r.root, sessionID)
	if err := deleteSessionDirectory(sessionDir); errors.Is(err, os.ErrNotExist) {
		return apperror.New(apperror.CodeNotFound, "session not found")
	} else if err != nil {
		return apperror.Wrap(apperror.CodeUnavailable, "session storage unavailable", err)
	}
	return nil
}
