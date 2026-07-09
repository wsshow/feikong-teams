package handler

import (
	"context"
	"fkteams/internal/adapters/storage/file/history"
	"fkteams/internal/app/agent/catalog/toolmeta"
	"fkteams/internal/app/appstate"
	appchat "fkteams/internal/app/chat"
	"fkteams/internal/app/chat/taskstream"
	"fkteams/internal/app/config"
	"fkteams/internal/app/tools/ask"
	domainevent "fkteams/internal/domain/event"
	domainmessage "fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/approval"
	"fkteams/internal/runtime/events"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

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

func attachContentParts(data taskstream.Event, parts []domainmessage.ContentPart) taskstream.Event {
	return data.WithContentParts(parts)
}

func messageContentParts(message domainmessage.Message) []domainmessage.ContentPart {
	if len(message.ContentParts) > 0 {
		return append([]domainmessage.ContentPart(nil), message.ContentParts...)
	}
	return nil
}

// buildChatInput 构建输入消息（含历史），支持多模态
func buildChatInput(recorder *eventlog.HistoryRecorder, message string, contents []ContentPart, manager appstate.MemoryManager) (input domainmessage.TurnInput, displayText string) {
	if len(contents) > 0 {
		parts := convertContentParts(contents)
		displayText = appchat.ExtractTextFromParts(parts)
		if displayText == "" {
			displayText = message
		}
		input = appchat.BuildMultimodalTurnInputWithMemory(recorder, displayText, parts, manager)
	} else {
		displayText = message
		input = appchat.BuildTurnInputWithMemory(recorder, message, manager)
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
		queued.DisplayText = appchat.ExtractTextFromParts(queued.Parts)
		if queued.DisplayText == "" {
			queued.DisplayText = message
		}
	} else {
		queued.DisplayText = message
	}
	return queued
}

func buildQueuedChatInput(recorder *eventlog.HistoryRecorder, msg taskstream.QueuedMessage, manager appstate.MemoryManager) domainmessage.TurnInput {
	if len(msg.Parts) > 0 {
		displayText := appchat.ExtractTextFromParts(msg.Parts)
		if displayText == "" {
			displayText = msg.DisplayText
		}
		if displayText == "" {
			displayText = msg.Text
		}
		return appchat.BuildMultimodalTurnInputWithMemory(recorder, displayText, msg.Parts, manager)
	}
	return appchat.BuildTurnInputWithMemory(recorder, msg.Text, manager)
}

func enqueueTaskMessage(stream *taskstream.Stream, sessionID string, kind taskstream.QueueKind, message string, contents []ContentPart) taskstream.QueuedMessage {
	queued := stream.EnqueueMessage(queuedChatMessage(kind, message, contents))
	publishQueueUpdated(stream, sessionID)
	return queued
}

func (rt *Runtime) enqueueTaskMessage(stream *taskstream.Stream, sessionID string, kind taskstream.QueueKind, message string, contents []ContentPart) taskstream.QueuedMessage {
	queued := enqueueTaskMessage(stream, sessionID, kind, message, contents)
	rt.persistQueueSnapshot(sessionID, stream)
	return queued
}

func publishQueueUpdated(stream *taskstream.Stream, sessionID string) {
	if stream == nil {
		return
	}
	runID, turnID := stream.CurrentTurn()
	payload := standardEventPayload(sessionID, events.QueueUpdated(runID, turnID), nil)
	payload["queue"] = stream.QueueSnapshot()
	payload["queued_count"] = stream.QueuedCount()
	stream.Publish(payload)
}

func publishQueuedExecutionStart(stream *taskstream.Stream, sessionID string, queued taskstream.QueuedMessage, runID string) {
	turnID := turnIDForRun(runID)
	stream.SetTurn(runID, turnID)
	message := "继续处理排队消息..."
	if queued.Kind == taskstream.QueueSteering {
		message = "应用转向消息..."
	}
	payload := standardEventPayload(sessionID, events.ProcessingStart(runID, turnID, message), nil)
	payload["message"] = message
	payload["queue_id"] = queued.ID
	payload["queue_kind"] = string(queued.Kind)
	payload["content"] = queued.DisplayText
	payload["queued_executing"] = true
	stream.Publish(payload)
}

func buildSteeringSource(stream *taskstream.Stream, recorder *eventlog.HistoryRecorder, sessionID string, currentRunID func() string, persistQueue func()) runtimeport.SteeringSource {
	return func(context.Context) ([]domainmessage.Message, error) {
		queued := stream.TakeSteeringMessages(1)
		if len(queued) == 0 {
			return nil, nil
		}
		publishQueueUpdated(stream, sessionID)
		if persistQueue != nil {
			persistQueue()
		}
		messages := make([]domainmessage.Message, 0, len(queued))
		for _, msg := range queued {
			message := msg.Message()
			runID := ""
			if currentRunID != nil {
				runID = currentRunID()
			}
			turnID := turnIDForRun(runID)
			userEvent := events.UserMessage(runID, turnID, fmt.Sprintf("%s:user:%s", turnID, msg.ID), message)
			recorder.RecordEvent(userEvent)
			stream.Publish(standardEventPayload(sessionID, userEvent, nil))
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

func extractInterruptMessage(interrupts []runtimeport.Interrupt) string {
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

func configuredApprovalRegistry() *approval.Registry {
	cfg := config.Get()
	if cfg == nil {
		return approval.NewDefaultRegistry()
	}
	return approval.NewDefaultSelectiveRegistry(cfg.Tools.Approval.AutoApprove)
}

// askResponseText 从中断结果中提取 ask_answered 的可读文本
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
func extractAskInfo(interrupts []runtimeport.Interrupt) *ask.AskInfo {
	if interrupt := extractAskInterrupt(interrupts); interrupt != nil {
		return interrupt.Info
	}
	return nil
}

type askInterrupt struct {
	ID    string
	Info  *ask.AskInfo
	Event events.Event
}

func extractAskInterrupt(interrupts []runtimeport.Interrupt) *askInterrupt {
	for _, ic := range interrupts {
		if !ic.IsRootCause {
			continue
		}
		info, ok := ic.Info.(*ask.AskInfo)
		if !ok {
			continue
		}
		return &askInterrupt{
			ID:    ic.ID,
			Info:  info,
			Event: memberEventFromInterrupt(ic),
		}
	}
	return nil
}

func askInterruptID(interrupts []runtimeport.Interrupt) string {
	if interrupt := extractAskInterrupt(interrupts); interrupt != nil && interrupt.ID != "" {
		return interrupt.ID
	}
	for _, ic := range interrupts {
		if ic.ID != "" {
			return ic.ID
		}
	}
	return ""
}

func interruptMemberEvent(interrupts []runtimeport.Interrupt) events.Event {
	for _, ic := range interrupts {
		if ic.MemberCallID == "" {
			continue
		}
		return memberEventFromInterrupt(ic)
	}
	return events.Event{}
}

func memberEventFromInterrupt(ic runtimeport.Interrupt) events.Event {
	if ic.MemberCallID == "" {
		return events.Event{}
	}
	return events.Event{
		MemberCallID:     ic.MemberCallID,
		MemberToolName:   ic.MemberToolName,
		MemberName:       ic.MemberName,
		MemberOrder:      ic.MemberOrder,
		ParentToolCallID: ic.MemberCallID,
		ParentToolName:   ic.MemberToolName,
	}
}

func memberEventFromMetadata(metadata runtimeport.InterruptMetadata) events.Event {
	if metadata.MemberCallID == "" {
		return events.Event{}
	}
	return events.Event{
		MemberCallID:     metadata.MemberCallID,
		MemberToolName:   metadata.MemberToolName,
		MemberName:       metadata.MemberName,
		MemberOrder:      metadata.MemberOrder,
		ParentToolCallID: metadata.MemberCallID,
		ParentToolName:   metadata.MemberToolName,
	}
}

func buildMemberAskRuntimeHandler(stream *taskstream.Stream, recorder *eventlog.HistoryRecorder, sessionID string) ask.RuntimeHandler {
	return func(ctx context.Context, req ask.RuntimeRequest) (*ask.AskResponse, error) {
		responseCh, err := stream.BeginAsk(req.ID)
		if err != nil {
			return nil, err
		}
		defer stream.CompleteAsk(req.ID)

		memberEvent := memberEventFromMetadata(req.Metadata)
		askEvent := askRequestedEvent(memberEvent, req.ID, req.Info)
		askEvent.ToolCallID = req.ToolCallID
		if req.ToolCallID != "" {
			askEvent.ToolCallRef = "tool_call:" + req.ToolCallID
		}
		askEvent.ToolName = req.ToolName
		askEvent = events.NormalizeEvent(askEvent)
		recorder.RecordEvent(askEvent)

		askPayload := standardEventPayload(sessionID, askEvent, nil).
			With("ask_id", req.ID).
			With("question", req.Info.Question).
			With("options", req.Info.Options).
			With("multi_select", req.Info.MultiSelect)
		stream.Publish(askPayload)

		var raw any
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case raw = <-responseCh:
		}
		resp, ok := raw.(*ask.AskResponse)
		if !ok || resp == nil {
			return nil, fmt.Errorf("invalid ask response")
		}
		if resp.AskID == "" {
			resp.AskID = req.ID
		}

		answerEvent := askAnsweredEvent(memberEvent, req.ID, resp, askResponseText(map[string]any{req.ID: resp}))
		answerEvent.ToolCallID = req.ToolCallID
		if req.ToolCallID != "" {
			answerEvent.ToolCallRef = "tool_call:" + req.ToolCallID
		}
		answerEvent.ToolName = req.ToolName
		answerEvent = events.NormalizeEvent(answerEvent)
		recorder.RecordEvent(answerEvent)
		return resp, nil
	}
}

func askRequestedEvent(base events.Event, askID string, info *ask.AskInfo) events.Event {
	event := base
	event.Type = events.EventAskRequested
	event.Detail = askID
	if info == nil {
		event.Ask = &domainevent.AskPayload{ID: askID}
		return event
	}
	event.Content = info.Question
	event.Ask = &domainevent.AskPayload{
		ID:          askID,
		Question:    info.Question,
		Options:     append([]string(nil), info.Options...),
		MultiSelect: info.MultiSelect,
	}
	return event
}

func askAnsweredEvent(base events.Event, askID string, resp *ask.AskResponse, content string) events.Event {
	event := base
	event.Type = events.EventAskAnswered
	event.Content = content
	event.Detail = askID
	event.Ask = &domainevent.AskPayload{ID: askID}
	if resp != nil {
		event.Ask.Selected = append([]string(nil), resp.Selected...)
		event.Ask.FreeText = resp.FreeText
	}
	return event
}

func askResponseFromResult(askID string, result runtimeport.InterruptDecisions) *ask.AskResponse {
	if result == nil {
		return nil
	}
	if askID != "" {
		if resp, ok := result[askID].(*ask.AskResponse); ok {
			return resp
		}
	}
	for _, raw := range result {
		if resp, ok := raw.(*ask.AskResponse); ok {
			return resp
		}
	}
	return nil
}

func attachMemberPayload(payload map[string]any, event events.Event) map[string]any {
	if event.MemberCallID != "" {
		payload["is_member_event"] = true
		payload["member_call_id"] = event.MemberCallID
	}
	if event.MemberToolName != "" {
		payload["member_tool_name"] = event.MemberToolName
	}
	if event.MemberName != "" {
		payload["member_name"] = event.MemberName
	}
	if event.MemberOrder != nil {
		payload["member_order"] = *event.MemberOrder
	}
	if event.ParentToolCallID != "" {
		payload["parent_tool_call_id"] = event.ParentToolCallID
	}
	if event.ParentToolName != "" {
		payload["parent_tool_name"] = event.ParentToolName
	}
	return payload
}

// --- 事件/内容转换 ---

// convertEventToMap 将事件转换为前端可用的格式
func convertEventToMap(event events.Event) map[string]any {
	return convertEventToMapWithResolver(event, nil)
}

type fallbackToolDisplayResolver struct{}

func (fallbackToolDisplayResolver) FormatToolDisplay(name string) toolmeta.ToolDisplay {
	return toolmeta.FallbackDisplay(name)
}

func (rt *Runtime) convertEventToMap(event events.Event) map[string]any {
	return convertEventToMapWithResolver(event, rt.ToolDisplays)
}

func convertEventToMapWithResolver(event events.Event, resolver toolmeta.Resolver) map[string]any {
	if resolver == nil {
		resolver = fallbackToolDisplayResolver{}
	}
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
	if event.BlockID != "" {
		result["block_id"] = event.BlockID
	}
	if event.BlockType != "" {
		result["block_type"] = event.BlockType
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
	if event.RunPath != "" {
		result["run_path"] = event.RunPath
	}
	if event.Content != "" {
		result["content"] = event.Content
	}
	if event.Message != nil {
		if parts := messageContentParts(*event.Message); len(parts) > 0 {
			result["content_parts"] = parts
		}
	}
	if event.ReasoningContent != "" {
		result["reasoning_content"] = event.ReasoningContent
	}
	if toolCallsFromEvent := events.ToolCallsFromEvent(event); len(toolCallsFromEvent) > 0 {
		toolCalls := make([]map[string]any, 0, len(toolCallsFromEvent))
		for i, tc := range toolCallsFromEvent {
			display := resolver.FormatToolDisplay(tc.Function.Name)
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
	if event.ToolCallID != "" {
		result["tool_call_id"] = event.ToolCallID
	}
	if event.ToolCallRef != "" {
		result["tool_call_ref"] = event.ToolCallRef
	}
	if event.ToolName != "" {
		result["tool_name"] = event.ToolName
		display := resolver.FormatToolDisplay(event.ToolName)
		result["tool_display_name"] = display.DisplayName
		result["tool_kind"] = display.Kind
		if display.Target != "" {
			result["tool_target"] = display.Target
		}
	}
	if event.ToolCallIndex != nil {
		result["tool_call_index"] = *event.ToolCallIndex
	}
	if event.ToolArgs != "" {
		result["tool_args"] = event.ToolArgs
	}
	if event.ToolResult != "" {
		result["tool_result"] = event.ToolResult
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
		attachFriendlyError(result, event.Error)
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
	if event.Ask != nil {
		result["ask_id"] = event.Ask.ID
		if event.Ask.Question != "" {
			result["question"] = event.Ask.Question
		}
		if len(event.Ask.Options) > 0 {
			result["options"] = append([]string(nil), event.Ask.Options...)
		}
		if event.Ask.MultiSelect {
			result["multi_select"] = event.Ask.MultiSelect
		}
		if len(event.Ask.Selected) > 0 {
			result["selected"] = append([]string(nil), event.Ask.Selected...)
		}
		if event.Ask.FreeText != "" {
			result["free_text"] = event.Ask.FreeText
		}
	}
	if event.Usage != nil {
		result["usage"] = event.Usage
	}
	if event.Notice != nil {
		result["notice"] = event.Notice
	}
	if event.Approval != nil {
		result["approval"] = event.Approval
	}
	return result
}

func standardEventPayload(sessionID string, event events.Event, resolver toolmeta.Resolver) taskstream.Event {
	event = events.NormalizeEvent(event)
	payload := taskstream.Event(convertEventToMapWithResolver(event, resolver))
	if sessionID != "" {
		payload["session_id"] = sessionID
	}
	return payload
}

func standardMessageEventPayload(sessionID, runID, turnID, message string) taskstream.Event {
	payload := standardEventPayload(sessionID, events.ProcessingStart(runID, turnID, message), nil)
	payload["message"] = message
	return payload
}

func processingEndEventPayload(sessionID, runID, message string) taskstream.Event {
	turnID := turnIDForRun(runID)
	payload := standardEventPayload(sessionID, events.ProcessingEnd(runID, turnID, message), nil)
	payload["message"] = message
	return payload
}

func cancelledEventPayload(sessionID, runID, message string) taskstream.Event {
	turnID := turnIDForRun(runID)
	payload := standardEventPayload(sessionID, events.Cancelled(runID, turnID, message), nil)
	payload["message"] = message
	return payload
}

func isDeltaEvent(event events.Event) bool {
	switch event.Type {
	case events.EventAssistantText, events.EventAssistantReasoning, events.EventToolCallArguments, events.EventToolCallResult:
		return true
	default:
		return false
	}
}

func attachFriendlyError(result map[string]any, raw string) map[string]any {
	if raw == "" {
		return result
	}
	friendly := events.NormalizeFriendlyError(raw)
	result["error_code"] = friendly.Code
	result["error_title"] = friendly.Title
	result["display_error"] = friendly.Message
	result["technical_error"] = friendly.TechnicalDetail
	if len(friendly.Suggestions) > 0 {
		result["error_suggestions"] = friendly.Suggestions
	}
	return result
}

func errorEventPayload(sessionID, raw string) taskstream.Event {
	payload := standardEventPayload(sessionID, events.Event{Type: events.EventError, Content: raw, Error: raw}, nil)
	payload["message"] = raw
	return taskstream.Event(attachFriendlyError(payload, raw))
}

// ContentPart 多模态内容部分
type ContentPart struct {
	Type       string `json:"type"`                  // text, image_url, image_base64, audio_url, video_url, file_url
	Name       string `json:"name,omitempty"`        // 附件名称
	Text       string `json:"text,omitempty"`        // type=text 时的文本内容
	URL        string `json:"url,omitempty"`         // type=image_url/audio_url/video_url/file_url 时的 URL
	Base64Data string `json:"base64_data,omitempty"` // type=image_base64 时的 Base64 数据
	MIMEType   string `json:"mime_type,omitempty"`   // type=image_base64 时的 MIME 类型
	Detail     string `json:"detail,omitempty"`      // type=image_url 时的精度: high/low/auto
}

// convertContentParts 将前端传入的多模态内容转换为核心内容部分
func convertContentParts(parts []ContentPart) []domainmessage.ContentPart {
	result := make([]domainmessage.ContentPart, 0, len(parts))
	for _, p := range parts {
		switch p.Type {
		case "text":
			part := appchat.TextPart(p.Text)
			part.Name = p.Name
			result = append(result, part)
		case "image_url":
			detail := "auto"
			switch p.Detail {
			case "high":
				detail = "high"
			case "low":
				detail = "low"
			}
			part := appchat.ImageURLPart(p.URL, detail)
			part.Name = p.Name
			result = append(result, part)
		case "image_base64":
			mimeType := p.MIMEType
			if mimeType == "" {
				mimeType = "image/png"
			}
			part := appchat.ImageBase64Part(p.Base64Data, mimeType)
			part.Name = p.Name
			result = append(result, part)
		case "audio_url":
			part := appchat.AudioURLPart(p.URL)
			part.Name = p.Name
			result = append(result, part)
		case "video_url":
			part := appchat.VideoURLPart(p.URL)
			part.Name = p.Name
			result = append(result, part)
		case "file_url":
			part := appchat.FileURLPart(p.URL)
			part.Name = p.Name
			result = append(result, part)
		}
	}
	return result
}
