package memory

import "time"

// MemoryType 记忆类型
type MemoryType string

const (
	Preference MemoryType = "preference" // 用户偏好、习惯
	Fact       MemoryType = "fact"       // 用户背景、身份信息
	Feedback   MemoryType = "feedback"   // 用户对工作方式的纠正和确认
	Lesson     MemoryType = "lesson"     // 踩坑经验、避坑记录
	Decision   MemoryType = "decision"   // 已确定的方案或结论
	Insight    MemoryType = "insight"    // 原则性观点或价值判断
	Experience MemoryType = "experience" // AI 操作经验：遇到的问题及解决方法
)

// AllMemoryTypes 所有合法记忆类型
var AllMemoryTypes = map[MemoryType]bool{
	Preference:  true,
	Fact:        true,
	Feedback:    true,
	Lesson:      true,
	Decision:    true,
	Insight:     true,
	Experience:  true,
}

// TypeMeta 记忆类型元信息
type TypeMeta struct {
	Type  MemoryType
	Title string
}

// typeOrder 类型展示顺序，injector 和 markdown 共用
var typeOrder = []TypeMeta{
	{Preference, "用户偏好"},
	{Fact, "个人信息"},
	{Feedback, "行为反馈"},
	{Lesson, "避坑记录"},
	{Decision, "已确定方案"},
	{Insight, "认知洞察"},
	{Experience, "操作经验"},
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
