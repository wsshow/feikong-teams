package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"fkteams/appstate"
	"fkteams/events"
	"fkteams/internal/adapters/storage/file/history"
	appchat "fkteams/internal/app/chat"
	"fkteams/internal/app/chat/taskstream"
	domainmessage "fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/turn"
	"fkteams/tools/approval"
	"fkteams/tools/ask"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// StreamStartRequest 启动流式任务请求
type StreamStartRequest struct {
	SessionID string        `json:"session_id"`
	Message   string        `json:"message"`
	Mode      string        `json:"mode"`
	AgentName string        `json:"agent_name"`
	Contents  []ContentPart `json:"contents"`
}

// StreamStartHandler 启动流式任务（任务在后台执行，前端通过 SSE 订阅事件流）
func StreamStartHandler() gin.HandlerFunc {
	return StreamStartHandlerWithState(nil)
}

// StreamStartHandlerWithState 启动流式任务并使用显式应用状态。
func StreamStartHandlerWithState(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req StreamStartRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
			return
		}

		if req.Message == "" && len(req.Contents) == 0 {
			Fail(c, http.StatusBadRequest, "message or contents is required")
			return
		}

		sessionID := req.SessionID
		if sessionID == "" {
			sessionID = uuid.New().String()
		}
		if !validateSessionID(sessionID) {
			Fail(c, http.StatusBadRequest, "invalid session ID")
			return
		}

		if existing := GlobalStreams.Get(sessionID); existing != nil && existing.Status() == "processing" {
			queued := enqueueTaskMessage(existing, sessionID, taskstream.QueueFollowUp, req.Message, req.Contents)
			OK(c, gin.H{
				"session_id":   sessionID,
				"status":       "queued",
				"message":      "message queued",
				"queue_kind":   queued.Kind,
				"queued_count": existing.QueuedCount(),
			})
			return
		}

		mode := req.Mode
		if mode == "" {
			mode = "team"
		}

		ctx := appstate.WithState(context.Background(), state)
		r, err := resolveRunner(ctx, mode, req.AgentName)
		if err != nil {
			log.Printf("failed to resolve runner: mode=%s, agent=%s, err=%v", mode, req.AgentName, err)
			status := http.StatusInternalServerError
			if req.AgentName != "" {
				status = http.StatusBadRequest
			}
			Fail(c, status, err.Error())
			return
		}

		recorder := eventlog.GlobalSessionManager.GetOrCreate(sessionID, historyDir)
		manager := memoryFromState(state)
		turnInput, userDisplayText := buildChatInput(recorder, req.Message, req.Contents, manager)

		// 创建任务——统一使用 GlobalStreams
		taskCtx, taskCancel := context.WithCancel(ctx)
		stream := GlobalStreams.Register(taskstream.StreamConfig{
			SessionID:  sessionID,
			Cancel:     taskCancel,
			CleanupTTL: 5 * time.Minute,
			Mode:       mode,
			AgentName:  req.AgentName,
		})

		updateSessionTitleAndStatus(sessionID, userDisplayText, "processing")
		initialRunID := newTurnRunID(sessionID)
		stream.Publish(attachContentParts(attachTurnMeta(map[string]any{
			"type":       events.NotifyUserMessage,
			"session_id": sessionID,
			"content":    userDisplayText,
		}, initialRunID), messageContentParts(turnInput.Message)))

		stream.Publish(attachTurnMeta(map[string]any{
			"type":       events.NotifyProcessingStart,
			"session_id": sessionID,
			"message":    "开始处理您的请求...",
		}, initialRunID))

		// 后台执行任务
		go runStreamTask(taskCtx, stream, sessionID, r, recorder, turnInput, userDisplayText, manager, initialRunID)

		OK(c, gin.H{
			"session_id": sessionID,
			"status":     "processing",
			"message":    "task started",
		})
	}
}

// StreamSteerHandler 在运行中的流式任务下一次模型调用前注入转向消息。
func StreamSteerHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req StreamStartRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
			return
		}
		if req.Message == "" && len(req.Contents) == 0 {
			Fail(c, http.StatusBadRequest, "message or contents is required")
			return
		}
		if req.SessionID == "" {
			Fail(c, http.StatusBadRequest, "session_id is required")
			return
		}
		if !validateSessionID(req.SessionID) {
			Fail(c, http.StatusBadRequest, "invalid session ID")
			return
		}

		stream := GlobalStreams.Get(req.SessionID)
		if stream == nil || stream.Status() != "processing" {
			Fail(c, http.StatusNotFound, "no running task for this session")
			return
		}

		queued := enqueueTaskMessage(stream, req.SessionID, taskstream.QueueSteering, req.Message, req.Contents)
		OK(c, gin.H{
			"session_id":   req.SessionID,
			"status":       "queued",
			"message":      "steering queued",
			"queue_kind":   queued.Kind,
			"queued_count": stream.QueuedCount(),
		})
	}
}

