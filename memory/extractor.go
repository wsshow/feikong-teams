package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// LLMClient LLM 调用接口
type LLMClient interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// Message 对话消息
type Message struct {
	Role    string
	Content string
}

// formatConversation 将消息列表转为纯文本，只保留 user 和 assistant 消息，
// assistant 消息截断到 500 字，对话总长度不足 200 字时返回空字符串
func formatConversation(messages []Message) string {
	var sb strings.Builder
	for _, msg := range messages {
		if msg.Role != "user" && msg.Role != "assistant" {
			continue
		}
		content := msg.Content
		if msg.Role == "assistant" && utf8.RuneCountInString(content) > 500 {
			content = string([]rune(content)[:500])
		}
		fmt.Fprintf(&sb, "%s: %s\n", msg.Role, content)
	}
	text := sb.String()
	if utf8.RuneCountInString(text) < 200 {
		return ""
	}
	return text
}

const extractPrompt = `你是一个记忆提取专家。请从以下对话中提取值得长期记忆的信息。

只提取以下三类：
1. preference（用户偏好和习惯）：用户明确表达的个人偏好、使用习惯、风格倾向、喜好厌恶等。
   示例："我喜欢简洁的回答风格"、"报告用中文撰写"、"我习惯用 Markdown 做笔记"、"每次汇总要包含数据来源"。
2. lesson（错误教训和避坑记录）：踩过的坑、排查后发现的根因、需要避免的做法、失败经验。
   示例："上次那个方案因为忽略了时区差异导致数据错乱"、"这个接口必须先鉴权才能调用"、"批量导入时不能超过 5000 条"。
3. decision（重要决策和结论）：经过讨论确定的方案、明确的结论、最终选定的方向。
   示例："最终决定用方案 B"、"周报改为每周五提交"、"项目命名规范确定为小驼峰"。

不要提取以下内容：
- 普通问答（如"帮我解释一下这个概念"）
- 已否定的方案（讨论后被否决的选择）
- 一次性操作细节（如"帮我改一下这个名字"、"翻译这段话"）
- 假设性讨论（如"如果换一种方案会怎样"）

输出格式为 JSON 数组，每个元素包含：
- type: "preference" | "lesson" | "decision"
- summary: 一句话摘要，20字以内
- detail: 详细内容，100字以内
- tags: 3-5个精炼关键词

没有值得提取的内容时返回空数组 []
只返回 JSON，不要任何解释或 markdown 代码块。

对话内容：
%s`

// extractedEntry LLM 返回的提取结果
type extractedEntry struct {
	Type    MemoryType `json:"type"`
	Summary string     `json:"summary"`
	Detail  string     `json:"detail"`
	Tags    []string   `json:"tags"`
}

// Extract 从对话历史中提取记忆条目
func Extract(ctx context.Context, messages []Message, sessionID string, llmClient LLMClient) ([]MemoryEntry, error) {
	conversation := formatConversation(messages)
	if conversation == "" {
		return nil, nil
	}

	prompt := fmt.Sprintf(extractPrompt, conversation)
	result, err := llmClient.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("llm complete failed: %w", err)
	}

	result = strings.TrimSpace(result)
	// 清理可能的 markdown 代码块包裹
	result = strings.TrimPrefix(result, "```json")
	result = strings.TrimPrefix(result, "```")
	result = strings.TrimSuffix(result, "```")
	result = strings.TrimSpace(result)

	var extracted []extractedEntry
	if err := json.Unmarshal([]byte(result), &extracted); err != nil {
		return nil, fmt.Errorf("failed to parse llm response: %w", err)
	}

	now := time.Now()
	entries := make([]MemoryEntry, 0, len(extracted))
	for _, e := range extracted {
		if e.Type != Preference && e.Type != Lesson && e.Type != Decision {
			continue
		}
		entries = append(entries, MemoryEntry{
			ID:        fmt.Sprintf("%s_%d", sessionID, now.UnixNano()),
			Type:      e.Type,
			Summary:   e.Summary,
			Detail:    e.Detail,
			Tags:      e.Tags,
			SessionID: sessionID,
			CreatedAt: now,
		})
		// 确保同一批次的 ID 唯一
		now = now.Add(time.Nanosecond)
	}
	return entries, nil
}
