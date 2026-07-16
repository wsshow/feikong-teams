package eventlog

import (
	"context"
	"fmt"
	"path/filepath"

	domainhistory "fkteams/internal/domain/history"
	domainsession "fkteams/internal/domain/session"
)

// SessionMessageReader 将文件历史记录适配为存储读取端口。
type SessionMessageReader struct {
	sessionsDir string
	manager     *SessionHistoryManager
}

func NewSessionMessageReader(sessionsDir string, manager *SessionHistoryManager) *SessionMessageReader {
	if manager == nil {
		manager = NewSessionHistoryManager()
	}
	return &SessionMessageReader{sessionsDir: sessionsDir, manager: manager}
}

func (r *SessionMessageReader) LoadSessionMessages(_ context.Context, sessionID string) ([]domainhistory.AgentMessage, error) {
	if r == nil {
		return nil, fmt.Errorf("session message reader is not initialized")
	}
	if !domainsession.ValidID(sessionID) {
		return nil, fmt.Errorf("invalid session ID")
	}
	releaseLease := AcquireSessionLease(r.sessionsDir, sessionID)
	defer releaseLease()
	if recorder := r.manager.Get(sessionID); recorder != nil {
		return recorder.GetMessages(), nil
	}
	recorder := NewHistoryRecorder()
	sessionDir := filepath.Join(r.sessionsDir, sessionID)
	recorder.SetSessionDir(sessionDir)
	transcriptFile := filepath.Join(sessionDir, TranscriptFileName)
	if err := recorder.LoadFromFile(transcriptFile); err != nil {
		return nil, fmt.Errorf("read session history: %w", err)
	}
	return recorder.GetMessages(), nil
}