// StreamQueueHandler 返回运行中任务的未消费队列快照。
func StreamQueueHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")
		stream := streamForQueueRequest(c, sessionID)
		if stream == nil {
			return
		}
		OK(c, gin.H{
			"session_id":   sessionID,
			"queue":        stream.QueueSnapshot(),
			"queued_count": stream.QueuedCount(),
		})
	}
}

// StreamQueueUpdateHandler 修改尚未执行的队列项。
func StreamQueueUpdateHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")
		queueID := c.Param("queueID")
		stream := streamForQueueRequest(c, sessionID)
		if stream == nil {
			return
		}
		var req StreamStartRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
			return
		}
		if req.Message == "" && len(req.Contents) == 0 {
			Fail(c, http.StatusBadRequest, "message or contents is required")
			return
		}
		queued := queuedChatMessage(taskstream.QueueFollowUp, req.Message, req.Contents)
		updated, ok := stream.UpdateQueuedMessage(queueID, queued.Text, queued.Parts, queued.DisplayText)
		if !ok {
			Fail(c, http.StatusNotFound, "queued message not found")
			return
		}
		publishQueueUpdated(stream, sessionID)
		OK(c, gin.H{
			"session_id": sessionID,
			"queue_item": updated,
			"queue":      stream.QueueSnapshot(),
		})
	}
}

// StreamQueueDeleteHandler 删除尚未执行的队列项。
func StreamQueueDeleteHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")
		queueID := c.Param("queueID")
		stream := streamForQueueRequest(c, sessionID)
		if stream == nil {
			return
		}
		removed, ok := stream.RemoveQueuedMessage(queueID)
		if !ok {
			Fail(c, http.StatusNotFound, "queued message not found")
			return
		}
		publishQueueUpdated(stream, sessionID)
		OK(c, gin.H{
			"session_id": sessionID,
			"queue_item": removed,
			"queue":      stream.QueueSnapshot(),
		})
	}
}

type StreamQueueMoveRequest struct {
	Direction string `json:"direction"`
}

type StreamQueueKindRequest struct {
	Kind string `json:"kind"`
}

// StreamQueueKindHandler 切换尚未执行队列项的语义类型。
func StreamQueueKindHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")
		queueID := c.Param("queueID")
		stream := streamForQueueRequest(c, sessionID)
		if stream == nil {
			return
		}
		var req StreamQueueKindRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
			return
		}
		kind := taskstream.QueueKind(req.Kind)
		if kind != taskstream.QueueFollowUp && kind != taskstream.QueueSteering {
			Fail(c, http.StatusBadRequest, "kind must be follow_up or steering")
			return
		}
		updated, ok := stream.SetQueuedMessageKind(queueID, kind)
		if !ok {
			Fail(c, http.StatusNotFound, "queued message not found")
			return
		}
		publishQueueUpdated(stream, sessionID)
		OK(c, gin.H{
			"session_id": sessionID,
			"queue_item": updated,
			"queue":      stream.QueueSnapshot(),
		})
	}
}

// StreamQueueMoveHandler 调整尚未执行队列项在同类队列中的顺序。
func StreamQueueMoveHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")
		queueID := c.Param("queueID")
		stream := streamForQueueRequest(c, sessionID)
		if stream == nil {
			return
		}
		var req StreamQueueMoveRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
			return
		}
		direction := 0
		switch req.Direction {
		case "up":
			direction = -1
		case "down":
			direction = 1
		default:
			Fail(c, http.StatusBadRequest, "direction must be up or down")
			return
		}
		moved, ok := stream.MoveQueuedMessage(queueID, direction)
		if !ok {
			Fail(c, http.StatusNotFound, "queued message not found")
			return
		}
		publishQueueUpdated(stream, sessionID)
		OK(c, gin.H{
			"session_id": sessionID,
			"queue_item": moved,
			"queue":      stream.QueueSnapshot(),
		})
	}
}

