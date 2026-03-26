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
	grouped := make(map[MemoryType][]MemoryEntry)
	for _, e := range entries {
		grouped[e.Type] = append(grouped[e.Type], e)
	}

	typeConfig := []struct {
		Type  MemoryType
		Title string
	}{
		{Preference, "用户偏好"},
		{Fact, "个人信息"},
		{Lesson, "避坑记录"},
		{Decision, "已确定方案"},
		{Insight, "认知洞察"},
		{Experience, "操作经验"},
	}

	var sb strings.Builder
	sb.WriteString("<!-- MEMORY_CONTEXT_START -->\n")
	sb.WriteString("## 长期记忆\n\n")
	sb.WriteString("以下是长期记忆信息，包含用户偏好和历史操作经验。请自然地融入回复中，据此调整回复风格和内容，参考操作经验避免重复踩坑。\n\n")

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
