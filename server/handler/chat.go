package handler

import (
	"context"
	"fkteams/agents"
	"fkteams/agents/middlewares/summary"
	"fkteams/chatutil"
	"fkteams/fkevent"
	"fkteams/g"
	"fkteams/runner"
	"fkteams/tools/command"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/cloudwego/eino/adk"
)

// Runner 缓存管理（按模式缓存）
var (
	runnerCacheMu sync.RWMutex
	runnerCache   = make(map[string]*adk.Runner)
)

// getOrCreateRunner 获取或创建 runner（带缓存，双重检查锁）
func getOrCreateRunner(ctx context.Context, mode string) *adk.Runner {
	runnerCacheMu.RLock()
	if r, exists := runnerCache[mode]; exists {
		runnerCacheMu.RUnlock()
		return r
	}
	runnerCacheMu.RUnlock()

	runnerCacheMu.Lock()
	defer runnerCacheMu.Unlock()

	if r, exists := runnerCache[mode]; exists {
		return r
	}

	var r *adk.Runner
	switch mode {
	case "roundtable":
		r = runner.CreateLoopAgentRunner(ctx)
	case "custom":
		r = runner.CreateCustomSupervisorRunner(ctx)
	case "deep":
		r = runner.CreateDeepAgentsRunner(ctx)
	default:
		r = runner.CreateSupervisorRunner(ctx)
	}

	runnerCache[mode] = r
	log.Printf("[WebSocket] runner created and cached: mode=%s", mode)
	return r
}

// getOrCreateAgentRunner 获取或创建指定智能体的 runner
func getOrCreateAgentRunner(ctx context.Context, agentName string) *adk.Runner {
	cacheKey := "agent_" + agentName

	runnerCacheMu.RLock()
	if r, exists := runnerCache[cacheKey]; exists {
		runnerCacheMu.RUnlock()
		return r
	}
	runnerCacheMu.RUnlock()

	runnerCacheMu.Lock()
	defer runnerCacheMu.Unlock()

	if r, exists := runnerCache[cacheKey]; exists {
		return r
	}

	agentInfo := agents.GetAgentByName(agentName)
	if agentInfo == nil {
		log.Printf("[WebSocket] agent not found: %s", agentName)
		return nil
	}

	agent := agentInfo.Creator(ctx)
	r := runner.CreateAgentRunner(ctx, agent)
	runnerCache[cacheKey] = r
	log.Printf("[WebSocket] agent runner created and cached: agent=%s", agentName)
	return r
}

// ClearRunnerCache 清除 runner 缓存
func ClearRunnerCache() {
	runnerCacheMu.Lock()
	defer runnerCacheMu.Unlock()
	runnerCache = make(map[string]*adk.Runner)
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

	// 构建输入消息（含历史）
	inputMessages := chatutil.BuildInputMessages(recorder, wsMsg.Message)
	countBeforeRun := recorder.GetMessageCount()
	recorder.RecordUserInput(wsMsg.Message)

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
		r = getOrCreateRunner(taskCtx, mode)
	}

	defer func() {
		if err := g.Cleaner.ExecuteAndClear(); err != nil {
			fmt.Printf("failed to cleanup resources: %v\n", err)
		}
	}()

	// 绑定事件回调（使用会话级别的 recorder）
	taskCtx = fkevent.WithCallback(taskCtx, func(event fkevent.Event) error {
		recorder.RecordEvent(event)
		return writeJSON(convertEventForWS(event))
	})

	// 设置摘要持久化回调
	taskCtx = summary.WithSummaryPersistCallback(taskCtx, func(summaryText string) {
		recorder.SetSummary(summaryText, countBeforeRun)
	})

	// 注入会话审批状态
	taskCtx = command.WithSessionApprovals(taskCtx, command.NewSessionApprovals())

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

	// 执行 agent runner（支持 HITL 中断→恢复循环）
	historyFilePath := fmt.Sprintf("%sfkteams_chat_history_%s", historyDir, sessionID)
	iter := r.Run(taskCtx, inputMessages, adk.WithCheckPointID("fkteams"))
	for {
		// 遍历事件
		var lastEvent *adk.AgentEvent
		aborted := false
		for {
			select {
			case <-taskCtx.Done():
				log.Printf("task cancelled: session=%s, saving history...", sessionID)
				saveHistory(recorder, historyFilePath, sessionID)
				return
			default:
			}

			event, ok := iter.Next()
			if !ok {
				break
			}
			lastEvent = event
			if err := fkevent.ProcessAgentEvent(taskCtx, event); err != nil {
				errMsg := err.Error()
				if strings.Contains(errMsg, "closed network connection") ||
					strings.Contains(errMsg, "broken pipe") ||
					strings.Contains(errMsg, "connection reset") {
					log.Printf("connection closed, stopping event processing: session=%s", sessionID)
					return
				}
				log.Printf("error processing event: %v", err)
				_ = writeJSON(map[string]interface{}{
					"type":  "error",
					"error": err.Error(),
				})
				aborted = true
				break
			}
		}

		if aborted {
			break
		}

		// HITL: 检查审批中断
		if lastEvent != nil && lastEvent.Action != nil && lastEvent.Action.Interrupted != nil {
			interrupts := lastEvent.Action.Interrupted.InterruptContexts
			if len(interrupts) > 0 {
				_ = writeJSON(map[string]interface{}{
					"type":    "approval_required",
					"message": "危险命令需要审批",
				})

				// 等待前端审批决定
				var decision int
				select {
				case <-taskCtx.Done():
					saveHistory(recorder, historyFilePath, sessionID)
					return
				case decision = <-approvalCh:
				}

				targets := make(map[string]any, len(interrupts))
				for _, ic := range interrupts {
					if ic.IsRootCause {
						targets[ic.ID] = decision
					}
				}

				resumeIter, err := r.ResumeWithParams(taskCtx, "fkteams", &adk.ResumeParams{
					Targets: targets,
				})
				if err != nil {
					log.Printf("Resume failed: %v", err)
					_ = writeJSON(map[string]interface{}{
						"type":  "error",
						"error": fmt.Sprintf("恢复执行失败: %v", err),
					})
					break
				}
				iter = resumeIter
				continue
			}
		}
		break
	}

	// 保存聊天历史
	saveHistory(recorder, historyFilePath, sessionID)

	// 异步提取记忆
	if g.MemManager != nil {
		g.MemManager.ExtractFromRecorder(recorder, sessionID)
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