func streamForQueueRequest(c *gin.Context, sessionID string) *taskstream.Stream {
	if !validateSessionID(sessionID) {
		Fail(c, http.StatusBadRequest, "invalid session ID")
		return nil
	}
	stream := GlobalStreams.Get(sessionID)
	if stream == nil || stream.Status() != "processing" {
		Fail(c, http.StatusNotFound, "no running task for this session")
		return nil
	}
	return stream
}

// runStreamTask 后台执行流式任务
func runStreamTask(ctx context.Context, stream *taskstream.Stream, sessionID string, r runtimeport.Runner, recorder *eventlog.HistoryRecorder, turnInput domainmessage.TurnInput, userDisplayText string, manager appstate.MemoryManager, initialRunID string) {
	defer stream.Done()

	interruptHandler := buildStreamInterruptHandler(stream, recorder, sessionID)
	currentRunID := initialRunID
	if currentRunID == "" {
		currentRunID = newTurnRunID(sessionID)
	}
	steeringSource := buildSteeringSource(stream, recorder, sessionID, func() string { return currentRunID })
	currentInput := turnInput
	currentDisplayText := userDisplayText
	chatService := appchat.NewService()
	for {
		_, runErr := chatService.RunTurn(ctx, appchat.TurnRequest{
			SessionID: sessionID,
			Runner:    r,
			Input:     currentInput,
		},
			appchat.WithRunID(currentRunID),
			appchat.NonInteractive(),
			appchat.OnEvent(func(event events.Event) error {
				if event.Type == events.EventAction && event.ActionType == events.ActionInterrupted {
					return nil
				}
				recorder.RecordEvent(event)
				data := convertEventToMap(event)
				data["session_id"] = sessionID
				stream.Publish(data)
				return nil
			}),
			appchat.WithHistory(recorder),
			appchat.OnInterrupt(runtimeport.InterruptHandler(interruptHandler)),
			appchat.WithContext(approval.RegistryContext(approval.NewDefaultRegistry())),
			appchat.WithContext(func(ctx context.Context) context.Context {
				return ask.WithRuntimeHandler(ctx, buildMemberAskRuntimeHandler(stream, recorder, sessionID))
			}),
			appchat.WithContext(func(ctx context.Context) context.Context {
				return runtimeport.WithSteeringSource(ctx, steeringSource)
			},
			),
		)
		if runErr != nil {
			if isConnectionClosed(ctx, runErr) {
				log.Printf("stream task cancelled: session=%s", sessionID)
				stream.SetStatus("cancelled")
				stream.Publish(map[string]any{
					"type":       events.NotifyCancelled,
					"session_id": sessionID,
					"message":    "任务已取消",
				})
				finishCancelledChat(recorder, sessionID, currentDisplayText)
				return
			}
			log.Printf("stream task error: session=%s, err=%v", sessionID, runErr)
			stream.SetStatus("error")
			stream.Publish(errorEventPayload(sessionID, runErr.Error()))
			finishErrorChat(recorder, sessionID, currentDisplayText, runErr)
			return
		}

		recorder.FinalizeCurrent()
		saveHistory(recorder, chatHistoryPath(sessionID), sessionID)
		if queued, ok := stream.DequeueNextMessage(); ok {
			publishQueueUpdated(stream, sessionID)
			currentDisplayText = queued.DisplayText
			currentInput = buildQueuedChatInput(recorder, queued, manager)
			currentRunID = queuedTurnRunID(sessionID, queued)
			updateSessionTitleAndStatus(sessionID, currentDisplayText, "processing")
			publishQueuedExecutionStart(stream, sessionID, queued, currentRunID)
			continue
		}

		stream.SetStatus("completed")
		stream.Publish(map[string]any{
			"type":       events.NotifyProcessingEnd,
			"session_id": sessionID,
			"message":    "处理完成",
		})
		ensureSessionMetadataWithStatus(sessionID, currentDisplayText, "completed")
		if manager != nil {
			manager.ExtractFromRecorder(recorder, sessionID)
		}
		return
	}
}

// StreamStopHandler 停止正在运行的流式任务
func StreamStopHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")
		if !validateSessionID(sessionID) {
			Fail(c, http.StatusBadRequest, "invalid session ID")
			return
		}

		stream := GlobalStreams.Get(sessionID)
		if stream == nil {
			Fail(c, http.StatusNotFound, "no task found for this session")
			return
		}
		if stream.Status() != "processing" {
			Fail(c, http.StatusConflict, fmt.Sprintf("task is not running, current status: %s", stream.Status()))
			return
		}

		stream.Cancel()
		OK(c, gin.H{
			"session_id": sessionID,
			"message":    "task stop requested",
		})
	}
}

