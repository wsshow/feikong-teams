package storage

import (
	"context"

	domainhistory "fkteams/internal/domain/history"
	"fkteams/internal/domain/message"
)

// SessionMessageReader 读取指定会话的结构化历史消息。
type SessionMessageReader interface {
	LoadSessionMessages(ctx context.Context, sessionID string) ([]domainhistory.AgentMessage, error)
}

// QueueStore 保存待执行输入；Save 返回错误时必须保留上次已提交快照。
// 同一会话的读写由队列用例串行调用。
type QueueStore interface {
	Load(sessionID string) ([]message.QueuedMessage, error)
	Save(sessionID string, items []message.QueuedMessage) error
}
