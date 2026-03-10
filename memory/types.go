package memory

import "time"

// MemoryType 记忆类型
type MemoryType string

const (
	Preference MemoryType = "preference" // 用户偏好、习惯、喜好厌恶
	Fact       MemoryType = "fact"       // 个人信息、身份背景、客观事实
	Lesson     MemoryType = "lesson"     // 错误教训、避坑记录
	Decision   MemoryType = "decision"   // 重要决策、确定方案
	Insight    MemoryType = "insight"    // 观点看法、认知原则
)

// AllMemoryTypes 所有合法记忆类型
var AllMemoryTypes = map[MemoryType]bool{
	Preference: true,
	Fact:       true,
	Lesson:     true,
	Decision:   true,
	Insight:    true,
}

// MemoryEntry 单条记忆
type MemoryEntry struct {
	ID        string     `json:"id"`
	Type      MemoryType `json:"type"`
	Summary   string     `json:"summary"` // 一句话摘要，20字以内，BM25 检索核心字段
	Detail    string     `json:"detail"`  // 详细内容，100字以内
	Tags      []string   `json:"tags"`    // LLM 生成的精炼关键词，3-5个
	SessionID string     `json:"session_id"`
	CreatedAt time.Time  `json:"created_at"`
	HitCount  int        `json:"hit_count"`
	LastHitAt *time.Time `json:"last_hit_at,omitempty"`
}