// StreamSubscribeHandler SSE 订阅流式事件，支持断线重连。
// 前端通过 Last-Event-ID 或 ?offset= 指定起始位置，
// 重连后可无损续接之前断开的事件流。
// 仅对内存中有活跃/刚完成任务的 session 有效；已完成的历史数据应通过会话接口获取。
func StreamSubscribeHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")
		if !validateSessionID(sessionID) {
			Fail(c, http.StatusBadRequest, "invalid session ID")
			return
		}

		stream := GlobalStreams.Get(sessionID)
		if stream == nil {
			Fail(c, http.StatusNotFound, "no active task for this session")
			return
		}

		// 解析起始 offset：优先 Last-Event-ID（SSE 标准重连机制），其次 query 参数
		var offset uint64
		if lastID := c.GetHeader("Last-Event-ID"); lastID != "" {
			if parsed, err := strconv.ParseUint(lastID, 10, 64); err == nil {
				offset = parsed + 1
			}
		} else if offsetStr := c.Query("offset"); offsetStr != "" {
			if parsed, err := strconv.ParseUint(offsetStr, 10, 64); err == nil {
				offset = parsed
			}
		}

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")

		notify, unsub := stream.Watch()
		defer unsub()

		ctx := c.Request.Context()
		flusher := c.Writer

		writeSSE := func(event taskstream.IndexedEvent) bool {
			data, _ := json.Marshal(event.Data)
			_, err := fmt.Fprintf(flusher, "id: %d\ndata: %s\n\n", event.ID, data)
			flusher.Flush()
			return err == nil
		}

		for {
			events := stream.EventsSince(offset)
			for _, e := range events {
				if !writeSSE(e) {
					return
				}
				offset = e.ID + 1
			}

			if stream.IsDone() {
				// 任务已结束，最后再读一次确保不丢失尾部事件
				for _, e := range stream.EventsSince(offset) {
					writeSSE(e)
				}
				return
			}

			select {
			case <-ctx.Done():
				return
			case <-notify:
			}
		}
	}
}

// StreamStatusHandler 获取流式任务状态。
func StreamStatusHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")
		if !validateSessionID(sessionID) {
			Fail(c, http.StatusBadRequest, "invalid session ID")
			return
		}

		stream := GlobalStreams.Get(sessionID)
		if stream != nil {
			result := gin.H{
				"session_id":  sessionID,
				"status":      stream.Status(),
				"has_task":    true,
				"mode":        stream.Mode(),
				"event_count": stream.EventCount(),
				"created_at":  stream.CreatedAt(),
			}
			if doneAt := stream.DoneAt(); !doneAt.IsZero() {
				result["finished_at"] = doneAt
			}
			if stream.AgentName() != "" {
				result["agent_name"] = stream.AgentName()
			}
			OK(c, result)
			return
		}

		// 无活跃任务，返回会话元数据（供前端判断会话是否存在）
		sessionDir := sessionDirPath(sessionID)
		meta, err := eventlog.LoadMetadata(sessionDir)
		if err != nil {
			Fail(c, http.StatusNotFound, "session not found")
			return
		}
		OK(c, gin.H{
			"session_id": sessionID,
			"status":     meta.Status,
			"has_task":   false,
			"title":      meta.Title,
			"created_at": meta.CreatedAt,
			"updated_at": meta.UpdatedAt,
		})
	}
}

// StreamApprovalHandler 提交 HITL 审批决定
func StreamApprovalHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			SessionID string `json:"session_id" binding:"required"`
			Decision  int    `json:"decision"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, "invalid request body")
			return
		}
		if !validateSessionID(req.SessionID) {
			Fail(c, http.StatusBadRequest, "invalid session ID")
			return
		}

		stream := GlobalStreams.Get(req.SessionID)
		if stream == nil || stream.Status() != "processing" {
			Fail(c, http.StatusNotFound, "no running task for this session")
			return
		}

		if err := stream.SubmitInterrupt(taskstream.InterruptApproval, req.Decision); err == nil {
			OK(c, gin.H{"message": "approval submitted"})
		} else {
			Fail(c, http.StatusConflict, "no pending approval request")
		}
	}
}

// StreamAskResponseHandler 提交 ask_questions 回答
func StreamAskResponseHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			SessionID string   `json:"session_id" binding:"required"`
			AskID     string   `json:"ask_id"`
			Selected  []string `json:"selected"`
			FreeText  string   `json:"free_text"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, "invalid request body")
			return
		}
		if !validateSessionID(req.SessionID) {
			Fail(c, http.StatusBadRequest, "invalid session ID")
			return
		}

		stream := GlobalStreams.Get(req.SessionID)
		if stream == nil || stream.Status() != "processing" {
			Fail(c, http.StatusNotFound, "no running task for this session")
			return
		}

		resp := &ask.AskResponse{
			AskID:    req.AskID,
			Selected: req.Selected,
			FreeText: req.FreeText,
		}
		if err := stream.SubmitAskResponse(req.AskID, resp); err == nil {
			OK(c, gin.H{"message": "response submitted"})
		} else {
			Fail(c, http.StatusConflict, "no pending ask request")
		}
	}
}

