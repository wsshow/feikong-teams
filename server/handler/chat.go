package handler

import (
	"context"
	"fkteams/agents"
	"fkteams/fkevent"
	"fkteams/g"
	"fkteams/runner"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
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

	// 构建输入消息（含历史）
	historyFilePath := fmt.Sprintf("%sfkteams_chat_history_%s", historyDir, sessionID)
	inputMessages := buildInputMessages(historyFilePath, wsMsg.Message)
	fkevent.GlobalHistoryRecorder.RecordUserInput(wsMsg.Message)

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

	// 绑定事件回调
	taskCtx = fkevent.WithCallback(taskCtx, func(event fkevent.Event) error {
		fkevent.GlobalHistoryRecorder.RecordEvent(event)
		return writeJSON(convertEventForWS(event))
	})

	_ = writeJSON(map[string]interface{}{
		"type":    "processing_start",
		"message": "开始处理您的请求...",
	})

	// 执行 agent runner
	iter := r.Run(taskCtx, inputMessages, adk.WithCheckPointID("fkteams"))
	for {
		select {
		case <-taskCtx.Done():
			log.Printf("task cancelled: session=%s, saving history...", sessionID)
			saveHistory(historyFilePath, sessionID)
			return
		default:
		}

		event, ok := iter.Next()
		if !ok {
			break
		}
		if err := fkevent.ProcessAgentEvent(taskCtx, event); err != nil {
			log.Printf("error processing event: %v", err)
			_ = writeJSON(map[string]interface{}{
				"type":  "error",
				"error": err.Error(),
			})
			break
		}
	}

	// 保存聊天历史
	saveHistory(historyFilePath, sessionID)

	_ = writeJSON(map[string]interface{}{
		"type":    "processing_end",
		"message": "处理完成",
	})
}

// buildInputMessages 构建输入消息（包含历史记录）
func buildInputMessages(historyFilePath, userInput string) []adk.Message {
	var inputMessages []adk.Message

	if err := fkevent.GlobalHistoryRecorder.LoadFromFile(historyFilePath); err == nil {
		log.Printf("auto-loaded chat history from: %s", historyFilePath)
	}

	agentMessages := fkevent.GlobalHistoryRecorder.GetMessages()
	if len(agentMessages) > 0 {
		var historyMessage strings.Builder
		for _, msg := range agentMessages {
			fmt.Fprintf(&historyMessage, "%s: %s\n", msg.AgentName, msg.GetTextContent())
		}
		inputMessages = append(inputMessages, schema.SystemMessage(
			fmt.Sprintf("以下是之前的对话历史:\n---\n%s\n---\n", historyMessage.String()),
		))
	}

	inputMessages = append(inputMessages, schema.UserMessage(userInput))
	return inputMessages
}

// saveHistory 保存聊天历史到文件
func saveHistory(filePath, sessionID string) {
	if err := fkevent.GlobalHistoryRecorder.SaveToFile(filePath); err != nil {
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
	if event.Error != "" {
		result["error"] = event.Error
	}
	return result
}
