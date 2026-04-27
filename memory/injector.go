package memory

import (
	"fmt"
	"strings"
)

const memoryUsageGuide = `## 如何使用记忆

- 这些记忆是从长期记忆中搜索得到的，已按相关性排序。自然地在回复中融入，作为背景而非答案
- **先验证再推荐**：记忆声称某文件、函数或参数存在，这只是"当时"的信息。文件可能被改名、删除或从未合并。引用前：记忆提到文件路径 → 检查文件是否存在；提到函数名 → grep 搜索确认；用户即将据此操作 → 务必验证
- 记忆可能过时。优先信任你当前观察到的代码和文件状态。如果记忆与现状冲突，以现状为准
- feedback 类型的记忆是用户对你行为的纠正——严格遵循，除非用户明确说不再适用

## 何时访问记忆

- 用户提到之前聊过的话题，或说"上次那个方案"、"之前你说的"
- 用户明确要求查看记忆（"我有什么记忆"、"记得什么"）
- 用户说需要忽略记忆时，把当前记忆视为不存在，不要引用或对比

## 什么不该存为记忆

- 代码模式、架构、文件路径、项目结构——这些可以直接从代码中读取
- git 历史、最近改动——git log 是权威来源
- 一次性任务细节和临时对话状态
- 已经在系统提示词或 AGENTS.md 中的信息
`

// BuildMemoryContext 构建记忆上下文
func BuildMemoryContext(entries []MemoryEntry) string {
	if len(entries) == 0 {
		return ""
	}

	grouped := make(map[MemoryType][]MemoryEntry)
	for _, e := range entries {
		grouped[e.Type] = append(grouped[e.Type], e)
	}

	var sb strings.Builder
	sb.WriteString("<!-- MEMORY_CONTEXT_START -->\n")

	hasContent := false
	for _, tc := range typeOrder {
		items := grouped[tc.Type]
		if len(items) == 0 {
			continue
		}
		if !hasContent {
			sb.WriteString("## 长期记忆\n\n")
			hasContent = true
		}
		fmt.Fprintf(&sb, "### %s\n\n", tc.Title)
		for _, item := range items {
			fmt.Fprintf(&sb, "- **%s**：%s\n", item.Summary, item.Detail)
		}
		sb.WriteString("\n")
	}

	if !hasContent {
		return ""
	}

	sb.WriteString(memoryUsageGuide)
	sb.WriteString("<!-- MEMORY_CONTEXT_END -->\n")
	return sb.String()
}
