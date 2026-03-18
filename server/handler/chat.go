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

// getOrCreateRunner 获取或创建 runner（带缓存）
func getOrCreateRunner(ctx context.Context, mode string) (*adk.Runner, error) {
	r, err := globalRunnerCache.GetOrCreate(mode, func() (*adk.Runner, error) {
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
	if err != nil {
		return nil, err
	}
	log.Printf("[WebSocket] runner ready: mode=%s", mode)
	return r, nil
}

// getOrCreateAgentRunner 获取或创建指定智能体的 runner
func getOrCreateAgentRunner(ctx context.Context, agentName string) *adk.Runner {
	r, err := globalRunnerCache.GetOrCreate("agent_"+agentName, func() (*adk.Runner, error) {
		agentInfo := agents.GetAgentByName(agentName)
		if agentInfo == nil {
			return nil, fmt.Errorf("agent not found: %s", agentName)
		}
		agent := agentInfo.Creator(ctx)
		return runner.CreateAgentRunner(ctx, agent), nil
	})
	if err != nil {
		log.Printf("[WebSocket] %v", err)
		return nil
	}
	log.Printf("[WebSocket] agent runner ready: agent=%s", agentName)
	return r
}

// ClearRunnerCache 清除 runner 缓存
func ClearRunnerCache() {
	globalRunnerCache.Clear()
	log.Println("[WebSocket] runner cache cleared")
}

// handleChatMessage 处理聊天消息
func handleChatMessage(connCtx context.Context, tm *taskManager, wsMsg WSMessage, writeJSON func(interface{}) error) {
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

	// 获取该会话的 HistoryRecorder（自动从文件加载或创建新的）
	recorder := fkevent.GlobalSessionManager.GetOrCreate(sessionID, historyDir)

	// 构建输入消息（含历史），支持多模态
	var inputMessages []adk.Message
	var userDisplayText string

	if len(wsMsg.Contents) > 0 {
		// 多模态输入
		parts := convertWSContentParts(wsMsg.Contents)
		userDisplayText = chatutil.ExtractTextFromParts(parts)
		if userDisplayText == "" {
			userDisplayText = wsMsg.Message
		}
		inputMessages = chatutil.BuildMultimodalInputMessages(recorder, userDisplayText, parts)
	} else {
		// 纯文本输入
		userDisplayText = wsMsg.Message
		inputMessages = chatutil.BuildInputMessages(recorder, wsMsg.Message)
	}

	countBeforeRun := recorder.GetMessageCount()
	recorder.RecordUserInput(userDisplayText)

	// 获取 runner
	var r *adk.Runner
	if wsMsg.AgentName != "" {
		r = getOrCreateAgentRunner(taskCtx, wsMsg.AgentName)
		if r == nil {
			_ = writeJSON(map[string]interface{}{
				"type":  "error",
				"error": fmt.Sprintf("agent not found: %s", wsMsg.AgentName),
			})
			return
		}
		log.Printf("using specified agent: %s", wsMsg.AgentName)
	} else {
		var err error
		r, err = getOrCreateRunner(taskCtx, mode)
		if err != nil {
			_ = writeJSON(map[string]interface{}{
				"type":  "error",
				"error": fmt.Sprintf("failed to create runner: %v", err),
			})
			return
		}
	}

	defer func() {
		if err := g.Cleaner.ExecuteAndClear(); err != nil {
			fmt.Printf("failed to cleanup resources: %v\n", err)
		}
	}()

	// 标记为非交互模式（禁止终端 TUI）
	taskCtx = fkevent.WithNonInteractive(taskCtx)

	// 绑定事件回调（使用会话级别的 recorder）
	taskCtx = fkevent.WithCallback(taskCtx, func(event fkevent.Event) error {
		// interrupted 事件由 interruptHandler 记录为 approval_required 并推送，此处跳过避免重复
		if event.Type == "action" && event.ActionType == "interrupted" {
			return nil
		}
		recorder.RecordEvent(event)
		return writeJSON(convertEventForWS(event))
	})

	// 设置摘要持久化回调
	taskCtx = summary.WithSummaryPersistCallback(taskCtx, func(summaryText string) {
		recorder.SetSummary(summaryText, countBeforeRun)
	})

	// 注入统一审批注册表
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

	_ = writeJSON(map[string]interface{}{
		"type":    "processing_start",
		"message": "开始处理您的请求...",
	})

	// 使用 engine 执行（支持 HITL 中断→恢复循环）
	historyFilePath := fmt.Sprintf("%sfkteams_chat_history_%s", historyDir, sessionID)

	// 构建 HITL 中断处理器：先通知前端，再等待审批
	channelHandler := engine.ChannelHandler(approvalCh)
	interruptHandler := func(ctx context.Context, interrupts []*adk.InterruptCtx) (map[string]any, error) {
		// 提取中断信息发送给前端
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
		msg := "需要审批"
		if len(infos) > 0 {
			msg = strings.Join(infos, "\n")
		}

		// 记录审批请求到历史（不走 callback 避免重复 WebSocket 推送）
		recorder.RecordEvent(fkevent.Event{
			Type:       "action",
			ActionType: "approval_required",
			Content:    msg,
		})

		_ = writeJSON(map[string]interface{}{
			"type":    "approval_required",
			"message": msg,
		})

		result, err := channelHandler(ctx, interrupts)

		// 记录审批决定
		if err == nil {
			var decisionText string
			for _, v := range result {
				switch v {
				case 0:
					decisionText = "已拒绝"
				case 1:
					decisionText = "已允许（一次）"
				case 2:
					decisionText = "已允许（该项）"
				case 3:
					decisionText = "已全部允许"
				}
				break
			}
			if decisionText != "" {
				// 记录审批决定到历史（不走 callback，前端已在 sendApprovalDecision 中处理展示）
				recorder.RecordEvent(fkevent.Event{
					Type:       "action",
					ActionType: "approval_decision",
					Content:    decisionText,
				})
			}
		}

		return result, err
	}

	_, err := engine.New(r, "fkteams").Run(taskCtx, inputMessages, engine.WithInterruptHandler(interruptHandler))
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "closed network connection") ||
			strings.Contains(errMsg, "broken pipe") ||
			strings.Contains(errMsg, "connection reset") ||
			taskCtx.Err() != nil {
			log.Printf("task cancelled or connection closed: session=%s, saving history...", sessionID)
			saveHistory(recorder, historyFilePath, sessionID)
			return
		}
		log.Printf("error processing event: %v", err)
		_ = writeJSON(map[string]interface{}{
			"type":  "error",
			"error": errMsg,
		})
	}

	// 保存聊天历史
	saveHistory(recorder, historyFilePath, sessionID)

	// 异步提取记忆
	if g.MemoryManager != nil {
		g.MemoryManager.ExtractFromRecorder(recorder, sessionID)
	}

	_ = writeJSON(map[string]interface{}{
		"type":    "processing_end",
		"message": "处理完成",
	})
}

// saveHistory 保存聊天历史到文件
func saveHistory(recorder *fkevent.HistoryRecorder, filePath, sessionID string) {
	if err := recorder.SaveToFile(filePath); err != nil {
		log.Printf("failed to save chat history: session=%s, err=%v", sessionID, err)
	} else {
		log.Printf("chat history saved: %s", filePath)
	}
}

// convertEventForWS 将事件转换为前端可用的格式
func convertEventForWS(event fkevent.Event) map[string]interface{} {
	result := map[string]interface{}{
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
		toolCalls := make([]map[string]interface{}, 0, len(event.ToolCalls))
		for _, tc := range event.ToolCalls {
			toolCall := map[string]interface{}{"name": tc.Function.Name}
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
