package handler

import (
	"context"
	"fkteams/agents"
	"fkteams/chatutil"
	"fkteams/fkevent"
	"fkteams/g"
	"fkteams/runner"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// RunnerCache 基于双重检查锁的 Runner 缓存
type RunnerCache struct {
	mu    sync.RWMutex
	cache map[string]*adk.Runner
}

// NewRunnerCache 创建 Runner 缓存
func NewRunnerCache() *RunnerCache {
	return &RunnerCache{cache: make(map[string]*adk.Runner)}
}

// GetOrCreate 获取缓存的 Runner，不存在则通过 factory 创建并缓存
func (c *RunnerCache) GetOrCreate(key string, factory func() (*adk.Runner, error)) (*adk.Runner, error) {
	c.mu.RLock()
	if r, exists := c.cache[key]; exists {
		c.mu.RUnlock()
		return r, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	if r, exists := c.cache[key]; exists {
		return r, nil
	}

	r, err := factory()
	if err != nil {
		return nil, err
	}

	c.cache[key] = r
	return r, nil
}

// Clear 清除所有缓存
func (c *RunnerCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]*adk.Runner)
}

var globalRunnerCache = NewRunnerCache()

// ClearRunnerCache 清除 runner 缓存
func ClearRunnerCache() {
	globalRunnerCache.Clear()
	log.Println("runner cache cleared")
}

// getOrCreateRunner 获取或创建 runner（带缓存）
func getOrCreateRunner(ctx context.Context, mode string) (*adk.Runner, error) {
	return globalRunnerCache.GetOrCreate(mode, func() (*adk.Runner, error) {
		switch mode {
		case "roundtable":
			return runner.CreateLoopAgentRunner(ctx)
		case "custom":
			return runner.CreateCustomSupervisorRunner(ctx)
		case "deep":
			return runner.CreateDeepAgentsRunner(ctx)
		default:
			return runner.CreateSupervisorRunner(ctx)
		}
	})
}

// getOrCreateAgentRunner 获取或创建指定智能体的 runner
func getOrCreateAgentRunner(ctx context.Context, agentName string) (*adk.Runner, error) {
	return globalRunnerCache.GetOrCreate("agent_"+agentName, func() (*adk.Runner, error) {
		agentInfo := agents.GetAgentByName(agentName)
		if agentInfo == nil {
			return nil, fmt.Errorf("agent not found: %s", agentName)
		}
		return runner.CreateAgentRunner(ctx, agentInfo.Creator(ctx)), nil
	})
}

// resolveRunner 按 agentName 或 mode 获取 runner
func resolveRunner(ctx context.Context, mode, agentName string) (*adk.Runner, error) {
	if agentName != "" {
		return getOrCreateAgentRunner(ctx, agentName)
	}
	return getOrCreateRunner(ctx, mode)
}

// --- 聊天输入构建 ---

// buildChatInput 构建输入消息（含历史），支持多模态
func buildChatInput(recorder *fkevent.HistoryRecorder, message string, contents []ContentPart) (messages []adk.Message, displayText string) {
	if len(contents) > 0 {
		parts := convertContentParts(contents)
		displayText = chatutil.ExtractTextFromParts(parts)
		if displayText == "" {
			displayText = message
		}
		messages = chatutil.BuildMultimodalInputMessages(recorder, displayText, parts)
	} else {
		displayText = message
		messages = chatutil.BuildInputMessages(recorder, message)
	}
	return
}

// chatHistoryPath 返回会话历史文件路径（使用 filepath.Base 防止路径穿越）
func chatHistoryPath(sessionID string) string {
	return filepath.Join(historyDir, filepath.Base(sessionID), "history.json")
}

// --- 执行后处理 ---

// saveHistory 保存聊天历史到文件
func saveHistory(recorder *fkevent.HistoryRecorder, filePath, sessionID string) {
	if err := recorder.SaveToFile(filePath); err != nil {
		log.Printf("failed to save history: session=%s, err=%v", sessionID, err)
	}
}

// updateSessionTitleAndStatus 更新会话标题（仅当标题为默认值时）和状态
func updateSessionTitleAndStatus(sessionID, userInput, status string) {
	sessionDir := sessionDirPath(sessionID)
	meta, err := fkevent.LoadMetadata(sessionDir)
	if err != nil {
		return
	}
	if userInput != "" && isDefaultTitle(meta.Title) {
		meta.Title = truncateTitle(userInput)
	}
	meta.Status = status
	meta.UpdatedAt = time.Now()
	if err := fkevent.SaveMetadata(sessionDir, meta); err != nil {
		log.Printf("failed to update session metadata: session=%s, err=%v", sessionID, err)
	}
}

// finishChat 保存历史、更新元数据、提取记忆
func finishChat(recorder *fkevent.HistoryRecorder, sessionID, userInput string) {
	saveHistory(recorder, chatHistoryPath(sessionID), sessionID)
	ensureSessionMetadata(sessionID, userInput)
	if g.MemoryManager != nil {
		g.MemoryManager.ExtractFromRecorder(recorder, sessionID)
	}
}

