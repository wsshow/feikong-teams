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

const extractPrompt = `你是个人助手的记忆提取专家。你的任务是识别值得长期记住的「关于用户本人」的信息。

## 对话格式
- [用户]：真实用户的消息，是提取信息的唯一来源
- [AI助手]：AI 的回复，仅提供上下文，不代表用户信息。绝不能将 AI 的自我描述记为用户信息

## 提取标准

只提取满足以下条件的信息：
1. 是关于「用户本人」的持久性信息，在未来对话中仍然有用
2. 由用户明确表达或可直接推断，不是猜测
3. 属于以下五类之一

### 五种记忆类型

| 类型 | 定义 | 典型信号词 |
|------|------|-----------|
| preference | 主观偏好、习惯、风格倾向 | "我喜欢/讨厌/习惯/偏好/不想要..." |
| fact | 客观的个人背景信息 | "我是/我在/我有/我的..." |
| lesson | 踩坑经验、需要避免的做法 | "上次...导致了问题"、"千万不要..." |
| decision | 经讨论确定的方案或结论 | "就用这个方案"、"决定了..." |
| insight | 用户的原则性观点或价值判断 | "我认为/我觉得...比...更重要" |

### 不要提取

- 一次性任务指令："帮我写个函数"、"翻译这段话"
- 技术讨论中的通用知识：某个 API 的用法、某段代码的实现细节
- 假设性讨论："如果用方案B会怎样"
- 已否定的方案
- AI 助手关于自身的描述
- 对话中的临时状态："我正在调试这个bug"（除非揭示了持久的工作内容/职责）

## 合并规则

同类型且主题相关的多条信息必须合并为一条：
- "喜欢Go" + "喜欢Python" → 一条 preference："用户喜欢Go和Python编程语言"
- "在杭州工作" + "住西湖区" → 一条 fact："用户在杭州西湖区工作和居住"

## 输出格式

JSON 数组，每个元素：
- type: "preference" | "fact" | "lesson" | "decision" | "insight"
- summary: 一句话摘要（20字以内，以「用户」为主语）
- detail: 补充说明（100字以内，包含具体细节）
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
