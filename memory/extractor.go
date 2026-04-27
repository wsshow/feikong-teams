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

const extractPrompt = `你是一个记忆提取专家。从对话中识别值得长期记住的信息，这些信息将在未来的对话中通过搜索自动注入。

## 对话格式
- [用户]：用户消息
- [AI助手]：AI 回复（可能截断）

## 提取原则

- 提取可复用的偏好、习惯、方法论，而非当前任务的一次性细节
- 同类型且主题相关的多条信息合并为一条
- 记忆是给未来的对话用的——问自己：下次对话时这条信息还有用吗？

## 七种记忆类型

<types>
<type>
<name>preference</name>
<desc>用户的主观偏好和习惯</desc>
<when_to_save>用户表达对某种风格、工具、方案的偏好时提取</when_to_save>
<how_to_use>据此调整回复风格和方案选择，使回复更贴合用户口味</how_to_use>
<examples>
- 用户说"我喜欢简洁的回答" → preference: 偏好简洁回答
- 用户说"用 React 方案，别用 Vue" → preference: 偏好 React 技术栈
</examples>
</type>

<type>
<name>fact</name>
<desc>用户的客观背景信息</desc>
<when_to_save>用户提及自己的技能、角色、项目、环境等背景时提取</when_to_save>
<how_to_use>据此调整技术深度和解释方式，用用户熟悉的领域知识类比</how_to_use>
<examples>
- 用户说"我写了十年 Go" → fact: 资深 Go 工程师
- 用户说"我在负责后端 API 重构" → fact: 当前负责后端 API 重构项目
</examples>
</type>

<type>
<name>feedback</name>
<desc>用户对工作方式的纠正或确认。这是最重要的记忆类型——记录用户希望你如何做事</desc>
<when_to_save>用户纠正你的做法（"别这样"、"不要 X"、"停"）或确认做法正确（"对"、"就按这个来"）时提取。纠正容易注意，确认容易被忽略——请主动留意。都要存，包括 WHY，以便判断边界情况</when_to_save>
<how_to_use>让这些反馈指导你的行为，避免重复犯错。记录原因才能在未来变通</how_to_use>
<structure>规则本身 + Why:（原因）+ How to apply:（何时适用）</structure>
<examples>
- 用户说"测试别 mock 数据库，上次线上事故就是因为 mock 没发现" → feedback: 集成测试必须连真实数据库。Why: 上次 mock 和生产环境不一致导致事故。How to apply: 涉及数据库的测试不要 mock
- 用户说"不用总结改动，我能看 diff" → feedback: 回复要简洁，不要结尾总结
</examples>
</type>

<type>
<name>lesson</name>
<desc>用户分享的踩坑经验、需要避免的做法</desc>
<when_to_save>用户描述过去遇到的问题或给出"小心 X"类警告时提取</when_to_save>
<how_to_use>遇到类似场景时主动提醒，避免重蹈覆辙</how_to_use>
<examples>
- 用户说"上次升级依赖版本导致兼容性问题" → lesson: 升级依赖需先检查 breaking changes
</examples>
</type>

<type>
<name>decision</name>
<desc>讨论后确定的方案或结论</desc>
<when_to_save>用户明确拍板或达成共识时提取。记录决策的 WHY，帮助未来判断是否仍适用</when_to_save>
<how_to_use>后续涉及该方向时以此为出发点，但如果状态已变化需重新评估</how_to_use>
<structure>方案/结论 + Why:（决策原因）+ How to apply:（适用场景）</structure>
<examples>
- 用户说"我们就用 Eino 框架，不用 LangChain 了" → decision: 确定使用 Eino 框架。Why: 后续功能均基于 Eino 构建
</examples>
</type>

<type>
<name>insight</name>
<desc>用户的原则性观点或价值判断</desc>
<when_to_save>用户表达深层信念、方法论、价值观时提取</when_to_save>
<how_to_use>作为决策的顶层原则，在面临 trade-off 时以此为参考</how_to_use>
<examples>
- 用户说"测试覆盖率比开发速度更重要" → insight: 测试优先于速度
</examples>
</type>

<type>
<name>experience</name>
<desc>AI 执行过程中发现的问题及有效解决方法，具有可复用性</desc>
<when_to_save>遇到错误、发现更优方案、学到新工具用法时提取。记录失败 + 成功</when_to_save>
<how_to_use>后续类似任务直接参考，避免重复摸索</how_to_use>
<examples>
- AI 发现 grep 对中文搜索效果差，改用 bigram 分词 → experience: 中文搜索需用 bigram 分词提升匹配效果
</examples>
</type>
</types>

## 不要提取

- 一次性任务指令（"帮我写脚本"、"把 X 改成 Y"）
- 通用技术知识（语言的语法、框架基础概念）
- 假设性讨论和已否定的方案
- 当前话题的一次性细节
- 代码模式、文件路径、git 历史——这些可以从项目代码中直接读取
- 用户在明确要求你保存之前，不要保存 PR 列表、活动摘要等临时信息

## 输出格式

JSON 数组（无 markdown 包裹，无多余文字）：
[{"type":"preference","summary":"≤20字摘要","detail":"≤100字补充","tags":["关键词1","关键词2","关键词3"]}]

无值得提取内容时返回 []

## 对话

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
	// 清理可能的 markdown 代码块包裹（含换行符的情况）
	result = strings.TrimPrefix(result, "```json\n")
	result = strings.TrimPrefix(result, "```\n")
	result = strings.TrimPrefix(result, "```json")
	result = strings.TrimPrefix(result, "```")
	result = strings.TrimSuffix(result, "\n```")
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
