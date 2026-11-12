package memory

import "time"

// MemoryType 记忆类型。
type MemoryType string

const (
	Preference MemoryType = "preference"
	Fact       MemoryType = "fact"
	Feedback   MemoryType = "feedback"
	Lesson     MemoryType = "lesson"
	Decision   MemoryType = "decision"
	Insight    MemoryType = "insight"
	Experience MemoryType = "experience"
)

// AllMemoryTypes 所有合法记忆类型。
var AllMemoryTypes = map[MemoryType]bool{
	Preference: true,
	Fact:       true,
	Feedback:   true,
	Lesson:     true,
	Decision:   true,
	Insight:    true,
	Experience: true,
}

// TypeMeta 记忆类型元信息。
type TypeMeta struct {
	Type  MemoryType
	Title string
}

// TypeOrder 返回稳定的记忆类型展示顺序。
func TypeOrder() []TypeMeta {
	return []TypeMeta{
		{Preference, "用户偏好"},
		{Fact, "个人信息"},
		{Feedback, "行为反馈"},
		{Lesson, "避坑记录"},
		{Decision, "已确定方案"},
		{Insight, "认知洞察"},
		{Experience, "操作经验"},
	}
}

// MemoryEntry 单条长期记忆。
type MemoryEntry struct {
	ID        string     `json:"id"`
	Scope     string     `json:"scope,omitempty"`
	Version   uint64     `json:"version"`
	UpdatedAt time.Time  `json:"updated_at"`
	Type      MemoryType `json:"type"`
	Summary   string     `json:"summary"`
	Detail    string     `json:"detail"`
	Tags      []string   `json:"tags"`
	SessionID string     `json:"session_id"`
	CreatedAt time.Time  `json:"created_at"`
	HitCount  int        `json:"hit_count"`
	LastHitAt *time.Time `json:"last_hit_at,omitempty"`
}
