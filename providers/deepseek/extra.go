package deepseek

import (
	"github.com/cloudwego/eino/schema"
)

const (
	extraKeyReasoningContent = "_fkteams_deepseek_reasoning_content"
	extraKeyPrefix           = "_fkteams_deepseek_prefix"
)

func SetReasoningContent(message *schema.Message, content string) {
	if message == nil {
		return
	}
	if message.Extra == nil {
		message.Extra = make(map[string]any)
	}
	message.Extra[extraKeyReasoningContent] = content
}

func GetReasoningContent(message *schema.Message) (string, bool) {
	if message == nil || message.Extra == nil {
		return "", false
	}
	result, ok := message.Extra[extraKeyReasoningContent].(string)
	return result, ok
}

func SetPrefix(message *schema.Message) {
	if message == nil {
		return
	}
	if message.Extra == nil {
		message.Extra = make(map[string]any)
	}
	message.Extra[extraKeyPrefix] = true
}

func HasPrefix(message *schema.Message) bool {
	if message == nil || message.Extra == nil {
		return false
	}
	_, ok := message.Extra[extraKeyPrefix].(bool)
	return ok
}