// StreamEventsHandler 获取当前任务的已缓冲事件（非 SSE，一次性拉取）。
func StreamEventsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")
		if !validateSessionID(sessionID) {
			Fail(c, http.StatusBadRequest, "invalid session ID")
			return
		}

		stream := GlobalStreams.Get(sessionID)
		if stream == nil {
			Fail(c, http.StatusNotFound, "no active task for this session")
			return
		}

		var offset uint64
		if offsetStr := c.Query("offset"); offsetStr != "" {
			if parsed, err := strconv.ParseUint(offsetStr, 10, 64); err == nil {
				offset = parsed
			}
		}

		events := stream.EventsSince(offset)
		OK(c, gin.H{
			"session_id":  sessionID,
			"status":      stream.Status(),
			"events":      events,
			"event_count": stream.EventCount(),
			"done":        stream.IsDone(),
		})
	}
}

// ==================== 内部辅助 ====================

// buildStreamInterruptHandler 构建流式任务的 HITL 中断处理器
func buildStreamInterruptHandler(stream *taskstream.Stream, recorder *eventlog.HistoryRecorder, sessionID string) turn.InterruptHandler {
	channelHandler := turn.ChannelHandler(stream.InterruptCh())
	return func(ctx context.Context, interrupts []runtimeport.Interrupt) (map[string]any, error) {
		// 检查是否为 ask_questions 中断
		if askInterrupt := extractAskInterrupt(interrupts); askInterrupt != nil {
			stream.BeginInterruptWithID(taskstream.InterruptAsk, askInterrupt.ID)
			defer stream.CompleteInterrupt(taskstream.InterruptAsk)

			info := askInterrupt.Info
			askID := askInterrupt.ID
			memberEvent := askInterrupt.Event
			askEvent := memberEvent
			askEvent.Type = events.EventAction
			askEvent.ActionType = events.ActionAskQuestions
			askEvent.Content = info.Question
			askEvent.Detail = askID
			recorder.RecordEvent(askEvent)
			stream.Publish(attachMemberPayload(map[string]any{
				"type":         events.NotifyAskQuestions,
				"session_id":   sessionID,
				"ask_id":       askID,
				"question":     info.Question,
				"options":      info.Options,
				"multi_select": info.MultiSelect,
			}, memberEvent))

			result, err := turn.ChannelTargetHandler(stream.InterruptCh(), askID)(ctx, interrupts)
			if err == nil {
				answerEvent := memberEvent
				answerEvent.Type = events.EventAction
				answerEvent.ActionType = events.ActionAskResponse
				answerEvent.Content = askResponseText(result)
				answerEvent.Detail = askID
				recorder.RecordEvent(answerEvent)
			}
			return result, err
		}

		// 默认审批流程
		msg := extractInterruptMessage(interrupts)

		stream.BeginInterrupt(taskstream.InterruptApproval)
		defer stream.CompleteInterrupt(taskstream.InterruptApproval)

		recorder.RecordEvent(events.Event{
			Type:       events.EventAction,
			ActionType: events.ActionApprovalRequired,
			Content:    msg,
		})
		stream.Publish(map[string]any{
			"type":       events.NotifyApprovalRequired,
			"session_id": sessionID,
			"message":    msg,
		})

		result, err := channelHandler(ctx, interrupts)
		if err == nil {
			if text := approvalDecisionText(result); text != "" {
				recorder.RecordEvent(events.Event{
					Type:       events.EventAction,
					ActionType: events.ActionApprovalDecision,
					Content:    text,
				})
			}
		}

		return result, err
	}
}
