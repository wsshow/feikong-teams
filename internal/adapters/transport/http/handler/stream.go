package handler

import (
	"context"
	"encoding/json"
	"fkteams/internal/runtime/log"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"fkteams/internal/adapters/storage/file/history"
	"fkteams/internal/app/appstate"
	appchat "fkteams/internal/app/chat"
	"fkteams/internal/app/chat/taskstream"
	"fkteams/internal/app/tools/ask"
	domainmessage "fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/events"

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

// StreamStartHandlerWithState 启动流式任务并使用显式应用状态。

// StreamStartHandlerWithState 启动流式任务并使用当前 HTTP runtime。
func (rt *Runtime) StreamStartHandlerWithState(state *appstate.State) gin.HandlerFunc {
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

		if existing := rt.Streams.Get(sessionID); existing != nil && existing.Status() == "processing" {
			queued := rt.enqueueTaskMessage(existing, sessionID, taskstream.QueueFollowUp, req.Message, req.Contents)
			OK(c, gin.H{
				"session_id":   sessionID,
				"status":       "queued",
				"message":      "message queued",
				"queue_kind":   queued.Kind,
				"queue":        existing.QueueSnapshot(),
				"queued_count": existing.QueuedCount(),
			})
			return
		}

		mode := req.Mode
		if mode == "" {
			mode = "team"
		}

		ctx := appstate.WithState(context.Background(), state)
		r, err := rt.resolveRunner(ctx, mode, req.AgentName)
		if err != nil {
			log.Printf("failed to resolve runner: mode=%s, agent=%s, err=%v", mode, req.AgentName, err)
			status := http.StatusInternalServerError
			if req.AgentName != "" {
				status = http.StatusBadRequest
			}
			Fail(c, status, err.Error())
			return
		}

		recorder := rt.recorder(sessionID)
		manager := memoryFromState(state)
		turnInput, userDisplayText := buildChatInput(recorder, req.Message, req.Contents, manager)

		// 创建任务并交给当前 HTTP runtime 的 stream hub 管理。
		taskCtx, taskCancel := context.WithCancel(ctx)
		stream := rt.Streams.Register(taskstream.StreamConfig{
			SessionID:  sessionID,
			Cancel:     taskCancel,
			CleanupTTL: 5 * time.Minute,
			Mode:       mode,
			AgentName:  req.AgentName,
		})
		rt.restorePersistentQueue(sessionID, stream)

		rt.updateSessionTitleAndStatus(sessionID, userDisplayText, "processing")
		initialRunID := newTurnRunID(sessionID)
		initialTurnID := turnIDForRun(initialRunID)
		stream.SetTurn(initialRunID, initialTurnID)
		stream.Publish(standardMessageEventPayload(sessionID, initialRunID, initialTurnID, "开始处理您的请求..."))

		// 后台执行任务
		go rt.runStreamTask(taskCtx, stream, sessionID, r, recorder, turnInput, userDisplayText, manager, initialRunID)

		OK(c, gin.H{
			"session_id":   sessionID,
			"status":       "processing",
			"message":      "task started",
			"queue":        stream.QueueSnapshot(),
			"queued_count": stream.QueuedCount(),
		})
	}
}

// StreamSteerHandler 在运行中的流式任务下一次模型调用前注入转向消息。

// StreamSteerHandler 在当前 HTTP runtime 中注入转向消息。
func (rt *Runtime) StreamSteerHandler() gin.HandlerFunc {
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

		stream := rt.Streams.Get(req.SessionID)
		if stream == nil || stream.Status() != "processing" {
			Fail(c, http.StatusNotFound, "no running task for this session")
			return
		}

		queued := rt.enqueueTaskMessage(stream, req.SessionID, taskstream.QueueSteering, req.Message, req.Contents)
		OK(c, gin.H{
			"session_id":   req.SessionID,
			"status":       "queued",
			"message":      "steering queued",
			"queue_kind":   queued.Kind,
			"queue":        stream.QueueSnapshot(),
			"queued_count": stream.QueuedCount(),
		})
	}
}

// StreamQueueHandler 返回运行中任务的未消费队列快照。

func (rt *Runtime) StreamQueueHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")
		if !validateSessionID(sessionID) {
			Fail(c, http.StatusBadRequest, "invalid session ID")
			return
		}
		stream := rt.Streams.Get(sessionID)
		queue := rt.queueForSessionResponse(sessionID, stream)
		OK(c, gin.H{
			"session_id":   sessionID,
			"queue":        queue,
			"queued_count": len(queue),
		})
	}
}

// StreamQueueUpdateHandler 修改尚未执行的队列项。

