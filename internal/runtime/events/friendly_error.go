package events

import "strings"

type FriendlyError struct {
	Code            string   `json:"code,omitempty"`
	Title           string   `json:"title,omitempty"`
	Message         string   `json:"message,omitempty"`
	Suggestions     []string `json:"suggestions,omitempty"`
	TechnicalDetail string   `json:"technical_detail,omitempty"`
}

func NormalizeFriendlyError(raw string) FriendlyError {
	raw = strings.TrimSpace(raw)
	lower := strings.ToLower(raw)
	friendly := FriendlyError{
		Code:            "unknown_error",
		Title:           "任务执行失败",
		Message:         "任务执行时遇到了问题。你可以查看技术详情，或稍后重试。",
		TechnicalDetail: raw,
	}
	if raw == "" {
		return friendly
	}

	switch {
	case strings.Contains(lower, "does not support image_url") ||
		strings.Contains(lower, "unsupported image_url") ||
		strings.Contains(lower, "image_url type"):
		provider := providerNameFromError(lower)
		friendly.Code = "model_unsupported_image_input"
		friendly.Title = "当前模型不支持图片输入"
		friendly.Message = "这次消息里包含图片，但当前模型" + providerSuffix(provider) + "不支持图片输入。图片和消息已经保存在会话历史中，后续不会自动再次发送给当前模型。"
		friendly.Suggestions = []string{
			"切换到支持视觉输入的模型后重试。",
			"继续用文字描述图片内容后再提问。",
			"需要查看历史图片时，可让模型读取会话附件信息。",
		}
	case strings.Contains(lower, "does not support audio") || strings.Contains(lower, "audio_url"):
		friendly.Code = "model_unsupported_audio_input"
		friendly.Title = "当前模型不支持音频输入"
		friendly.Message = "这次消息里包含音频，但当前模型不支持音频输入。音频和消息已经保存在会话历史中。"
		friendly.Suggestions = []string{"切换到支持音频输入的模型后重试。", "将音频内容转写成文字后再发送。"}
	case strings.Contains(lower, "does not support video") || strings.Contains(lower, "video_url"):
		friendly.Code = "model_unsupported_video_input"
		friendly.Title = "当前模型不支持视频输入"
		friendly.Message = "这次消息里包含视频，但当前模型不支持视频输入。视频和消息已经保存在会话历史中。"
		friendly.Suggestions = []string{"切换到支持视频输入的模型后重试。", "用文字描述视频内容或提供截图后再发送。"}
	case strings.Contains(lower, "does not support file") || strings.Contains(lower, "file_url"):
		friendly.Code = "model_unsupported_file_input"
		friendly.Title = "当前模型不支持文件输入"
		friendly.Message = "这次消息里包含文件，但当前模型不支持直接读取文件输入。文件信息已经保存在会话历史中。"
		friendly.Suggestions = []string{"使用文件或文档读取工具读取文件内容。", "复制关键文本后再提问。"}
	case strings.Contains(lower, "exceeds max iterations"):
		friendly.Code = "max_iterations_exceeded"
		friendly.Title = "执行步数已达上限"
		friendly.Message = "任务执行步数达到上限后自动停止。你可以点击继续，让模型从中断处接着处理。"
		friendly.Suggestions = []string{"点击继续按钮。", "也可以拆分任务，降低单次执行复杂度。"}
	case strings.Contains(lower, "context length") ||
		strings.Contains(lower, "maximum context") ||
		strings.Contains(lower, "token limit") ||
		strings.Contains(lower, "too many tokens"):
		friendly.Code = "context_length_exceeded"
		friendly.Title = "上下文长度超出模型限制"
		friendly.Message = "当前会话或输入内容太长，超过了模型一次请求能处理的范围。"
		friendly.Suggestions = []string{"压缩或删除不必要的上下文后重试。", "换用上下文窗口更大的模型。", "把任务拆成几轮处理。"}
	case strings.Contains(lower, "rate limit") || strings.Contains(lower, "status code: 429") || strings.Contains(lower, " 429"):
		friendly.Code = "rate_limited"
		friendly.Title = "请求过于频繁"
		friendly.Message = "模型服务返回了限流错误。"
		friendly.Suggestions = []string{"稍等一会儿再试。", "检查当前供应商账号的限流策略。"}
	case strings.Contains(lower, "quota") || strings.Contains(lower, "insufficient balance") || strings.Contains(lower, "billing"):
		friendly.Code = "quota_exceeded"
		friendly.Title = "模型服务额度不足"
		friendly.Message = "当前模型供应商返回了额度、余额或计费相关错误。"
		friendly.Suggestions = []string{"检查 API Key 对应账号的余额和额度。", "切换到可用的模型供应商。"}
	case strings.Contains(lower, "unauthorized") ||
		strings.Contains(lower, "invalid api key") ||
		strings.Contains(lower, "api key") ||
		strings.Contains(lower, "401") ||
		strings.Contains(lower, "登录已过期"):
		friendly.Code = "authentication_failed"
		friendly.Title = "认证失败"
		friendly.Message = "当前请求没有通过认证，可能是登录状态或 API Key 已失效。"
		friendly.Suggestions = []string{"重新登录或刷新认证状态。", "检查模型供应商 API Key 是否正确。"}
	case strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "connection reset") ||
		strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "eof") ||
		strings.Contains(lower, "network"):
		friendly.Code = "provider_connection_failed"
		friendly.Title = "模型服务连接失败"
		friendly.Message = "连接模型服务时遇到网络或供应商响应问题。"
		friendly.Suggestions = []string{"稍后重试。", "检查网络、代理或模型供应商状态。"}
	}
	return friendly
}

func providerNameFromError(lower string) string {
	for _, name := range []string{"deepseek", "openai", "claude", "gemini", "qwen", "ollama", "openrouter", "ark"} {
		if strings.Contains(lower, name) {
			return name
		}
	}
	return ""
}

func providerSuffix(provider string) string {
	if provider == "" {
		return ""
	}
	return "（" + provider + "）"
}
