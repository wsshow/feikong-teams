package handler

import (
	"context"
	"fkteams/agents"
	"fkteams/agents/middlewares/summary"
	"fkteams/chatutil"
	"fkteams/common"
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
	return filepath.Join(historyDir, common.ChatHistoryPrefix+filepath.Base(sessionID))
}

// --- 执行后处理 ---

// saveHistory 保存聊天历史到文件
func saveHistory(recorder *fkevent.HistoryRecorder, filePath, sessionID string) {
	if err := recorder.SaveToFile(filePath); err != nil {
		log.Printf("failed to save history: session=%s, err=%v", sessionID, err)
	}
}

// finishChat 保存历史、提取记忆、清理资源
func finishChat(recorder *fkevent.HistoryRecorder, sessionID string) {
	saveHistory(recorder, chatHistoryPath(sessionID), sessionID)
	if g.MemoryManager != nil {
		g.MemoryManager.ExtractFromRecorder(recorder, sessionID)
	}
	if err := g.Cleaner.ExecuteAndClear(); err != nil {
		log.Printf("failed to cleanup resources: %v", err)
	}
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
func buildInterruptHandler(recorder *fkevent.HistoryRecorder, writeJSON func(any) error, approvalCh <-chan int) engine.InterruptHandler {
	channelHandler := engine.ChannelHandler(approvalCh)
	return func(ctx context.Context, interrupts []*adk.InterruptCtx) (map[string]any, error) {
		msg := extractInterruptMessage(interrupts)

		recorder.RecordEvent(fkevent.Event{
			Type:       "action",
			ActionType: "approval_required",
			Content:    msg,
		})
		_ = writeJSON(map[string]any{
			"type":    "approval_required",
			"message": msg,
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
func wsEventCallback(recorder *fkevent.HistoryRecorder, writeJSON func(any) error) func(fkevent.Event) error {
	return func(event fkevent.Event) error {
		// interrupted 由 interruptHandler 记录为 approval_required 并推送，此处跳过避免重复
		if event.Type == "action" && event.ActionType == "interrupted" {
			return nil
		}
		recorder.RecordEvent(event)
		return writeJSON(convertEventForWS(event))
	}
}

// --- WebSocket 聊天处理 ---

// handleChatMessage 处理 WebSocket 聊天消息
func handleChatMessage(connCtx context.Context, tm *taskManager, wsMsg WSMessage, writeJSON func(any) error) {
	sessionID := wsMsg.SessionID
	if sessionID == "" {
		sessionID = "default"
	}
	mode := wsMsg.Mode
	if mode == "" {
		mode = "supervisor"
	}

	// 为任务创建独立的 context
	taskCtx, taskCancel := context.WithCancel(connCtx)
	defer taskCancel()

	tm.mu.Lock()
	tm.taskCancel = taskCancel
	tm.mu.Unlock()
	defer func() {
		tm.mu.Lock()
		tm.taskCancel = nil
		tm.mu.Unlock()
	}()

	select {
	case <-taskCtx.Done():
		return
	default:
	}

	// 获取 runner
	r, err := resolveRunner(taskCtx, mode, wsMsg.AgentName)
	if err != nil {
		_ = writeJSON(map[string]any{"type": "error", "error": err.Error()})
		return
	}

	// 构建输入消息
	recorder := fkevent.GlobalSessionManager.GetOrCreate(sessionID, historyDir)
	inputMessages, userDisplayText := buildChatInput(recorder, wsMsg.Message, wsMsg.Contents)
	countBeforeRun := recorder.GetMessageCount()
	recorder.RecordUserInput(userDisplayText)

	// 装配 context
	taskCtx = fkevent.WithNonInteractive(taskCtx)
	taskCtx = fkevent.WithCallback(taskCtx, wsEventCallback(recorder, writeJSON))
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
	tm.mu.Lock()
	tm.approvalCh = approvalCh
	tm.mu.Unlock()
	defer func() {
		tm.mu.Lock()
		tm.approvalCh = nil
		tm.mu.Unlock()
	}()

	_ = writeJSON(map[string]any{
		"type":    "processing_start",
		"message": "开始处理您的请求...",
	})

	// 执行
	interruptHandler := buildInterruptHandler(recorder, writeJSON, approvalCh)
	_, err = engine.New(r, "fkteams").Run(taskCtx, inputMessages, engine.WithInterruptHandler(interruptHandler))
	if err != nil {
		if isConnectionClosed(taskCtx, err) {
			log.Printf("task cancelled or connection closed: session=%s", sessionID)
			saveHistory(recorder, chatHistoryPath(sessionID), sessionID)
			return
		}
		log.Printf("error processing event: %v", err)
		_ = writeJSON(map[string]any{"type": "error", "error": err.Error()})
	}

	finishChat(recorder, sessionID)
	_ = writeJSON(map[string]any{
		"type":    "processing_end",
		"message": "处理完成",
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
