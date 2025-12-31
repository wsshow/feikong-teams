package common

import (
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"
)

func ConcatMessages(msgs []*schema.Message) (*schema.Message, error) {
	var (
		contents                      []string
		contentLen                    int
		reasoningContents             []string
		reasoningContentLen           int
		toolCalls                     []schema.ToolCall
		assistantGenMultiContentParts []schema.MessageOutputPart
		ret                           = schema.Message{}
		extraList                     = make([]map[string]any, 0, len(msgs))
	)

	for idx, msg := range msgs {
		if msg == nil {
			return nil, fmt.Errorf("unexpected nil chunk in message stream, index: %d", idx)
		}

		if msg.Role != "" {
			if ret.Role == "" {
				ret.Role = msg.Role
			} else if ret.Role != msg.Role {
				return nil, fmt.Errorf("cannot concat messages with "+
					"different roles: '%s' '%s'", ret.Role, msg.Role)
			}
		}

		if msg.Name != "" {
			if ret.Name == "" {
				ret.Name = msg.Name
			} else if ret.Name != msg.Name {
				return nil, fmt.Errorf("cannot concat messages with"+
					" different names: '%s' '%s'", ret.Name, msg.Name)
			}
		}

		if msg.ToolCallID != "" {
			if ret.ToolCallID == "" {
				ret.ToolCallID = msg.ToolCallID
			} else if ret.ToolCallID != msg.ToolCallID {
				return nil, fmt.Errorf("cannot concat messages with"+
					" different toolCallIDs: '%s' '%s'", ret.ToolCallID, msg.ToolCallID)
			}
		}
		if msg.ToolName != "" {
			if ret.ToolName == "" {
				ret.ToolName = msg.ToolName
			} else if ret.ToolName != msg.ToolName {
				return nil, fmt.Errorf("cannot concat messages with"+
					" different toolNames: '%s' '%s'", ret.ToolCallID, msg.ToolCallID)
			}
		}

		if msg.Content != "" {
			contents = append(contents, msg.Content)
			contentLen += len(msg.Content)
		}
		if msg.ReasoningContent != "" {
			reasoningContents = append(reasoningContents, msg.ReasoningContent)
			reasoningContentLen += len(msg.ReasoningContent)
		}

		if len(msg.ToolCalls) > 0 {
			toolCalls = append(toolCalls, msg.ToolCalls...)
		}

		if len(msg.Extra) > 0 {
			extraList = append(extraList, msg.Extra)
		}

		if len(msg.AssistantGenMultiContent) > 0 {
			assistantGenMultiContentParts = append(assistantGenMultiContentParts, msg.AssistantGenMultiContent...)
		}

		if msg.ResponseMeta != nil && ret.ResponseMeta == nil {
			ret.ResponseMeta = &schema.ResponseMeta{}
		}

		if msg.ResponseMeta != nil && ret.ResponseMeta != nil {
			// keep the last FinishReason with a valid value.
			if msg.ResponseMeta.FinishReason != "" {
				ret.ResponseMeta.FinishReason = msg.ResponseMeta.FinishReason
			}

			if msg.ResponseMeta.Usage != nil {
				if ret.ResponseMeta.Usage == nil {
					ret.ResponseMeta.Usage = &schema.TokenUsage{}
				}

				if msg.ResponseMeta.Usage.PromptTokens > ret.ResponseMeta.Usage.PromptTokens {
					ret.ResponseMeta.Usage.PromptTokens = msg.ResponseMeta.Usage.PromptTokens
				}
				if msg.ResponseMeta.Usage.CompletionTokens > ret.ResponseMeta.Usage.CompletionTokens {
					ret.ResponseMeta.Usage.CompletionTokens = msg.ResponseMeta.Usage.CompletionTokens
				}

				if msg.ResponseMeta.Usage.TotalTokens > ret.ResponseMeta.Usage.TotalTokens {
					ret.ResponseMeta.Usage.TotalTokens = msg.ResponseMeta.Usage.TotalTokens
				}

				if msg.ResponseMeta.Usage.PromptTokenDetails.CachedTokens > ret.ResponseMeta.Usage.PromptTokenDetails.CachedTokens {
					ret.ResponseMeta.Usage.PromptTokenDetails.CachedTokens = msg.ResponseMeta.Usage.PromptTokenDetails.CachedTokens
				}
			}

			if msg.ResponseMeta.LogProbs != nil {
				if ret.ResponseMeta.LogProbs == nil {
					ret.ResponseMeta.LogProbs = &schema.LogProbs{}
				}

				ret.ResponseMeta.LogProbs.Content = append(ret.ResponseMeta.LogProbs.Content, msg.ResponseMeta.LogProbs.Content...)
			}

		}
	}

	if len(contents) > 0 {
		var sb strings.Builder
		sb.Grow(contentLen)
		for _, content := range contents {
			_, err := sb.WriteString(content)
			if err != nil {
				return nil, err
			}
		}

		ret.Content = sb.String()
	}
	if len(reasoningContents) > 0 {
		var sb strings.Builder
		sb.Grow(reasoningContentLen)
		for _, rc := range reasoningContents {
			_, err := sb.WriteString(rc)
			if err != nil {
				return nil, err
			}
		}

		ret.ReasoningContent = sb.String()
	}

	return &ret, nil
}
