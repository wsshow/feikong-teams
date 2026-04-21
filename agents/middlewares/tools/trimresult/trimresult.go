// Package trimresult 提供一个 BeforeModel 中间件，用于修剪噪声工具的已处理历史结果。
//
// 对于 fetch、doc 等会产生大量输出的工具，
// 只修剪"已消化"的结果（即 LLM 已将其处理为文字响应之前的结果），
// 仍在活跃工具调用链中的结果（尚无文字响应跟随）始终保留，
// 从而减少 LLM 上下文中的冗余噪声，同时不影响当前任务中工具结果的可见性。
package trimresult

import (
	"context"
	"fkteams/fkevent"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

const defaultPlaceholder = "[result omitted]"

// omittedMsg 生成动态占位符，包含被省略的字符数。
func omittedMsg(placeholder, content string) string {
	n := len([]rune(content))
	return fmt.Sprintf("%s (~%d chars)", placeholder, n)
}

// Config 配置噪声工具结果修剪中间件。
type Config struct {
	// NoisyToolPrefixes 指定噪声工具名称前缀列表，结果将从已处理的历史中移除。
	// 默认值来自 fkevent.NoisyToolPrefixes。
	NoisyToolPrefixes []string

	// Placeholder 是替换被修剪内容的占位符文本。
	// 默认值："[result omitted]"
	Placeholder string
}

// New 创建一个 AgentMiddleware，在每次 LLM 调用前修剪已消化的噪声工具结果。
//
// 判断标准：某个工具结果之后存在 Assistant 文字响应（Content != ""），
// 说明 LLM 已将该结果处理到其文字输出中，不再需要在上下文中保留完整内容。
// 活跃工具调用链（尚无文字响应跟随）的结果始终保留。
func New(cfg *Config) adk.AgentMiddleware {
	prefixes := fkevent.NoisyToolPrefixes
	placeholder := defaultPlaceholder

	if cfg != nil {
		if len(cfg.NoisyToolPrefixes) > 0 {
			prefixes = cfg.NoisyToolPrefixes
		}
		if cfg.Placeholder != "" {
			placeholder = cfg.Placeholder
		}
	}

	return adk.AgentMiddleware{
		BeforeChatModel: func(ctx context.Context, state *adk.ChatModelAgentState) error {
			if state == nil || len(state.Messages) < 2 {
				return nil
			}
			state.Messages = trimNoisyResults(state.Messages, prefixes, placeholder)
			return nil
		},
	}
}

// isNoisy 判断工具名是否匹配任意噪声前缀。
func isNoisy(name string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// trimNoisyResults 找出"最后一条 Assistant 文字响应"的位置，
// 将其之前的所有匹配噪声前缀的 tool 消息内容替换为占位符。
//
// 语义：最后一条文字响应之前的工具结果，LLM 已将其消化并写入了自己的输出中，
// 无需再在上下文中保留原始内容。当前仍在处理中的工具调用链（无文字响应跟随）不受影响。
func trimNoisyResults(messages []*schema.Message, prefixes []string, placeholder string) []*schema.Message {
	// 找出最后一条 Assistant 文字响应的索引（Content 非空即视为文字响应）
	lastTextIdx := -1
	for i, m := range messages {
		if m != nil && m.Role == schema.Assistant && m.Content != "" {
			lastTextIdx = i
		}
	}

	if lastTextIdx < 0 {
		// 尚无文字响应 → 仍在活跃工具调用链中 → 不修剪
		return messages
	}

	// 收集需要修剪的 tool 消息索引（仅限 lastTextIdx 之前）
	trimIndices := make(map[int]struct{})
	for i := 0; i < lastTextIdx; i++ {
		m := messages[i]
		if m != nil && m.Role == schema.Tool && isNoisy(m.ToolName, prefixes) {
			trimIndices[i] = struct{}{}
		}
	}

	if len(trimIndices) == 0 {
		return messages
	}

	// 构造新消息切片，对需要修剪的消息进行浅拷贝并替换 Content
	result := make([]*schema.Message, len(messages))
	for i, m := range messages {
		if _, ok := trimIndices[i]; ok {
			cp := *m
			cp.Content = omittedMsg(placeholder, m.Content)
			result[i] = &cp
		} else {
			result[i] = m
		}
	}
	return result
}
