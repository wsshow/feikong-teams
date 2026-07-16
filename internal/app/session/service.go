// Package session 提供会话资源管理用例。
package session

import (
	"context"
	"strings"
	"time"

	"fkteams/internal/domain/apperror"
	domainsession "fkteams/internal/domain/session"
	storageport "fkteams/internal/ports/storage"
)

const defaultTitle = "未命名会话"

type Service struct {
	repository storageport.SessionRepository
	now        func() time.Time
}

type CreateRequest struct {
	SessionID string
	Title     string
}

type UpdateRequest struct {
	SessionID    string
	Title        *string
	Favorite     *bool
	CurrentAgent *string
}

func NewService(repository storageport.SessionRepository) *Service {
	return &Service{repository: repository, now: time.Now}
}

func (s *Service) requireRepository() (storageport.SessionRepository, error) {
	if s == nil || s.repository == nil {
		return nil, apperror.New(apperror.CodeUnavailable, "session service is not initialized")
	}
	return s.repository, nil
}

func (s *Service) List(ctx context.Context) ([]domainsession.Record, error) {
	repository, err := s.requireRepository()
	if err != nil {
		return nil, err
	}
	return repository.ListSessions(ctx)
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (domainsession.Metadata, bool, error) {
	repository, err := s.requireRepository()
	if err != nil {
		return domainsession.Metadata{}, false, err
	}
	if req.SessionID == "" {
		req.SessionID = domainsession.NewID()
	}
	if !domainsession.ValidID(req.SessionID) {
		return domainsession.Metadata{}, false, apperror.New(apperror.CodeInvalidArgument, "invalid session ID")
	}
	now := s.now()
	metadata := domainsession.Metadata{
		ID:        req.SessionID,
		Title:     NormalizeTitle(req.Title),
		Status:    domainsession.StatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
	return repository.CreateSession(ctx, metadata)
}

func (s *Service) Get(ctx context.Context, sessionID string) (domainsession.Metadata, error) {
	repository, err := s.requireRepository()
	if err != nil {
		return domainsession.Metadata{}, err
	}
	return repository.LoadSession(ctx, sessionID)
}

func (s *Service) Update(ctx context.Context, req UpdateRequest) (domainsession.Metadata, error) {
	repository, err := s.requireRepository()
	if err != nil {
		return domainsession.Metadata{}, err
	}
	if req.Title == nil && req.Favorite == nil && req.CurrentAgent == nil {
		return domainsession.Metadata{}, apperror.New(apperror.CodeInvalidArgument, "at least one session field is required")
	}
	if req.Title != nil {
		if strings.TrimSpace(*req.Title) == "" {
			return domainsession.Metadata{}, apperror.New(apperror.CodeInvalidArgument, "session title is required")
		}
	}
	return repository.UpdateSession(ctx, req.SessionID, func(metadata *domainsession.Metadata) error {
		if req.Title != nil {
			metadata.Title = NormalizeTitle(*req.Title)
		}
		if req.Favorite != nil {
			metadata.Favorite = *req.Favorite
		}
		if req.CurrentAgent != nil {
			metadata.CurrentAgent = strings.TrimSpace(*req.CurrentAgent)
		}
		metadata.UpdatedAt = s.now()
		return nil
	})
}

func (s *Service) Delete(ctx context.Context, sessionID string) error {
	repository, err := s.requireRepository()
	if err != nil {
		return err
	}
	return repository.DeleteSession(ctx, sessionID)
}

// NormalizeTitle 统一创建和更新会话时的标题规则。
func NormalizeTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = defaultTitle
	}
	const maxTitleRunes = 50
	runes := []rune(title)
	if len(runes) > maxTitleRunes {
		return string(runes[:maxTitleRunes]) + "..."
	}
	return title
}
