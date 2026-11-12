package session

import (
	"context"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

// ID 是稳定的会话标识。
type ID string

// AssistantID 是当前应用常驻助手的固定会话标识。
const AssistantID = "workspace-assistant"

// AssistantAgent 是常驻助手使用的内置智能体标识。
const AssistantAgent = "workspace_assistant"

// Status 是会话对外可见的生命周期状态。
type Status string

const (
	StatusIdle       Status = "idle"
	StatusActive     Status = "active"
	StatusProcessing Status = "processing"
	StatusCompleted  Status = "completed"
	StatusCancelled  Status = "cancelled"
	StatusError      Status = "error"
)

// Metadata 是与具体存储实现无关的会话元数据。
type Metadata struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Status       Status    `json:"status"`
	Mode         string    `json:"mode,omitempty"`
	CurrentAgent string    `json:"current_agent,omitempty"`
	Favorite     bool      `json:"favorite,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Record 是会话列表所需的存储无关快照。
type Record struct {
	Metadata Metadata
	Size     int64
	ModTime  time.Time
}

type contextKey struct{}

// NewID 生成新的会话 ID。
func NewID() string {
	return uuid.NewString()
}

// ValidID 校验会话 ID 是否可以安全、稳定地作为资源标识。
func ValidID(id string) bool {
	if id == "" || id == "." || id == ".." || len(id) > 200 || strings.TrimSpace(id) != id {
		return false
	}
	for _, r := range id {
		if r == '/' || r == '\\' || unicode.IsControl(r) {
			return false
		}
	}
	return true
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
