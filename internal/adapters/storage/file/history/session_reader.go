package eventlog

import (
	"context"
	"fmt"
	"path/filepath"

	domainhistory "fkteams/internal/domain/history"
	domainsession "fkteams/internal/domain/session"
	"fkteams/internal/runtime/pathguard"
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
	sessionDir := filepath.Join(r.sessionsDir, sessionID)
	transcriptFile := filepath.Join(sessionDir, TranscriptFileName)
	if r.manager.Documents == nil {
		// 跨会话读取不能经历史文件或成员目录的符号链接访问存储根以外的数据。
		paths := []string{transcriptFile, filepath.Join(sessionDir, subagentsDirName)}
		for _, path := range paths {
			if err := r.validatePath(path); err != nil {
				return nil, err
			}
		}
		members, err := documentTranscripts(sessionDir)
		if err != nil {
			return nil, err
		}
		for _, path := range members {
			for _, file := range []string{path, filepath.Join(filepath.Dir(path), "metadata.json")} {
				if err := r.validatePath(file); err != nil {
					return nil, err
				}
			}
		}
	}
	// 查询不使用恢复模式，避免读取未完成会话时修复或改写 transcript 尾部。
	transcript, err := loadTranscript(transcriptFile, r.manager.Documents)
	if err != nil {
		return nil, fmt.Errorf("read session history: %w", err)
	}
	return projectTranscriptEvents(sessionDir, transcript, r.manager.Documents)
}

func (r *SessionMessageReader) validatePath(path string) error {
	relative, err := filepath.Rel(r.sessionsDir, path)
	if err != nil {
		return err
	}
	_, err = pathguard.ResolveWorkspace(r.sessionsDir, relative)
	return err
}
