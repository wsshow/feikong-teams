// Package autocontinue 提供模型输出截断自动续接中间件。
// 当模型因 max_tokens 限制截断纯文本输出时，注入假工具调用触发 react 循环继续，
// 使模型在下一轮迭代中自动续接被截断的内容。
package autocontinue

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"fkteams/log"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
)

const (
	toolName = "continue_output"
	toolDesc = "System-managed internal tool. DO NOT call this tool directly — it is automatically injected by the system when output is truncated."

	// maxContinues 单次模型调用最大自动续接次数，防止无限循环
	maxContinues           = 3
	continueTextPrompt     = "Your previous text output was truncated due to max_tokens limit. Continue your response from where it was cut off. Do not repeat content already output."
	continueToolCallPrompt = "Your previous tool call was truncated due to max_tokens limit and could not be executed. " +
		"The truncated tool calls were: %s. " +
		"Please retry the tool call. If the arguments are too long (e.g. large file content), " +
		"split into multiple smaller calls (e.g. write first part, then append remaining parts)."
	continueToolCallRepairedPrompt = "Your previous tool call for %s was truncated due to max_tokens limit. " +
		"The truncated arguments were auto-repaired and the tool has been executed with partial content. " +
		"Please continue from where the content was cut off. " +
		"Use append mode or appropriate method to add the remaining content."
)

// isTruncated 检查 FinishReason 是否表示 max_tokens 截断
// OpenAI/DeepSeek: "length", Claude: "max_tokens", Gemini: "MAX_TOKENS"
func isTruncated(reason string) bool {
	r := strings.ToLower(reason)
	return r == "length" || r == "max_tokens"
}

// continueToolInput 续接工具的输入参数
type continueToolInput struct {
	Prompt string `json:"prompt" jsonschema:"description=The continuation prompt to return"`
}

// continueTool 内部工具：返回续接提示
func continueTool(_ context.Context, input *continueToolInput) (string, error) {
	if input != nil && input.Prompt != "" {
		return input.Prompt, nil
	}
	return continueTextPrompt, nil
}

// ContinueTool 返回 continue_output 工具实例
func ContinueTool() (tool.BaseTool, error) {
	return utils.InferTool(toolName, toolDesc, continueTool)
}

// NewAgentMiddleware 创建自动续接中间件，包含工具注册和 AfterChatModel 钩子
func NewAgentMiddleware() (adk.AgentMiddleware, error) {
	t, err := ContinueTool()
	if err != nil {
		return adk.AgentMiddleware{}, err
	}
	return adk.AgentMiddleware{
		AdditionalTools: []tool.BaseTool{t},
		AfterChatModel:  afterChatModel,
	}, nil
}

// afterChatModel 检查模型输出是否被截断，注入续接工具调用
func afterChatModel(_ context.Context, state *adk.ChatModelAgentState) error {
	if len(state.Messages) == 0 {
		return nil
	}
	last := state.Messages[len(state.Messages)-1]
	if last == nil || last.Role != schema.Assistant {
		return nil
	}

	// 检查 FinishReason 是否为截断
	if last.ResponseMeta == nil || !isTruncated(last.ResponseMeta.FinishReason) {
		return nil
	}

	// 检查续接计数
	count := countRecentContinues(state.Messages)
	if count >= maxContinues {
		log.Warnf("autocontinue: reached max continues (%d), stopping", maxContinues)
		return nil
	}

	// 根据是否有工具调用选择不同的续接策略
	prompt := continueTextPrompt
	if len(last.ToolCalls) > 0 {
		// 工具调用被截断：尝试修复不完整的 JSON 参数
		var names []string
		repaired := false
		for i := range last.ToolCalls {
			tc := &last.ToolCalls[i]
			if tc.Function.Name != "" {
				names = append(names, tc.Function.Name)
			}
			if tc.Function.Arguments != "" && !json.Valid([]byte(tc.Function.Arguments)) {
				fixed := repairJSON(tc.Function.Arguments)
				if json.Valid([]byte(fixed)) {
					log.Infof("autocontinue: repaired truncated JSON for tool '%s'", tc.Function.Name)
					tc.Function.Arguments = fixed
					repaired = true
				}
			}
		}

		if repaired {
			// JSON 修复成功：保留原始工具调用让它们执行（写入部分内容），
			// 同时追加 continue_output 提示模型续写剩余部分
			prompt = fmt.Sprintf(continueToolCallRepairedPrompt, strings.Join(names, ", "))
		} else {
			// JSON 无法修复：清除工具调用，通过 continue_output 提示重试
			prompt = fmt.Sprintf(continueToolCallPrompt, strings.Join(names, ", "))
			last.ToolCalls = nil
		}
	}

	log.Infof("autocontinue: detected truncation (finish_reason=%s, continues=%d/%d), injecting continue tool call",
		last.ResponseMeta.FinishReason, count+1, maxContinues)

	// 注入 continue_output 工具调用
	args, _ := json.Marshal(continueToolInput{Prompt: prompt})
	last.ToolCalls = append(last.ToolCalls, schema.ToolCall{
		ID:   uuid.New().String(),
		Type: "function",
		Function: schema.FunctionCall{
			Name:      toolName,
			Arguments: string(args),
		},
	})
	last.ResponseMeta.FinishReason = "tool_calls"

	return nil
}

// countRecentContinues 从消息历史末尾向前统计连续的 continue_output 工具调用次数
func countRecentContinues(messages []*schema.Message) int {
	count := 0
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg == nil {
			continue
		}
		if msg.Role == schema.Tool && msg.ToolCallID != "" {
			// 检查这个 tool result 对应的 tool call 是否是 continue_output
			if isContinueToolResult(messages, msg.ToolCallID) {
				count++
				continue
			}
			break
		}
		if msg.Role == schema.Assistant && len(msg.ToolCalls) > 0 {
			hasContinue := false
			for _, tc := range msg.ToolCalls {
				if tc.Function.Name == toolName {
					hasContinue = true
					break
				}
			}
			if hasContinue {
				continue
			}
			break
		}
		// 遇到其他消息类型（user/system）则停止计数
		if msg.Role == schema.User || msg.Role == schema.System {
			break
		}
	}
	return count
}

// isContinueToolResult 检查给定 toolCallID 对应的工具调用是否为 continue_output
func isContinueToolResult(messages []*schema.Message, toolCallID string) bool {
	for _, msg := range messages {
		if msg == nil || msg.Role != schema.Assistant {
			continue
		}
		for _, tc := range msg.ToolCalls {
			if tc.ID == toolCallID && tc.Function.Name == toolName {
				return true
			}
		}
	}
	return false
}
