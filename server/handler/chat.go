package handler

import (
	"context"
	"fkteams/agents"
	"fkteams/agents/middlewares/summary"
	"fkteams/chatutil"
	"fkteams/engine"
	"fkteams/fkevent"
	"fkteams/g"
	"fkteams/runner"
	"fkteams/tools/approval"
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
func buildChatInput(recorder *fkevent.HistoryRecorder, message string, contents []WSContentPart) (messages []adk.Message, displayText string) {
	if len(contents) > 0 {
		parts := convertWSContentParts(contents)
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

// --- HITL 中断处理器 ---

// buildInterruptHandler 构建 WebSocket 聊天的 HITL 中断处理器
func buildInterruptHandler(recorder *fkevent.HistoryRecorder, sessionID string, writeJSON func(any) error, approvalCh <-chan int) engine.InterruptHandler {
	channelHandler := engine.ChannelHandler(approvalCh)
	return func(ctx context.Context, interrupts []*adk.InterruptCtx) (map[string]any, error) {
		msg := extractInterruptMessage(interrupts)

		recorder.RecordEvent(fkevent.Event{
			Type:       "action",
			ActionType: "approval_required",
			Content:    msg,
		})
		_ = writeJSON(map[string]any{
			"type":       "approval_required",
			"session_id": sessionID,
			"message":    msg,
		})

		result, err := channelHandler(ctx, interrupts)

		if err == nil {
			if text := approvalDecisionText(result); text != "" {
				recorder.RecordEvent(fkevent.Event{
					Type:       "action",
					ActionType: "approval_decision",
					Content:    text,
				})
			}
		}

		return result, err
	}
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

// --- WebSocket 事件回调 ---

// wsEventCallback 构建 WebSocket 模式的事件回调
func wsEventCallback(recorder *fkevent.HistoryRecorder, sessionID string, writeJSON func(any) error) func(fkevent.Event) error {
	return func(event fkevent.Event) error {
		// interrupted 由 interruptHandler 记录为 approval_required 并推送，此处跳过避免重复
		if event.Type == "action" && event.ActionType == "interrupted" {
			return nil
		}
		recorder.RecordEvent(event)
		data := convertEventForWS(event)
		data["session_id"] = sessionID
		return writeJSON(data)
	}
}

// --- WebSocket 聊天处理 ---

// handleChatMessage 处理 WebSocket 聊天消息
func handleChatMessage(connCtx context.Context, sm *sessionManager, wsMsg WSMessage, writeJSON func(any) error) {
	sessionID := wsMsg.SessionID
	mode := wsMsg.Mode
	if mode == "" {
		mode = "supervisor"
	}

	// 为任务创建独立的 context
	taskCtx, taskCancel := context.WithCancel(connCtx)
	defer taskCancel()

	// 注册任务（同一 session 的旧任务会被自动取消）
	taskID := sm.startTask(sessionID, taskCancel)
	defer sm.removeTask(sessionID, taskID)

	select {
	case <-taskCtx.Done():
		return
	default:
	}

	// 获取 runner
	r, err := resolveRunner(taskCtx, mode, wsMsg.AgentName)
	if err != nil {
		log.Printf("failed to resolve runner: session=%s, err=%v", sessionID, err)
		_ = writeJSON(map[string]any{"type": "error", "session_id": sessionID, "error": err.Error()})
		return
	}

	// 构建输入消息
	recorder := fkevent.GlobalSessionManager.GetOrCreate(sessionID, historyDir)
	inputMessages, userDisplayText := buildChatInput(recorder, wsMsg.Message, wsMsg.Contents)
	countBeforeRun := recorder.GetMessageCount()
	recorder.RecordUserInput(userDisplayText)

	// 装配 context
	taskCtx = fkevent.WithNonInteractive(taskCtx)
	taskCtx = fkevent.WithCallback(taskCtx, wsEventCallback(recorder, sessionID, writeJSON))
	taskCtx = summary.WithSummaryPersistCallback(taskCtx, func(summaryText string) {
		recorder.SetSummary(summaryText, countBeforeRun)
	})
	taskCtx = approval.WithRegistry(taskCtx, approval.NewRegistry(
		approval.StoreConfig{Name: approval.StoreCommand},
		approval.StoreConfig{Name: approval.StoreFile, Matcher: approval.DirMatchFunc},
		approval.StoreConfig{Name: approval.StoreDispatch},
	))

	// 初始化 HITL 审批通道
	approvalCh := make(chan int, 1)
	sm.setApprovalCh(sessionID, taskID, approvalCh)
	defer sm.setApprovalCh(sessionID, taskID, nil)

	// 更新会话标题（首次提交时从默认标题更新为用户输入）和状态
	updateSessionTitleAndStatus(sessionID, userDisplayText, "processing")

	_ = writeJSON(map[string]any{
		"type":       "processing_start",
		"session_id": sessionID,
		"message":    "开始处理您的请求...",
	})

	// 执行
	interruptHandler := buildInterruptHandler(recorder, sessionID, writeJSON, approvalCh)
	_, err = engine.New(r, "fkteams").Run(taskCtx, inputMessages, engine.WithInterruptHandler(interruptHandler))
	if err != nil {
		if isConnectionClosed(taskCtx, err) {
			log.Printf("task cancelled or connection closed: session=%s", sessionID)
			saveHistory(recorder, chatHistoryPath(sessionID), sessionID)
			ensureSessionMetadata(sessionID, userDisplayText)
			return
		}
		log.Printf("failed to run task: session=%s, err=%v", sessionID, err)
		_ = writeJSON(map[string]any{"type": "error", "session_id": sessionID, "error": err.Error()})
	}

	finishChat(recorder, sessionID, userDisplayText)
	_ = writeJSON(map[string]any{
		"type":       "processing_end",
		"session_id": sessionID,
		"message":    "处理完成",
	})
}

// --- 事件/内容转换 ---

// convertEventForWS 将事件转换为前端可用的格式
func convertEventForWS(event fkevent.Event) map[string]any {
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

// convertWSContentParts 将前端传入的多模态内容转换为 eino MessageInputPart
func convertWSContentParts(parts []WSContentPart) []schema.MessageInputPart {
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
