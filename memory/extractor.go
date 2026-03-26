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
		var label string
		if msg.Role == "user" {
			label = "[用户]"
		} else {
			label = "[AI助手]"
			if utf8.RuneCountInString(content) > 500 {
				content = string([]rune(content)[:500])
			}
		}
		fmt.Fprintf(&sb, "%s: %s\n", label, content)
	}
	text := sb.String()
	if utf8.RuneCountInString(text) < 200 {
		return ""
	}
	return text
}

const extractPrompt = `你是个人助手的记忆提取专家。你的任务是从对话中识别两类值得长期记住的信息：
1. 关于「用户本人」的持久性信息
2. AI 在执行任务过程中积累的「操作经验」

## 对话格式
- [用户]：真实用户的消息
- [AI助手]：AI 的回复，包含执行过程和结果

## 核心原则

- 用户信息：必须由用户明确表达或可直接推断
- 操作经验：从 AI 的执行过程中提取遇到的问题和有效的解决方法，这些经验应具有通用参考价值
- **关注通用共性**：提取可复用的偏好、习惯、方法论，而非当前话题的具体细节
- 同类型且主题相关的多条信息合并为一条
- 经验类记忆应随对话迭代不断更新完善

## 六种记忆类型

| 类型 | 定义 |
|------|------|
| preference | 用户的主观偏好、习惯、风格倾向 |
| fact | 用户的客观背景信息 |
| lesson | 用户提到的踩坑经验、需要避免的做法 |
| decision | 经讨论确定的方案或结论 |
| insight | 用户的原则性观点或价值判断 |
| experience | AI 操作中遇到的问题及有效解决方法 |

## 不要提取

- 一次性任务指令
- 通用知识和常规技术细节
- 假设性讨论和已否定的方案
- 对话中的临时状态和当前话题的具体细节（除非能反映持久的偏好、习惯或可复用的经验）

## 输出格式

JSON 数组，每个元素：
- type: 上述六种之一
- summary: 一句话摘要（20字以内）
- detail: 补充说明（100字以内）
- tags: 3-5个关键词

没有值得提取的内容时返回空数组 []
只返回 JSON，不要任何解释或 markdown 代码块。

## 对话内容

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
		if !AllMemoryTypes[e.Type] {
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