func (rt *Runtime) StreamQueueUpdateHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")
		queueID := c.Param("queueID")
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
		stream, ok := rt.editQueue(c, sessionID)
		if !ok {
			return
		}
		updated, ok := stream.UpdateQueuedMessage(queueID, queued.Text, queued.Parts, queued.DisplayText)
		if !ok {
			Fail(c, http.StatusNotFound, "queued message not found")
			return
		}
		publishQueueUpdated(stream, sessionID)
		rt.persistQueueSnapshot(sessionID, stream)
		OK(c, gin.H{
			"session_id": sessionID,
			"queue_item": updated,
			"queue":      stream.QueueSnapshot(),
		})
	}
}

// StreamQueueDeleteHandler 删除尚未执行的队列项。

func (rt *Runtime) StreamQueueDeleteHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")
		queueID := c.Param("queueID")
		stream, editing := rt.editQueue(c, sessionID)
		if !editing {
			return
		}
		removed, ok := stream.RemoveQueuedMessage(queueID)
		if !ok {
			Fail(c, http.StatusNotFound, "queued message not found")
			return
		}
		publishQueueUpdated(stream, sessionID)
		rt.persistQueueSnapshot(sessionID, stream)
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

func (rt *Runtime) StreamQueueKindHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")
		queueID := c.Param("queueID")
		stream, editing := rt.editQueue(c, sessionID)
		if !editing {
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
		rt.persistQueueSnapshot(sessionID, stream)
		OK(c, gin.H{
			"session_id": sessionID,
			"queue_item": updated,
			"queue":      stream.QueueSnapshot(),
		})
	}
}

// StreamQueueMoveHandler 调整尚未执行队列项在同类队列中的顺序。

func (rt *Runtime) StreamQueueMoveHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")
		queueID := c.Param("queueID")
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
		stream, editing := rt.editQueue(c, sessionID)
		if !editing {
			return
		}
		moved, ok := stream.MoveQueuedMessage(queueID, direction)
		if !ok {
			Fail(c, http.StatusNotFound, "queued message not found")
			return
		}
		publishQueueUpdated(stream, sessionID)
		rt.persistQueueSnapshot(sessionID, stream)
		OK(c, gin.H{
			"session_id": sessionID,
			"queue_item": moved,
			"queue":      stream.QueueSnapshot(),
		})
	}
}

func (rt *Runtime) streamForQueueRequest(c *gin.Context, sessionID string) *taskstream.Stream {
	if !validateSessionID(sessionID) {
		Fail(c, http.StatusBadRequest, "invalid session ID")
		return nil
	}
	stream := rt.Streams.Get(sessionID)
	if stream == nil || stream.Status() != "processing" {
		Fail(c, http.StatusNotFound, "no running task for this session")
		return nil
	}
	return stream
}

// runStreamTask 后台执行流式任务
func (rt *Runtime) runStreamTask(ctx context.Context, stream *taskstream.Stream, sessionID string, r runtimeport.Runner, recorder *eventlog.HistoryRecorder, turnInput domainmessage.TurnInput, userDisplayText string, manager appstate.MemoryManager, initialRunID string) {
	ctx = rt.withExecutionDependencies(ctx)
	defer stream.Done()

	interruptHandler := buildStreamInterruptHandler(stream, recorder, sessionID)
	currentRunID := initialRunID
	if currentRunID == "" {
		currentRunID = newTurnRunID(sessionID)
	}
	steeringSource := buildSteeringSource(stream, recorder, sessionID, func() string { return currentRunID }, func() {
		rt.persistQueueSnapshot(sessionID, stream)
	})
	currentInput := turnInput
	currentDisplayText := userDisplayText
	chatService := appchat.NewService()
	for {
		_, runErr := chatService.RunTurn(ctx, appchat.TurnRequest{
			SessionID:        sessionID,
			RunID:            currentRunID,
			Runner:           r,
			Input:            currentInput,
			Summary:          recorder,
			InterruptHandler: runtimeport.InterruptHandler(interruptHandler),
			NonInteractive:   true,
			ApprovalRegistry: configuredApprovalRegistry(),
			AskHandler:       buildMemberAskRuntimeHandler(stream, recorder, sessionID),
			SteeringSource:   steeringSource,
			EventSink: func(event events.Event) error {
				if stream.Status() == "cancelled" {
					return context.Canceled
				}
				recorder.RecordEvent(event)
				stream.Publish(standardEventPayload(sessionID, event, rt.ToolDisplays))
				return nil
			},
		})
		if runErr != nil {
			if isConnectionClosed(ctx, runErr) {
				log.Printf("stream task cancelled: session=%s", sessionID)
				if stream.Status() != "cancelled" {
					stream.SetStatus("cancelled")
					stream.Publish(cancelledEventPayload(sessionID, currentRunID, "任务已取消"))
				}
				rt.finishCancelledChat(recorder, sessionID, currentDisplayText)
				return
			}
			log.Printf("stream task error: session=%s, err=%v", sessionID, runErr)
			stream.SetStatus("error")
			stream.Publish(errorEventPayload(sessionID, runErr.Error()))
			rt.finishErrorChat(recorder, sessionID, currentDisplayText, runErr)
			return
		}

		recorder.FinalizeCurrent()
		rt.saveTurnHistory(recorder, sessionID)
		if queued, ok := stream.DequeueNextMessage(); ok {
			publishQueueUpdated(stream, sessionID)
			rt.persistQueueSnapshot(sessionID, stream)
			currentDisplayText = queued.DisplayText
			currentInput = buildQueuedChatInput(recorder, queued, manager)
			currentRunID = queuedTurnRunID(sessionID, queued)
			rt.updateSessionTitleAndStatus(sessionID, currentDisplayText, "processing")
			publishQueuedExecutionStart(stream, sessionID, queued, currentRunID)
			continue
		}

		stream.SetStatus("completed")
		stream.Publish(processingEndEventPayload(sessionID, currentRunID, "处理完成"))
		rt.finishChat(recorder, sessionID, currentDisplayText, manager)
		return
	}
}

