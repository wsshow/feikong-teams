package session

import (
	"context"
	"crypto/rand"
	"fmt"
)

// ID 是稳定的会话标识。
type ID string

type contextKey struct{}

// NewID 生成新的会话 ID。
func NewID() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		panic(fmt.Errorf("generate session id: %w", err))
	}
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", id[0:4], id[4:6], id[6:8], id[8:10], id[10:16])
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
