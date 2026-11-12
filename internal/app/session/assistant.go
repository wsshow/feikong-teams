package session

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"fkteams/internal/domain/apperror"
	domainsession "fkteams/internal/domain/session"
)

// OpenAssistant 幂等创建常驻助手会话，保留已有名称和全部聊天记录。
func (s *Service) OpenAssistant(ctx context.Context) (domainsession.Metadata, error) {
	repository, err := s.requireRepository()
	if err != nil {
		return domainsession.Metadata{}, err
	}
	now := s.now()
	metadata, _, err := repository.CreateSession(ctx, domainsession.Metadata{
		ID: domainsession.AssistantID, Title: "助手", CurrentAgent: domainsession.AssistantAgent,
		Mode: "team", Status: domainsession.StatusIdle, CreatedAt: now, UpdatedAt: now,
	})
	return metadata, err
}

// RenameAssistant 保存用户指定的助手名称，仅影响常驻助手会话。
func (s *Service) RenameAssistant(ctx context.Context, name string) (domainsession.Metadata, error) {
	return s.Update(ctx, UpdateRequest{SessionID: domainsession.AssistantID, Title: &name})
}

// ClearAssistantContext 清空常驻助手历史及队列，保留名称和长期记忆。
func (s *Service) ClearAssistantContext(ctx context.Context) error {
	repository, err := s.requireRepository()
	if err != nil {
		return err
	}
	return repository.ClearSessionHistory(ctx, domainsession.AssistantID)
}

func validateAssistantName(name string) error {
	if strings.TrimSpace(name) == "" || utf8.RuneCountInString(strings.TrimSpace(name)) > 40 {
		return apperror.New(apperror.CodeInvalidArgument, "assistant name must contain 1 to 40 characters")
	}
	for _, char := range name {
		if unicode.IsControl(char) {
			return apperror.New(apperror.CodeInvalidArgument, "assistant name must not contain control characters")
		}
	}
	return nil
}
