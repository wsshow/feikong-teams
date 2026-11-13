package handler

import (
	"context"
	"errors"
	"fkteams/internal/adapters/storage/file/history"
	"fkteams/internal/app/agent/catalog/toolmeta"
	"fkteams/internal/app/appstate"
	appchat "fkteams/internal/app/chat"
	"fkteams/internal/app/chat/taskstream"
	"fkteams/internal/app/config"
	"fkteams/internal/app/tools/ask"
	domainmessage "fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/approval"
	"fkteams/internal/runtime/events"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// --- 聊天输入构建 ---

// publishStreamFailure 为执行前或收尾失败发布明确终态，避免留下永远运行中的快照。
func publishStreamFailure(stream *taskstream.Stream, sessionID, message string) {
	stream.SetStatus("error")
	stream.Publish(errorEventPayload(sessionID, message))
	runID, _ := stream.CurrentTurn()
	terminal := processingEndEventPayload(sessionID, runID, "处理失败")
	terminal["status"] = "error"
	stream.Publish(terminal)
}

func newTurnRunID(sessionID string) string {
	return fmt.Sprintf("%s:run:%s", sessionID, uuid.NewString())
}

func queuedTurnRunID(sessionID string, queued taskstream.QueuedMessage) string {
	if queued.ID != "" {
		return appchat.QueuedRunID(sessionID, queued)
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
	return appchat.BuildQueuedInput(recorder, msg, manager)
}

func enqueueTaskMessage(stream *taskstream.Stream, sessionID string, kind taskstream.QueueKind, message string, contents []ContentPart) (taskstream.QueuedMessage, error) {
	queued, err := stream.EnqueueMessageIfProcessing(queuedChatMessage(kind, message, contents))
	if err != nil {
		return taskstream.QueuedMessage{}, err
	}
	publishQueueUpdated(stream, sessionID)
	return queued, nil
}

func (rt *Runtime) enqueueTaskMessage(stream *taskstream.Stream, sessionID string, kind taskstream.QueueKind, message string, contents []ContentPart) (taskstream.QueuedMessage, error) {
	return rt.Queues.Enqueue(stream, queuedChatMessage(kind, message, contents))
}

func writeTaskQueueError(c *gin.Context, err error) {
	status := http.StatusConflict
	switch {
	case errors.Is(err, appchat.ErrQueuePersistence):
		status = http.StatusInternalServerError
	case errors.Is(err, taskstream.ErrQueuedMessageNotFound):
		status = http.StatusNotFound
	case errors.Is(err, taskstream.ErrQueueMessageTooLarge):
		status = http.StatusRequestEntityTooLarge
	case errors.Is(err, taskstream.ErrQueueFull):
		status = http.StatusTooManyRequests
	}
	Fail(c, status, taskQueueErrorMessage(err))
}

func taskQueueErrorMessage(err error) string {
	switch {
	case errors.Is(err, appchat.ErrQueuePersistence):
		return "failed to persist queued messages"
	case errors.Is(err, taskstream.ErrQueuedMessageNotFound):
		return err.Error()
	case errors.Is(err, taskstream.ErrQueueMessageTooLarge):
		return "queued message is too large"
	case errors.Is(err, taskstream.ErrQueueFull):
		return "task queue is full; retry after queued messages are processed"
	default:
		return "task is finishing; retry the request"
	}
}

func publishQueueUpdated(stream *taskstream.Stream, sessionID string) {
	if stream == nil {
		return
	}
	publishQueueSnapshot(stream, sessionID, stream.QueueSnapshot())
}

func publishQueueSnapshot(stream *taskstream.Stream, sessionID string, items []taskstream.QueuedMessage) {
	runID, turnID := stream.CurrentTurn()
	payload := standardEventPayload(sessionID, events.QueueUpdated(runID, turnID), nil)
	payload["queue"] = items
	payload["queued_count"] = len(items)
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

func buildSteeringSource(stream *taskstream.Stream, sessionID string, currentRunID func() string, queues *appchat.QueueService) runtimeport.SteeringSource {
	return func(context.Context) ([]domainmessage.Message, error) {
		queued, err := queues.TakeSteering(stream, 1)
		if err != nil {
			return nil, err
		}
		if len(queued) == 0 {
			return nil, nil
		}
		messages := make([]domainmessage.Message, 0, len(queued))
		for _, msg := range queued {
			message := msg.Message()

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

func configuredApprovalRegistry() *approval.Registry {
	cfg := config.Get()
	if cfg == nil {
		return approval.NewDefaultRegistry()
	}
	return approval.NewDefaultSelectiveRegistry(cfg.Tools.Approval.AutoApprove)
}

func buildMemberAskRuntimeHandler(stream *taskstream.Stream) ask.RuntimeHandler {
	return func(ctx context.Context, req ask.RuntimeRequest) (*ask.AskResponse, error) {
		responseCh, err := stream.AskResponseChannel(req.ID)
		if err != nil {
			return nil, err
		}
		defer stream.CompleteAsk(req.ID)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case raw := <-responseCh:
			resp, ok := raw.(*ask.AskResponse)
			if !ok || resp == nil {
				return nil, fmt.Errorf("invalid ask response")
			}
			if resp.AskID == "" {
				resp.AskID = req.ID
			}
			return resp, nil
		}
	}
}

func prepareStreamInteraction(stream *taskstream.Stream, e events.Event) error {
	switch e.Type {
	case events.EventAskRequested:
		if e.Ask != nil {
			_, err := stream.BeginAsk(e.Ask.ID)
			return err
		}
	case events.EventApprovalRequested:
		stream.BeginInterrupt(taskstream.InterruptApproval)
	}
	return nil
}

// --- 事件/内容转换 ---

// convertEventToMap 将事件转换为前端可用的格式
func convertEventToMap(event events.Event) map[string]any {
	return convertEventToMapWithResolver(event, nil)
}

func (rt *Runtime) convertEventToMap(event events.Event) map[string]any {
	return convertEventToMapWithResolver(event, rt.ToolDisplays)
}

func convertEventToMapWithResolver(event events.Event, resolver toolmeta.Resolver) map[string]any {
	return taskstream.RuntimeEventPayload(event, resolver)
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
	payload["status"] = "completed"
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
	FileID     string `json:"file_id,omitempty"`     // 模型供应商托管的文件引用
	MIMEType   string `json:"mime_type,omitempty"`   // type=image_base64 时的 MIME 类型
	Detail     string `json:"detail,omitempty"`      // type=image_url 时的精度: high/low/auto
}

// convertContentParts 将前端传入的多模态内容转换为核心内容部分
func convertContentParts(parts []ContentPart) []domainmessage.ContentPart {
	result := make([]domainmessage.ContentPart, 0, len(parts))
	for _, p := range parts {
		var part domainmessage.ContentPart
		switch p.Type {
		case "text":
			part = appchat.TextPart(p.Text)
		case "image_url":
			detail := "auto"
			switch p.Detail {
			case "high":
				detail = "high"
			case "low":
				detail = "low"
			}
			part = appchat.ImageURLPart(p.URL, detail)
		case "image_base64":
			mimeType := p.MIMEType
			if mimeType == "" {
				mimeType = "image/png"
			}
			part = appchat.ImageBase64Part(p.Base64Data, mimeType)
		case "audio_url":
			part = appchat.AudioURLPart(p.URL)
		case "video_url":
			part = appchat.VideoURLPart(p.URL)
		case "file_url":
			part = appchat.FileURLPart(p.URL)
		default:
			continue
		}
		part.Name = p.Name
		part.FileID = p.FileID
		if p.Base64Data != "" {
			part.Base64Data = p.Base64Data
		}
		if p.MIMEType != "" {
			part.MIMEType = p.MIMEType
		}
		result = append(result, part)
	}
	return result
}
