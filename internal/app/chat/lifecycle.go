package chat

import (
	"context"
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

// FinishRequest 描述历史提交后的会话状态与记忆更新。
type FinishRequest struct {
	SessionID      string
	TitleSource    string
	SuccessStatus  session.Status
	DefaultTitle   string
	Error          error
	Memory         MemoryExtractor
	MemoryMessages []domainmemory.Message
}

// SessionLifecycle 在回合历史提交后更新会话 metadata 并触发记忆提取。
type SessionLifecycle struct {
	metadata MetadataStore
}

// CompletionStatus 统一执行结果到会话状态的映射，保证传输状态和持久化状态一致。
func CompletionStatus(err error) session.Status {
	if err == nil {
		return session.StatusCompleted
	}
	if cancellationOnly(err) {
		return session.StatusCancelled
	}
	return session.StatusError
}

// finalizationError 将收尾故障与执行取消区分，保留 errors.Is 的原始错误链。
type finalizationError struct{ cause error }

func (e *finalizationError) Error() string { return "finalize chat: " + e.cause.Error() }
func (e *finalizationError) Unwrap() error { return e.cause }
func finalizationFailure(err error) error {
	if err == nil {
		return nil
	}
	return &finalizationError{cause: err}
}

func cancellationOnly(err error) bool {
	if err == context.Canceled {
		return true
	}
	switch e := err.(type) {
	case *finalizationError:
		return false
	case interface{ Unwrap() []error }:
		children := e.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !cancellationOnly(child) {
				return false
			}
		}
		return true
	case interface{ Unwrap() error }:
		return cancellationOnly(e.Unwrap())
	default:
		return false
	}
}

func finishHistory(history SessionHistory, runErr error) error {
	if history == nil {
		return nil
	}
	switch CompletionStatus(runErr) {
	case session.StatusCancelled:
		history.RecordCancelled("任务已取消")
	case session.StatusError:
		history.RecordEvent(event.Event{Type: event.TypeError, Content: runErr.Error(), Error: runErr.Error()})
	}
	history.FinalizeCurrent()
	return history.Flush()
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
	status := CompletionStatus(req.Error)
	if req.Error == nil && req.SuccessStatus != "" {
		status = req.SuccessStatus
	}

	if err := l.updateMetadata(ctx, MetadataUpdate{
		SessionID:          req.SessionID,
		TitleSource:        req.TitleSource,
		Status:             status,
		DefaultTitle:       req.DefaultTitle,
		CreateIfMissing:    true,
		UpdateDefaultTitle: true,
	}); err != nil {
		return finalizationFailure(err)
	}
	if status == session.StatusCompleted {
		ExtractMemoryAsync(req.Memory, req.MemoryMessages, req.SessionID)
	}
	return nil
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