// StreamStopHandler 停止正在运行的流式任务

func (rt *Runtime) StreamStopHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")
		if !validateSessionID(sessionID) {
			Fail(c, http.StatusBadRequest, "invalid session ID")
			return
		}

		stream := rt.Streams.Get(sessionID)
		if stream == nil {
			Fail(c, http.StatusNotFound, "当前会话没有正在执行的任务")
			return
		}
		if stream.Status() != "processing" {
			Fail(c, http.StatusConflict, "任务已结束，无法取消")
			return
		}

		stream.Cancel()
		stream.SetStatus("cancelled")
		runID, _ := stream.CurrentTurn()
		stream.Publish(cancelledEventPayload(sessionID, runID, "任务已取消"))
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

func (rt *Runtime) StreamSubscribeHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")
		if !validateSessionID(sessionID) {
			Fail(c, http.StatusBadRequest, "invalid session ID")
			return
		}

		stream := rt.Streams.Get(sessionID)
		if stream == nil {
			Fail(c, http.StatusNotFound, "no active task for this session")
			return
		}

		clearSSEWriteDeadline(c.Writer)

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
		authToken := RequestAuthToken(c)
		authTicker := time.NewTicker(30 * time.Second)
		defer authTicker.Stop()

		writeSSE := func(event taskstream.IndexedEvent) bool {
			data, _ := json.Marshal(event.Data)
			_, err := fmt.Fprintf(flusher, "id: %d\ndata: %s\n\n", event.ID, data)
			flusher.Flush()
			return err == nil
		}
		writeDone := func() {
			_, _ = fmt.Fprint(flusher, "data: [DONE]\n\n")
			flusher.Flush()
		}

		if stream.IsDone() && offset == 0 {
			writeDone()
			return
		}

		for {
			if !streamSubscriptionAuthorized(authToken) {
				return
			}
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
					if !writeSSE(e) {
						return
					}
				}
				writeDone()
				return
			}

			select {
			case <-ctx.Done():
				return
			case <-notify:
			case <-authTicker.C:
			}
		}
	}
}

func streamSubscriptionAuthorized(token string) bool {
	authEnabled, err := AuthEnabled()
	if err != nil {
		return false
	}
	return !authEnabled || ValidateToken(token)
}

func clearSSEWriteDeadline(w http.ResponseWriter) {
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		log.Printf("[stream] clear SSE write deadline failed: %v", err)
	}
}

// StreamStatusHandler 获取流式任务状态。

// StreamSnapshotHandler 返回运行中任务的轻量快照，供新连接先同步缓存再追实时事件。

func (rt *Runtime) StreamSnapshotHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")
		if !validateSessionID(sessionID) {
			Fail(c, http.StatusBadRequest, "invalid session ID")
			return
		}

		stream := rt.Streams.Get(sessionID)
		if stream == nil {
			Fail(c, http.StatusNotFound, "no active task for this session")
			return
		}

		eventCount := stream.EventCount()
		limit := parseSnapshotLimit(c.Query("limit"))
		offset, hasOffset := parseOptionalOffset(c.Query("offset"))
		if !hasOffset {
			offset = snapshotTailOffset(eventCount, limit)
		}

		events := stream.EventsSince(offset)
		if len(events) > limit {
			events = events[:limit]
		}

		items := make([]taskstream.Event, 0, len(events))
		nextOffset := uint64(eventCount)
		if len(events) > 0 {
			nextOffset = events[len(events)-1].ID + 1
			for _, event := range events {
				items = append(items, event.Data)
			}
		}

		result := gin.H{
			"session_id":      sessionID,
			"status":          stream.Status(),
			"has_task":        true,
			"mode":            stream.Mode(),
			"event_count":     eventCount,
			"next_offset":     nextOffset,
			"more_available":  nextOffset < uint64(eventCount),
			"queue":           stream.QueueSnapshot(),
			"events":          items,
			"snapshot_offset": offset,
			"limit":           limit,
			"created_at":      stream.CreatedAt(),
		}
		if doneAt := stream.DoneAt(); !doneAt.IsZero() {
			result["finished_at"] = doneAt
		}
		if stream.AgentName() != "" {
			result["agent_name"] = stream.AgentName()
		}
		OK(c, result)
	}
}

