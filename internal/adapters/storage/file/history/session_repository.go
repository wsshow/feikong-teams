package eventlog

import (
	"context"
	"fkteams/internal/runtime/log"
	"fkteams/internal/runtime/pathguard"
	"strings"

	"errors"
	storageport "fkteams/internal/ports/storage"
	"os"
	"path/filepath"
	"sync"

	"fkteams/internal/domain/apperror"
	domainsession "fkteams/internal/domain/session"
)

// SessionRepository 将文件会话目录适配为会话存储端口。
type SessionRepository struct {
	Documents storageport.Documents
	root      string
	mu        sync.RWMutex
}

func NewSessionRepository(root string, stores ...storageport.Documents) *SessionRepository {
	return &SessionRepository{root: root, Documents: selectedDocuments(stores)}
}

func (r *SessionRepository) ListSessions(_ context.Context) ([]domainsession.Record, error) {
	if r == nil || r.root == "" {
		return nil, apperror.New(apperror.CodeUnavailable, "session storage is not configured")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Documents != nil {
		return r.listDatabaseSessions()
	}
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
		if _, err := pathguard.ResolveWorkspace(r.root, filepath.Join(entry.Name(), "metadata.json")); err != nil {
			return nil, err
		}
		metadata, loadErr := LoadMetadata(sessionDir, r.Documents)
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
	existing, err := LoadMetadata(sessionDir, r.Documents)
	if err == nil {
		return *existing, false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return domainsession.Metadata{}, false, apperror.Wrap(apperror.CodeUnavailable, "session storage unavailable", err)
	}
	if err := SaveMetadata(sessionDir, &metadata, r.Documents); err != nil {
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
	if r.Documents == nil {
		if _, err := pathguard.ResolveWorkspace(r.root, filepath.Join(sessionID, "metadata.json")); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return domainsession.Metadata{}, apperror.New(apperror.CodeNotFound, "session not found")
			}
			return domainsession.Metadata{}, err
		}
	}
	metadata, err := LoadMetadata(filepath.Join(r.root, sessionID), r.Documents)
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
	metadata, err := UpdateMetadata(filepath.Join(r.root, sessionID), false, update, r.Documents)
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
	if r.Documents != nil {
		if _, err := LoadMetadata(sessionDir, r.Documents); errors.Is(err, os.ErrNotExist) {
			return apperror.New(apperror.CodeNotFound, "session not found")
		} else if err != nil {
			return err
		}
		if err := r.Documents.RemoveTree(sessionDir); err != nil {
			return err
		}
		// 数据库删除已经提交，附件清理失败不应让调用方重复提交删除。
		if err := deleteSessionDirectory(sessionDir); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("session attachment cleanup failed: %v", err)
		}
		return nil
	}
	if err := deleteSessionDirectory(sessionDir); errors.Is(err, os.ErrNotExist) {
		return apperror.New(apperror.CodeNotFound, "session not found")
	} else if err != nil {
		return apperror.Wrap(apperror.CodeUnavailable, "session storage unavailable", err)
	}
	return nil
}

// ClearSessionHistory 在空闲边界清除会话内容，仅保留身份元数据。
func (r *SessionRepository) ClearSessionHistory(_ context.Context, sessionID string) error {
	if !domainsession.ValidID(sessionID) {
		return apperror.New(apperror.CodeInvalidArgument, "invalid session ID")
	}
	finish, ok := beginSessionDelete(r.root, sessionID)
	if !ok {
		return apperror.New(apperror.CodeConflict, "session is active")
	}
	defer finish()
	r.mu.Lock()
	defer r.mu.Unlock()
	// 即使使用数据库，也先验证实体目录，避免清理附件时沿符号链接越界。
	root, err := os.OpenRoot(r.root)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := pathguard.RejectRootSymlinkComponents(root, sessionID); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return apperror.New(apperror.CodeNotFound, "session not found")
		}
		return err
	}
	if info, err := root.Lstat(filepath.Join(sessionID, "metadata.json")); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return apperror.New(apperror.CodeInvalidArgument, "symbolic links are not allowed")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	sessionDir := filepath.Join(r.root, sessionID)
	if _, err := LoadMetadata(sessionDir, r.Documents); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return apperror.New(apperror.CodeNotFound, "session not found")
		}
		return err
	}
	if r.Documents != nil {
		if err := r.Documents.RemoveTree(sessionDir, filepath.Join(sessionDir, "metadata.json")); err != nil {
			return err
		}
		// 数据库提交成功后，实体附件仅作垃圾回收，不影响清空结果。
		if err := clearSessionDirectory(sessionDir); err != nil {
			log.Printf("session attachment cleanup failed: %v", err)
		}
		return nil
	}
	return clearSessionDirectory(sessionDir)
}

func clearSessionDirectory(sessionDir string) error {
	root, err := os.OpenRoot(sessionDir)
	if err != nil {
		return err
	}
	defer root.Close()
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == "metadata.json" {
			continue
		}
		if err := root.RemoveAll(entry.Name()); err != nil {
			return err
		}
	}
	return nil
}

func (r *SessionRepository) listDatabaseSessions() ([]domainsession.Record, error) {
	paths, err := r.Documents.List(r.root)
	if err != nil {
		return nil, err
	}
	records := []domainsession.Record{}
	for _, path := range paths {
		rel, err := filepath.Rel(r.root, path)
		if err != nil {
			return nil, err
		}
		if filepath.Base(path) != "metadata.json" || strings.Count(filepath.ToSlash(rel), "/") != 1 {
			continue
		}
		meta, err := LoadMetadata(filepath.Dir(path), r.Documents)
		if err != nil {
			return nil, err
		}
		modified := meta.UpdatedAt
		records = append(records, domainsession.Record{Metadata: *meta, ModTime: modified})
	}
	return records, nil
}
