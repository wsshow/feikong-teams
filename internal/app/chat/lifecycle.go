package chat

import (
	"context"
	"errors"
	"fkteams/internal/app/appstate"
	"fkteams/internal/domain/session"

	"fkteams/internal/domain/event"
	domainmemory "fkteams/internal/domain/memory"
)

// SessionHistory 描述聊天用例需要编排的历史记录能力。
type SessionHistory interface {
	FinalizeCurrent()
	RecordEvent(event.Event)
	RecordCancelled(message string)
	Flush() error
}

// MetadataUpdate 描述一次会话 metadata 状态变更。
type MetadataUpdate struct {
	SessionID          string
	TitleSource        string
	Status             session.Status
	DefaultTitle       string
	CreateIfMissing    bool
	UpdateDefaultTitle bool
	Mode               *string
	CurrentAgent       *string
}

// ExecutionTarget 描述当前会话实际使用的运行模式和目标智能体。
type ExecutionTarget struct {
	Mode         string
	CurrentAgent string
}

// MetadataStore 保存会话 metadata。
type MetadataStore interface {
	UpdateMetadata(ctx context.Context, update MetadataUpdate) error
}

// MemoryExtractor 是对话结束后长期记忆提取需要的最小能力。
type MemoryExtractor = appstate.MemoryExtractor

type FinishRequest struct {
	SessionID       string
	TitleSource     string
	Status          session.Status
	DefaultTitle    string
	History         SessionHistory
	FinalizeHistory bool
	Error           error
	Memory          MemoryExtractor
	MemoryMessages  []domainmemory.Message
}

// SessionLifecycle 统一编排一次会话运行后的历史、metadata 和记忆收尾。
type SessionLifecycle struct {
	metadata MetadataStore
}

// CompletionStatus 统一执行结果到会话状态的映射，保证传输状态和持久化状态一致。
func CompletionStatus(err error) session.Status {
	if errors.Is(err, context.Canceled) {
		return session.StatusCancelled
	}
	if err != nil {
		return session.StatusError
	}
	return session.StatusCompleted
}

// NewSessionLifecycle 使用显式元数据存储创建会话收尾用例。
func NewSessionLifecycle(metadata MetadataStore) *SessionLifecycle {
	return &SessionLifecycle{metadata: metadata}
}

func (l *SessionLifecycle) MarkProcessing(ctx context.Context, sessionID, titleSource string) error {
	return l.updateMetadata(ctx, MetadataUpdate{
		SessionID:          sessionID,
		TitleSource:        titleSource,
		Status:             session.StatusProcessing,
		DefaultTitle:       "未命名会话",
		CreateIfMissing:    true,
		UpdateDefaultTitle: true,
	})
}

// MarkProcessingWithTarget 在进入处理态时同步本次运行目标。
func (l *SessionLifecycle) MarkProcessingWithTarget(ctx context.Context, sessionID, titleSource string, target ExecutionTarget) error {
	return l.updateMetadata(ctx, MetadataUpdate{
		SessionID:          sessionID,
		TitleSource:        titleSource,
		Status:             session.StatusProcessing,
		DefaultTitle:       "未命名会话",
		CreateIfMissing:    true,
		UpdateDefaultTitle: true,
		Mode:               &target.Mode,
		CurrentAgent:       &target.CurrentAgent,
	})
}

func (l *SessionLifecycle) Finish(ctx context.Context, req FinishRequest) error {
	var persistErr error
	if req.History != nil {
		switch req.Status {
		case session.StatusCancelled:
			req.History.RecordCancelled("任务已取消")
		case session.StatusError:
			if req.Error != nil {
				req.History.RecordEvent(event.Event{
					Type:    event.TypeError,
					Content: req.Error.Error(),
					Error:   req.Error.Error(),
				})
			}
			if req.FinalizeHistory {
				req.History.FinalizeCurrent()
			}
		default:
			if req.FinalizeHistory {
				req.History.FinalizeCurrent()
			}
		}
		persistErr = req.History.Flush()
		if persistErr != nil {
			req.Status = session.StatusError
		}
	}
	if err := l.updateMetadata(ctx, MetadataUpdate{
		SessionID:          req.SessionID,
		TitleSource:        req.TitleSource,
		Status:             req.Status,
		DefaultTitle:       req.DefaultTitle,
		CreateIfMissing:    true,
		UpdateDefaultTitle: true,
	}); err != nil {
		return errors.Join(persistErr, err)
	}
	if req.Status == session.StatusCompleted {
		ExtractMemoryAsync(req.Memory, req.MemoryMessages, req.SessionID)
	}
	return persistErr
}

func (l *SessionLifecycle) SaveActive(ctx context.Context, sessionID, titleSource string, history SessionHistory) error {
	if history != nil {
		if err := history.Flush(); err != nil {
			return err
		}
	}
	return l.updateMetadata(ctx, MetadataUpdate{
		SessionID:          sessionID,
		TitleSource:        titleSource,
		Status:             session.StatusActive,
		DefaultTitle:       "未命名会话",
		CreateIfMissing:    true,
		UpdateDefaultTitle: true,
	})
}

func (l *SessionLifecycle) updateMetadata(ctx context.Context, update MetadataUpdate) error {
	if l == nil || l.metadata == nil || update.SessionID == "" {
		return nil
	}
	if update.DefaultTitle == "" {
		update.DefaultTitle = "未命名会话"
	}
	return l.metadata.UpdateMetadata(ctx, update)
}

func ExtractMemoryAsync(manager MemoryExtractor, messages []domainmemory.Message, sessionID string) {
	if manager == nil || len(messages) == 0 {
		return
	}
	copied := append([]domainmemory.Message(nil), messages...)
	manager.ExtractAndStoreAsync(copied, sessionID)
}

func FlushMemory(ctx context.Context, manager MemoryExtractor, messages []domainmemory.Message, sessionID string) {
	if manager == nil || len(messages) == 0 {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	manager.FlushExtract(ctx, messages, sessionID)
}