// ensureSessionMetadata 确保会话元数据存在，不存在则创建，已存在则更新 UpdatedAt
// 如果提供了 userInput 且当前标题是默认时间戳格式，则更新为用户输入（截断）
func ensureSessionMetadata(sessionID, userInput string) {
	sessionDir := sessionDirPath(sessionID)
	now := time.Now()
	meta, err := fkevent.LoadMetadata(sessionDir)
	if err != nil {
		// 首次创建
		title := "未命名会话"
		if userInput != "" {
			title = truncateTitle(userInput)
		}
		meta = &fkevent.SessionMetadata{
			ID:        sessionID,
			Title:     title,
			Status:    "completed",
			CreatedAt: now,
			UpdatedAt: now,
		}
	} else {
		meta.UpdatedAt = now
		meta.Status = "completed"
		// 如果标题仍是默认时间戳格式且有用户输入，则更新
		if userInput != "" && isDefaultTitle(meta.Title) {
			meta.Title = truncateTitle(userInput)
		}
	}
	if err := fkevent.SaveMetadata(sessionDir, meta); err != nil {
		log.Printf("failed to save metadata: session=%s, err=%v", sessionID, err)
	}
}

// isDefaultTitle 检查标题是否为默认标题
func isDefaultTitle(title string) bool {
	if title == "未命名会话" {
		return true
	}
	_, err := time.Parse("2006-01-02 15:04:05", title)
	return err == nil
}

// truncateTitle 截断标题，最多 50 个字符（按 rune 处理，对中文安全）
func truncateTitle(s string) string {
	const maxLen = 50
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// isConnectionClosed 检查是否为连接断开导致的错误
func isConnectionClosed(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "closed network connection") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection reset")
}

func extractInterruptMessage(interrupts []*adk.InterruptCtx) string {
	var infos []string
	for _, ic := range interrupts {
		if ic.IsRootCause && ic.Info != nil {
			if s, ok := ic.Info.(fmt.Stringer); ok {
				infos = append(infos, s.String())
			} else {
				infos = append(infos, fmt.Sprintf("%v", ic.Info))
			}
		}
	}
	if len(infos) > 0 {
		return strings.Join(infos, "\n")
	}
	return "需要审批"
}

func approvalDecisionText(result map[string]any) string {
	for _, v := range result {
		switch v {
		case 0:
			return "已拒绝"
		case 1:
			return "已允许（一次）"
		case 2:
			return "已允许（该项）"
		case 3:
			return "已全部允许"
		}
		break
	}
	return ""
}

// --- 事件/内容转换 ---

// convertEventToMap 将事件转换为前端可用的格式
func convertEventToMap(event fkevent.Event) map[string]any {
	result := map[string]any{
		"type":       event.Type,
		"agent_name": event.AgentName,
	}
	if event.RunPath != "" {
		result["run_path"] = event.RunPath
	}
	if event.Content != "" {
		result["content"] = event.Content
	}
	if event.ReasoningContent != "" {
		result["reasoning_content"] = event.ReasoningContent
	}
	if len(event.ToolCalls) > 0 {
		toolCalls := make([]map[string]any, 0, len(event.ToolCalls))
		for _, tc := range event.ToolCalls {
			toolCall := map[string]any{"name": tc.Function.Name}
			if tc.Function.Arguments != "" {
				toolCall["arguments"] = tc.Function.Arguments
			}
			toolCalls = append(toolCalls, toolCall)
		}
		result["tool_calls"] = toolCalls
	}
	if event.ActionType != "" {
		result["action_type"] = event.ActionType
	}
	if event.Detail != "" {
		result["detail"] = event.Detail
	}
	if event.Error != "" {
		result["error"] = event.Error
	}
	return result
}

// ContentPart 多模态内容部分
type ContentPart struct {
	Type       string `json:"type"`                  // text, image_url, image_base64, audio_url, video_url, file_url
	Text       string `json:"text,omitempty"`        // type=text 时的文本内容
	URL        string `json:"url,omitempty"`         // type=image_url/audio_url/video_url/file_url 时的 URL
	Base64Data string `json:"base64_data,omitempty"` // type=image_base64 时的 Base64 数据
	MIMEType   string `json:"mime_type,omitempty"`   // type=image_base64 时的 MIME 类型
	Detail     string `json:"detail,omitempty"`      // type=image_url 时的精度: high/low/auto
}

// convertContentParts 将前端传入的多模态内容转换为 eino MessageInputPart
func convertContentParts(parts []ContentPart) []schema.MessageInputPart {
	result := make([]schema.MessageInputPart, 0, len(parts))
	for _, p := range parts {
		switch p.Type {
		case "text":
			result = append(result, chatutil.TextPart(p.Text))
		case "image_url":
			detail := schema.ImageURLDetailAuto
			switch p.Detail {
			case "high":
				detail = schema.ImageURLDetailHigh
			case "low":
				detail = schema.ImageURLDetailLow
			}
			result = append(result, chatutil.ImageURLPart(p.URL, detail))
		case "image_base64":
			mimeType := p.MIMEType
			if mimeType == "" {
				mimeType = "image/png"
			}
			result = append(result, chatutil.ImageBase64Part(p.Base64Data, mimeType))
		case "audio_url":
			result = append(result, chatutil.AudioURLPart(p.URL))
		case "video_url":
			result = append(result, chatutil.VideoURLPart(p.URL))
		case "file_url":
			result = append(result, chatutil.FileURLPart(p.URL))
		}
	}
	return result
}
