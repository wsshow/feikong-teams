package storage

import (
	"context"

	domainsession "fkteams/internal/domain/session"
)

// SessionRepository 管理会话元数据和资源生命周期。
type SessionRepository interface {
	ListSessions(ctx context.Context) ([]domainsession.Record, error)
	CreateSession(ctx context.Context, metadata domainsession.Metadata) (domainsession.Metadata, bool, error)
	LoadSession(ctx context.Context, sessionID string) (domainsession.Metadata, error)
	UpdateSession(ctx context.Context, sessionID string, update func(*domainsession.Metadata) error) (domainsession.Metadata, error)
	DeleteSession(ctx context.Context, sessionID string) error
}
