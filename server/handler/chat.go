package handler

import (
	"context"
	"fkteams/agentcore"
	"fkteams/agents/toolmeta"
	"fkteams/appstate"
	"fkteams/engine"
	"fkteams/events"
	"fkteams/events/chat"
	"fkteams/events/log"
	"fkteams/runner"
	"fkteams/server/handler/taskstream"
	"fkteams/tools/ask"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

var globalRunnerCache = runner.NewCache()

// ClearRunnerCache 清除 runner 缓存
func ClearRunnerCache() {
	globalRunnerCache.Clear()
	log.Println("runner cache cleared")
}

// resolveRunner 按 agentName 或 mode 获取 runner
func resolveRunner(ctx context.Context, mode, agentName string) (agentcore.Runner, error) {
	return globalRunnerCache.ResolveWithTeamFallback(ctx, mode, agentName)
}

// --- 聊天输入构建 ---

func newTurnRunID(sessionID string) string {
	return fmt.Sprintf("%s:run:%s", sessionID, uuid.NewString())
}

func queuedTurnRunID(sessionID string, queued taskstream.QueuedMessage) string {
	if queued.ID != "" {
		return fmt.Sprintf("%s:queue:%s", sessionID, queued.ID)
	}
	return newTurnRunID(sessionID)
}

func turnIDForRun(runID string) string {
	if runID == "" {
		return ""
	}
	return events.TurnID(runID, 1)
}

func attachTurnMeta(data map[string]any, runID string) map[string]any {
	if runID == "" {
		return data
	}
	data["run_id"] = runID
	data["turn_id"] = turnIDForRun(runID)
	return data
}

// buildChatInput 构建输入消息（含历史），支持多模态
func buildChatInput(recorder *eventlog.HistoryRecorder, message string, contents []ContentPart, manager appstate.MemoryManager) (input engine.TurnInput, displayText string) {
	if len(contents) > 0 {
		parts := convertContentParts(contents)
		displayText = chat.ExtractTextFromParts(parts)
		if displayText == "" {
			displayText = message
		}
		input = chat.BuildMultimodalTurnInputWithMemory(recorder, displayText, parts, manager)
	} else {
		displayText = message
		input = chat.BuildTurnInputWithMemory(recorder, message, manager)
	}
	return
}

func queuedChatMessage(kind taskstream.QueueKind, message string, contents []ContentPart) taskstream.QueuedMessage {
	queued := taskstream.QueuedMessage{
		Kind: kind,
		Text: message,
	}
	if len(contents) > 0 {
		queued.Parts = convertContentParts(contents)
		queued.DisplayText = chat.ExtractTextFromParts(queued.Parts)
		if queued.DisplayText == "" {
			queued.DisplayText = message
		}
	} else {
		queued.DisplayText = message
	}
	return queued
}

func buildQueuedChatInput(recorder *eventlog.HistoryRecorder, msg taskstream.QueuedMessage, manager appstate.MemoryManager) engine.TurnInput {
	if len(msg.Parts) > 0 {
		displayText := chat.ExtractTextFromParts(msg.Parts)
		if displayText == "" {
			displayText = msg.DisplayText
		}
		if displayText == "" {
			displayText = msg.Text
		}
		return chat.BuildMultimodalTurnInputWithMemory(recorder, displayText, msg.Parts, manager)
	}
	return chat.BuildTurnInputWithMemory(recorder, msg.Text, manager)
}

func enqueueTaskMessage(stream *taskstream.Stream, sessionID string, kind taskstream.QueueKind, message string, contents []ContentPart) taskstream.QueuedMessage {
	queued := stream.EnqueueMessage(queuedChatMessage(kind, message, contents))
	stream.Publish(map[string]any{
		"type":         events.NotifyUserMessage,
		"session_id":   sessionID,
		"content":      queued.DisplayText,
		"queued":       true,
		"queue_id":     queued.ID,
		"queue_kind":   string(queued.Kind),
		"queued_count": stream.QueuedCount(),
	})
	publishQueueUpdated(stream, sessionID)
	return queued
}

func publishQueueUpdated(stream *taskstream.Stream, sessionID string) {
	if stream == nil {
		return
	}
	stream.Publish(map[string]any{
		"type":         events.NotifyQueueUpdated,
		"session_id":   sessionID,
		"queue":        stream.QueueSnapshot(),
		"queued_count": stream.QueuedCount(),
	})
}

func publishQueuedExecutionStart(stream *taskstream.Stream, sessionID string, queued taskstream.QueuedMessage, runID string) {
	if queued.Kind == taskstream.QueueFollowUp {
		stream.Publish(attachTurnMeta(map[string]any{
			"type":             events.NotifyUserMessage,
			"session_id":       sessionID,
			"content":          queued.DisplayText,
			"queue_id":         queued.ID,
			"queue_kind":       string(queued.Kind),
			"queued_executing": true,
		}, runID))
	}

	message := "继续处理排队消息..."
	if queued.Kind == taskstream.QueueSteering {
		message = "应用转向消息..."
	}
	stream.Publish(attachTurnMeta(map[string]any{
		"type":             events.NotifyProcessingStart,
		"session_id":       sessionID,
		"message":          message,
		"queue_id":         queued.ID,
		"queue_kind":       string(queued.Kind),
		"content":          queued.DisplayText,
		"queued_executing": true,
	}, runID))
}

func buildSteeringSource(stream *taskstream.Stream, recorder *eventlog.HistoryRecorder, sessionID string, currentRunID func() string) agentcore.SteeringSource {
	return func(context.Context) ([]agentcore.Message, error) {
		queued := stream.TakeSteeringMessages(1)
		if len(queued) == 0 {
			return nil, nil
		}
		publishQueueUpdated(stream, sessionID)
		messages := make([]agentcore.Message, 0, len(queued))
		for _, msg := range queued {
			message := msg.Message()
			recorder.RecordUserMessage(message)
			messages = append(messages, message)
		}
		runID := ""
		if currentRunID != nil {
			runID = currentRunID()
		}
		publishQueuedExecutionStart(stream, sessionID, queued[0], runID)
		return messages, nil
	}
}

// chatHistoryPath 返回会话历史文件路径（使用 filepath.Base 防止路径穿越）
func chatHistoryPath(sessionID string) string {
	return filepath.Join(historyDir, filepath.Base(sessionID), eventlog.HistoryFileName)
}

// --- 执行后处理 ---

// saveHistory 保存聊天历史到文件
func saveHistory(recorder *eventlog.HistoryRecorder, filePath, sessionID string) {
	if err := recorder.SaveToFile(filePath); err != nil {
		log.Printf("failed to save history: session=%s, err=%v", sessionID, err)
	}
}

// updateSessionTitleAndStatus 更新会话标题（仅当标题为默认值时）和状态
func updateSessionTitleAndStatus(sessionID, userInput, status string) {
	sessionDir := sessionDirPath(sessionID)
	meta, err := eventlog.LoadMetadata(sessionDir)
	if err != nil {
		return
	}
	if userInput != "" && isDefaultTitle(meta.Title) {
		meta.Title = truncateTitle(userInput)
	}
	meta.Status = status
	meta.UpdatedAt = time.Now()
	if err := eventlog.SaveMetadata(sessionDir, meta); err != nil {
		log.Printf("failed to update session metadata: session=%s, err=%v", sessionID, err)
	}
}

// finishChat 保存历史、更新元数据、提取记忆
func finishChat(recorder *eventlog.HistoryRecorder, sessionID, userInput string, manager appstate.MemoryManager) {
	recorder.FinalizeCurrent()
	saveHistory(recorder, chatHistoryPath(sessionID), sessionID)
	ensureSessionMetadataWithStatus(sessionID, userInput, "completed")
	if manager != nil {
		manager.ExtractFromRecorder(recorder, sessionID)
	}
}

func finishCancelledChat(recorder *eventlog.HistoryRecorder, sessionID, userInput string) {
	recorder.RecordCancelled("任务已取消")
	saveHistory(recorder, chatHistoryPath(sessionID), sessionID)
	ensureSessionMetadataWithStatus(sessionID, userInput, "cancelled")
}

func finishErrorChat(recorder *eventlog.HistoryRecorder, sessionID, userInput string, err error) {
	if err != nil {
		recorder.RecordEvent(events.Event{
			Type:    events.EventError,
			Content: err.Error(),
			Error:   err.Error(),
		})
	}
	recorder.FinalizeCurrent()
	saveHistory(recorder, chatHistoryPath(sessionID), sessionID)
	ensureSessionMetadataWithStatus(sessionID, userInput, "error")
}

func ensureSessionMetadataWithStatus(sessionID, userInput, status string) {
	sessionDir := sessionDirPath(sessionID)
	now := time.Now()
	meta, err := eventlog.LoadMetadata(sessionDir)
	if err != nil {
		// 首次创建
		title := "未命名会话"
		if userInput != "" {
			title = truncateTitle(userInput)
		}
		meta = &eventlog.SessionMetadata{
			ID:        sessionID,
			Title:     title,
			Status:    status,
			CreatedAt: now,
			UpdatedAt: now,
		}
	} else {
		meta.UpdatedAt = now
		meta.Status = status
		// 如果标题仍是默认时间戳格式且有用户输入，则更新
		if userInput != "" && isDefaultTitle(meta.Title) {
			meta.Title = truncateTitle(userInput)
		}
	}
	if err := eventlog.SaveMetadata(sessionDir, meta); err != nil {
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

func extractInterruptMessage(interrupts []agentcore.Interrupt) string {
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

// askResponseText 从中断结果中提取 ask_response 的可读文本
func askResponseText(result map[string]any) string {
	for _, v := range result {
		if resp, ok := v.(*ask.AskResponse); ok {
			var parts []string
			if len(resp.Selected) > 0 {
				parts = append(parts, strings.Join(resp.Selected, ", "))
			}
			if resp.FreeText != "" {
				parts = append(parts, resp.FreeText)
			}
			if len(parts) > 0 {
				return strings.Join(parts, " | ")
			}
			return "已回答"
		}
	}
	return fmt.Sprintf("%v", result)
}

// extractAskInfo 从中断上下文中提取 ask_questions 信息
func extractAskInfo(interrupts []agentcore.Interrupt) *ask.AskInfo {
	for _, ic := range interrupts {
		if ic.IsRootCause {
			if info, ok := ic.Info.(*ask.AskInfo); ok {
				return info
			}
		}
	}
	return nil
}

// --- 事件/内容转换 ---

// convertEventToMap 将事件转换为前端可用的格式
func convertEventToMap(event events.Event) map[string]any {
	result := map[string]any{
		"type":       event.Type,
		"agent_name": event.AgentName,
	}
	if event.RunID != "" {
		result["run_id"] = event.RunID
	}
	if event.EventID != "" {
		result["event_id"] = event.EventID
	}
	if event.Sequence != 0 {
		result["sequence"] = event.Sequence
	}
	if !event.CreatedAt.IsZero() {
		result["created_at"] = event.CreatedAt
	}
	if event.TurnID != "" {
		result["turn_id"] = event.TurnID
	}
	if event.MessageID != "" {
		result["message_id"] = event.MessageID
	}
	if event.Role != "" {
		result["role"] = event.Role
	}
	if event.DeltaKind != "" {
		result["delta_kind"] = event.DeltaKind
	}
	if event.MessageID != "" && event.DeltaKind != "" {
		result["stream_id"] = fmt.Sprintf("%s:%s", event.MessageID, event.DeltaKind)
		if event.Sequence != 0 {
			result["chunk_index"] = event.Sequence
		}
	}
	if event.Type == events.EventMessageDelta && event.Content != "" {
		result["delta"] = event.Content
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
	if toolCallsFromEvent := events.ToolCallsFromEvent(event); len(toolCallsFromEvent) > 0 {
		toolCalls := make([]map[string]any, 0, len(toolCallsFromEvent))
		for i, tc := range toolCallsFromEvent {
			display := toolmeta.FormatToolDisplay(tc.Function.Name)
			toolCall := map[string]any{
				"name":         tc.Function.Name,
				"display_name": display.DisplayName,
				"kind":         display.Kind,
			}
			if tc.ID != "" {
				toolCall["id"] = tc.ID
			}
			if ref := events.ToolCallRefAt(event, tc, i); ref != "" {
				toolCall["ref"] = ref
			}
			if tc.Index != nil {
				toolCall["index"] = *tc.Index
			}
			if display.Target != "" {
				toolCall["target"] = display.Target
			}
			if tc.Function.Arguments != "" {
				toolCall["arguments"] = tc.Function.Arguments
			}
			toolCalls = append(toolCalls, toolCall)
		}
		result["tool_calls"] = toolCalls
		if len(toolCalls) == 1 {
			result["tool_call"] = toolCalls[0]
		}
	}
	if event.ActionType != "" {
		result["action_type"] = event.ActionType
	}
	if event.ToolCallID != "" {
		result["tool_call_id"] = event.ToolCallID
	}
	if event.ToolCallRef != "" {
		result["tool_call_ref"] = event.ToolCallRef
	}
	if event.ToolName != "" {
		result["tool_name"] = event.ToolName
		display := toolmeta.FormatToolDisplay(event.ToolName)
		result["tool_display_name"] = display.DisplayName
		result["tool_kind"] = display.Kind
		if display.Target != "" {
			result["tool_target"] = display.Target
		}
	}
	if event.ToolCallIndex != nil {
		result["tool_call_index"] = *event.ToolCallIndex
	}
	if events.IsMemberEvent(event) {
		result["is_member_event"] = true
	}
	if event.MemberCallID != "" {
		result["member_call_id"] = event.MemberCallID
	}
	if event.MemberToolName != "" {
		result["member_tool_name"] = event.MemberToolName
	}
	if event.MemberName != "" {
		result["member_name"] = event.MemberName
	}
	if event.MemberOrder != nil {
		result["member_order"] = *event.MemberOrder
	}
	if event.ParentToolCallID != "" {
		result["parent_tool_call_id"] = event.ParentToolCallID
	}
	if event.ParentToolName != "" {
		result["parent_tool_name"] = event.ParentToolName
	}
	if event.Detail != "" {
		result["detail"] = event.Detail
	}
	if event.Error != "" {
		result["error"] = event.Error
	}
	if event.PromptTokens > 0 {
		result["prompt_tokens"] = event.PromptTokens
	}
	if event.CompletionTokens > 0 {
		result["completion_tokens"] = event.CompletionTokens
	}
	if event.TotalTokens > 0 {
		result["total_tokens"] = event.TotalTokens
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

// convertContentParts 将前端传入的多模态内容转换为核心内容部分
func convertContentParts(parts []ContentPart) []agentcore.ContentPart {
	result := make([]agentcore.ContentPart, 0, len(parts))
	for _, p := range parts {
		switch p.Type {
		case "text":
			result = append(result, chat.TextPart(p.Text))
		case "image_url":
			detail := "auto"
			switch p.Detail {
			case "high":
				detail = "high"
			case "low":
				detail = "low"
			}
			result = append(result, chat.ImageURLPart(p.URL, detail))
		case "image_base64":
			mimeType := p.MIMEType
			if mimeType == "" {
				mimeType = "image/png"
			}
			result = append(result, chat.ImageBase64Part(p.Base64Data, mimeType))
		case "audio_url":
			result = append(result, chat.AudioURLPart(p.URL))
		case "video_url":
			result = append(result, chat.VideoURLPart(p.URL))
		case "file_url":
			result = append(result, chat.FileURLPart(p.URL))
		}
	}
	return result
}
