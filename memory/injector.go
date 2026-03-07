package memory

import (
	"fmt"
	"strings"
)

// BuildMemoryContext 构建记忆上下文的 System Prompt 片段
func BuildMemoryContext(entries []MemoryEntry) string {
	if len(entries) == 0 {
		return ""
	}

	// 按类型分组
	grouped := map[MemoryType][]MemoryEntry{
		Lesson:     {},
		Decision:   {},
		Preference: {},
	}
	for _, e := range entries {
		grouped[e.Type] = append(grouped[e.Type], e)
	}

	typeConfig := []struct {
		Type  MemoryType
		Title string
	}{
		{Lesson, "⚠️ 避坑记录"},
		{Decision, "✅ 已确定方案"},
		{Preference, "💡 用户偏好"},
	}

	var sb strings.Builder
	sb.WriteString("<!-- MEMORY_CONTEXT_START -->\n")
	sb.WriteString("## 长期记忆\n\n")

	hasContent := false
	for _, tc := range typeConfig {
		items := grouped[tc.Type]
		if len(items) == 0 {
			continue
		}
		hasContent = true
		fmt.Fprintf(&sb, "### %s\n\n", tc.Title)
		for _, item := range items {
			fmt.Fprintf(&sb, "- **%s**：%s\n", item.Summary, item.Detail)
		}
		sb.WriteString("\n")
	}

	if !hasContent {
		return ""
	}

	sb.WriteString("<!-- MEMORY_CONTEXT_END -->\n")
	return sb.String()
}
