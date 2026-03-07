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

const extractPrompt = `你是一个个人助手的记忆提取专家。你的目标是尽可能了解用户，记录一切能改善用户体验、让助手越用越顺手的信息。

对话格式说明：
- [用户]：真实用户发送的消息，是提取用户信息的唯一来源
- [AI助手]：AI 助手的回复，仅作为上下文理解参考，不代表用户信息

重要原则：只从 [用户] 的消息中提取关于用户本人的信息。[AI助手] 消息中的"我"指的是 AI 自身，不是用户，请勿将 AI 的自我描述误记为用户信息。

请从以下对话中提取值得长期记忆的信息，分为以下五类：

1. preference（偏好习惯）：用户的喜好、厌恶、习惯、风格倾向，涵盖工作和生活各方面。
   示例："我喜欢简洁的回答"、"我喜欢吃橘子"、"代码风格用 4 空格缩进"、"不喜欢加班"。
2. fact（个人信息）：关于用户本人的客观事实，如身份、背景、关系、环境等。
   示例："我的生日是 5 月 1 号"、"我在杭州工作"、"我养了一只猫叫小白"、"我用的是 Mac"。
3. lesson（经验教训）：踩过的坑、排查过的问题、需要避免的做法。
   示例："忽略时区差异导致数据错乱"、"这个接口必须先鉴权"、"批量导入不能超过 5000 条"。
4. decision（决策结论）：经过讨论确定的方案、明确的结论、选定的方向。
   示例："最终决定用方案 B"、"周报改为每周五提交"、"命名规范用小驼峰"。
5. insight（认知洞察）：用户表达的观点、原则、价值判断、独到见解。
   示例："我认为代码可读性比性能更重要"、"做事要先有全局观再抠细节"。

不要提取以下内容：
- [AI助手] 消息中关于 AI 自身的角色描述、能力介绍（如"我是XXX助手"）
- 一次性操作指令（如"帮我翻译这段话"）
- 假设性讨论（如"如果换一种方案会怎样"）
- 已明确否定的方案

提取原则：宁可多记不要漏记。只要是能帮助助手更好地服务用户的信息，都应该提取。

输出格式为 JSON 数组，每个元素包含：
- type: "preference" | "fact" | "lesson" | "decision" | "insight"
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
		fmt.Printf("[memory] conversation too short (%d chars), skipping extraction\n", utf8.RuneCountInString(conversation))
		return nil, nil
	}

	fmt.Printf("[memory] formatted conversation: %d chars, calling LLM...\n", utf8.RuneCountInString(conversation))
	prompt := fmt.Sprintf(extractPrompt, conversation)
	start := time.Now()
	result, err := llmClient.Complete(ctx, prompt)
	if err != nil {
		fmt.Printf("[memory] LLM call failed after %s: %v\n", time.Since(start).Round(time.Millisecond), err)
		return nil, fmt.Errorf("llm complete failed: %w", err)
	}
	fmt.Printf("[memory] LLM call completed in %s\n", time.Since(start).Round(time.Millisecond))

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