func (rt *Runtime) StreamStatusHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")
		if !validateSessionID(sessionID) {
			Fail(c, http.StatusBadRequest, "invalid session ID")
			return
		}

		stream := rt.Streams.Get(sessionID)
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
		sessionDir := rt.sessionDirPath(sessionID)
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

func parseSnapshotLimit(raw string) int {
	const (
		defaultLimit = 300
		maxLimit     = 1000
	)
	if raw == "" {
		return defaultLimit
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return defaultLimit
	}
	if parsed > maxLimit {
		return maxLimit
	}
	return parsed
}

func parseOptionalOffset(raw string) (uint64, bool) {
	if raw == "" {
		return 0, false
	}
	parsed, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func snapshotTailOffset(eventCount, limit int) uint64 {
	if eventCount <= limit {
		return 0
	}
	return uint64(eventCount - limit)
}

// StreamApprovalHandler 提交 HITL 审批决定

func (rt *Runtime) StreamApprovalHandler() gin.HandlerFunc {
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

		stream := rt.Streams.Get(req.SessionID)
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

func (rt *Runtime) StreamAskResponseHandler() gin.HandlerFunc {
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

		stream := rt.Streams.Get(req.SessionID)
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

func (rt *Runtime) StreamEventsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")
		if !validateSessionID(sessionID) {
			Fail(c, http.StatusBadRequest, "invalid session ID")
			return
		}

		stream := rt.Streams.Get(sessionID)
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
func buildStreamInterruptHandler(stream *taskstream.Stream, recorder *eventlog.HistoryRecorder, sessionID string) runtimeport.InterruptHandler {
	channelHandler := appchat.ChannelInterruptHandler(stream.InterruptCh())
	return func(ctx context.Context, interrupts []runtimeport.Interrupt) (runtimeport.InterruptDecisions, error) {
		// 检查是否为 ask_questions 中断
		if askInterrupt := extractAskInterrupt(interrupts); askInterrupt != nil {
			stream.BeginInterruptWithID(taskstream.InterruptAsk, askInterrupt.ID)
			defer stream.CompleteInterrupt(taskstream.InterruptAsk)

			info := askInterrupt.Info
			askID := askInterrupt.ID
			memberEvent := askInterrupt.Event
			askEvent := askRequestedEvent(memberEvent, askID, info)
			askEvent = events.NormalizeEvent(askEvent)
			recorder.RecordEvent(askEvent)
			stream.Publish(standardEventPayload(sessionID, askEvent, nil))

			result, err := appchat.ChannelTargetInterruptHandler(stream.InterruptCh(), askID)(ctx, interrupts)
			if err == nil {
				answerEvent := askAnsweredEvent(memberEvent, askID, askResponseFromResult(askID, result), askResponseText(result))
				answerEvent = events.NormalizeEvent(answerEvent)
				recorder.RecordEvent(answerEvent)
			}
			return result, err
		}

		// 默认审批流程
		msg := extractInterruptMessage(interrupts)

		stream.BeginInterrupt(taskstream.InterruptApproval)
		defer stream.CompleteInterrupt(taskstream.InterruptApproval)

		runID, turnID := stream.CurrentTurn()
		approvalEvent := events.NormalizeEvent(events.Event{
			Type:    events.EventApprovalRequested,
			RunID:   runID,
			TurnID:  turnID,
			Content: msg,
			Approval: &events.ApprovalPayload{
				Message: msg,
			},
		})
		recorder.RecordEvent(approvalEvent)
		payload := standardEventPayload(sessionID, approvalEvent, nil)
		payload["message"] = msg
		stream.Publish(payload)

		result, err := channelHandler(ctx, interrupts)
		if err == nil {
			if text := approvalDecisionText(result); text != "" {
				recorder.RecordEvent(events.NormalizeEvent(events.Event{
					Type:    events.EventApprovalAnswered,
					RunID:   runID,
					TurnID:  turnID,
					Content: text,
					Approval: &events.ApprovalPayload{
						Decision: text,
					},
				}))
			}
		}

		return result, err
	}
}
