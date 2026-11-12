package eventlog

import (
	"context"
	"errors"
	storageport "fkteams/internal/ports/storage"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	appchat "fkteams/internal/app/chat"
	domainsession "fkteams/internal/domain/session"
)

type ChatSessionStore struct {
	Documents   storageport.Documents
	sessionsDir string
}

func NewChatSessionStore(sessionsDir string, stores ...storageport.Documents) *ChatSessionStore {
	return &ChatSessionStore{sessionsDir: sessionsDir, Documents: selectedDocuments(stores)}
}

func (s *ChatSessionStore) UpdateMetadata(_ context.Context, update appchat.MetadataUpdate) error {
	if !domainsession.ValidID(update.SessionID) {
		return fmt.Errorf("invalid session ID")
	}
	sessionDir := s.sessionDir(update.SessionID)
	now := time.Now()
	_, err := UpdateMetadata(sessionDir, update.CreateIfMissing, func(meta *SessionMetadata) error {
		if meta.ID == "" {
			meta.ID = update.SessionID
			meta.Title = titleFromSource(update.TitleSource, update.DefaultTitle)
			meta.Status = domainsession.Status(update.Status)
			meta.CreatedAt = now
			meta.UpdatedAt = now
		} else {
			meta.UpdatedAt = now
			if update.Status != "" {
				meta.Status = domainsession.Status(update.Status)
			}
			if update.SessionID != domainsession.AssistantID && update.UpdateDefaultTitle && update.TitleSource != "" && isDefaultTitle(meta.Title) {
				meta.Title = truncateTitle(update.TitleSource)
			}
		}
		if update.Mode != nil {
			meta.Mode = strings.TrimSpace(*update.Mode)
		}
		if update.CurrentAgent != nil {
			meta.CurrentAgent = strings.TrimSpace(*update.CurrentAgent)
		}
		return nil
	}, s.Documents)
	if errors.Is(err, os.ErrNotExist) && !update.CreateIfMissing {
		return nil
	}
	return err
}

func (s *ChatSessionStore) sessionDir(sessionID string) string {
	return filepath.Join(s.sessionsDir, sessionID)
}

func titleFromSource(source, fallback string) string {
	if source != "" {
		return truncateTitle(source)
	}
	if fallback != "" {
		return fallback
	}
	return "未命名会话"
}

func isDefaultTitle(title string) bool {
	if title == "" || title == "未命名会话" {
		return true
	}
	_, err := time.Parse("2006-01-02 15:04:05", title)
	return err == nil
}

func truncateTitle(s string) string {
	const maxLen = 50
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
