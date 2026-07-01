package chat

import (
	"context"
	"time"

	"fkteams/internal/domain/event"
	domainmemory "fkteams/internal/domain/memory"
	"fkteams/internal/runtime/log"
)

const (
	SessionStatusActive     = "active"
	SessionStatusProcessing = "processing"
	SessionStatusCompleted  = "completed"
	SessionStatusCancelled  = "cancelled"
	SessionStatusError      = "error"
)

// SessionHistory 描述聊天用例需要编排的历史记录能力。
type SessionHistory interface {
	FinalizeCurrent()
	RecordEvent(event.Event)
	RecordCancelled(message string)
}

// HistoryStore 保存指定会话的历史记录。
type HistoryStore interface {
	SaveHistory(ctx context.Context, sessionID string, history SessionHistory) error
}

// MetadataUpdate 描述一次会话 metadata 状态变更。
type MetadataUpdate struct {
	SessionID          string
	TitleSource        string
	Status             string
	DefaultTitle       string
	CreateIfMissing    bool
	UpdateDefaultTitle bool
}

// MetadataStore 保存会话 metadata。
type MetadataStore interface {
	UpdateMetadata(ctx context.Context, update MetadataUpdate) error
}

// MemoryExtractor 是对话结束后长期记忆提取需要的最小能力。
type MemoryExtractor interface {
	ExtractAndStore(ctx context.Context, messages []domainmemory.Message, sessionID string)
	FlushExtract(ctx context.Context, messages []domainmemory.Message, sessionID string)
}

type FinishRequest struct {
	SessionID       string
	TitleSource     string
	Status          string
	DefaultTitle    string
	History         SessionHistory
	FinalizeHistory bool
	Error           error
	Memory          MemoryExtractor
	MemoryMessages  []domainmemory.Message
}

// SessionLifecycle 统一编排一次会话运行后的历史、metadata 和记忆收尾。
type SessionLifecycle struct {
	history  HistoryStore
	metadata MetadataStore
}

func NewSessionLifecycle(history HistoryStore, metadata MetadataStore) *SessionLifecycle {
	return &SessionLifecycle{history: history, metadata: metadata}
}

func (l *SessionLifecycle) MarkProcessing(ctx context.Context, sessionID, titleSource string) error {
	return l.updateMetadata(ctx, MetadataUpdate{
		SessionID:          sessionID,
		TitleSource:        titleSource,
		Status:             SessionStatusProcessing,
		DefaultTitle:       "未命名会话",
		CreateIfMissing:    true,
		UpdateDefaultTitle: true,
	})
}

func (l *SessionLifecycle) Finish(ctx context.Context, req FinishRequest) error {
	if req.History != nil {
		switch req.Status {
		case SessionStatusCancelled:
			req.History.RecordCancelled("任务已取消")
		case SessionStatusError:
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
		if err := l.saveHistory(ctx, req.SessionID, req.History); err != nil {
			return err
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
		return err
	}
	if req.Status == SessionStatusCompleted {
		ExtractMemoryAsync(req.Memory, req.MemoryMessages, req.SessionID)
	}
	return nil
}

func (l *SessionLifecycle) SaveActive(ctx context.Context, sessionID, titleSource string, history SessionHistory) error {
	if err := l.saveHistory(ctx, sessionID, history); err != nil {
		return err
	}
	return l.updateMetadata(ctx, MetadataUpdate{
		SessionID:          sessionID,
		TitleSource:        titleSource,
		Status:             SessionStatusActive,
		DefaultTitle:       "未命名会话",
		CreateIfMissing:    true,
		UpdateDefaultTitle: true,
	})
}

func (l *SessionLifecycle) saveHistory(ctx context.Context, sessionID string, history SessionHistory) error {
	if l == nil || l.history == nil || history == nil {
		return nil
	}
	return l.history.SaveHistory(ctx, sessionID, history)
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
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		manager.ExtractAndStore(ctx, copied, sessionID)
	}()
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

func LogLifecycleError(scope, sessionID string, err error) {
	if err != nil {
		log.Printf("[%s] session lifecycle failed: session=%s, err=%v", scope, sessionID, err)
	}
}
