package session

import (
	"context"

	"github.com/google/uuid"
)

// ID 是稳定的会话标识。
type ID string

type contextKey struct{}

// NewID 生成新的会话 ID。
func NewID() string {
	return uuid.NewString()
}

// WithID 将会话 ID 注入 context。
func WithID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

// IDFromContext 从 context 中提取会话 ID。
func IDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(contextKey{}).(string)
	return id, ok
}
