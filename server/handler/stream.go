package handler

import (
	"context"
	"encoding/json"
	"fkteams/agents/middlewares/summary"
	"fkteams/engine"
	"fkteams/fkevent"
	"fkteams/server/handler/taskstream"
	"fkteams/tools/approval"
	"fkteams/tools/ask"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ==================== API Handlers ====================

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

		// 如果已有运行中的任务，拒绝重复启动
		if existing := GlobalStreams.Get(sessionID); existing != nil && existing.Status() == "processing" {
			Fail(c, http.StatusConflict, "session has a running task, stop it first")
			return
		}

		mode := req.Mode
		if mode == "" {
			mode = "supervisor"
		}

		ctx := context.Background()
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

		recorder := fkevent.GlobalSessionManager.GetOrCreate(sessionID, historyDir)
		inputMessages, userDisplayText := buildChatInput(recorder, req.Message, req.Contents)
		countBeforeRun := recorder.GetMessageCount()
		recorder.RecordUserInput(userDisplayText)

		// 创建任务——统一使用 GlobalStreams
		taskCtx, taskCancel := context.WithCancel(ctx)
		stream := GlobalStreams.Register(taskstream.StreamConfig{
			SessionID:   sessionID,
			Cancel:      taskCancel,
			GracePeriod: 0,
			CleanupTTL:  5 * time.Minute,
			Mode:        mode,
			AgentName:   req.AgentName,
		})

		updateSessionTitleAndStatus(sessionID, userDisplayText, "processing")

		stream.Publish(map[string]any{
			"type":       "processing_start",
			"session_id": sessionID,
			"message":    "开始处理您的请求...",
		})

		// 后台执行任务
		go runStreamTask(taskCtx, stream, sessionID, r, recorder, inputMessages, countBeforeRun, userDisplayText)

		OK(c, gin.H{
			"session_id": sessionID,
			"status":     "processing",
			"message":    "task started",
		})
	}
}

// runStreamTask 后台执行流式任务
func runStreamTask(ctx context.Context, stream *taskstream.Stream, sessionID string, r *adk.Runner, recorder *fkevent.HistoryRecorder, inputMessages []adk.Message, countBeforeRun int, userDisplayText string) {
	defer stream.Done()

	ctx = fkevent.WithNonInteractive(ctx)
	ctx = fkevent.WithCallback(ctx, func(event fkevent.Event) error {
		if event.Type == "action" && event.ActionType == "interrupted" {
			return nil
		}
		recorder.RecordEvent(event)
		data := convertEventToMap(event)
		data["session_id"] = sessionID
		stream.Publish(data)
		return nil
	})
	ctx = summary.WithSummaryPersistCallback(ctx, func(summaryText string) {
		recorder.SetSummary(summaryText, countBeforeRun)
	})
	ctx = approval.WithRegistry(ctx, approval.NewRegistry(
		approval.StoreConfig{Name: approval.StoreCommand},
		approval.StoreConfig{Name: approval.StoreFile, Matcher: approval.DirMatchFunc},
		approval.StoreConfig{Name: approval.StoreDispatch},
	))

	interruptHandler := buildStreamInterruptHandler(stream, recorder, sessionID)
	_, err := engine.New(r, sessionID).Run(ctx, inputMessages, engine.WithInterruptHandler(interruptHandler))

	if err != nil {
		if isConnectionClosed(ctx, err) {
			log.Printf("stream task cancelled: session=%s", sessionID)
			stream.SetStatus("cancelled")
			stream.Publish(map[string]any{
				"type":       "cancelled",
				"session_id": sessionID,
				"message":    "任务已取消",
			})
			saveHistory(recorder, chatHistoryPath(sessionID), sessionID)
			ensureSessionMetadata(sessionID, userDisplayText)
			return
		}
		log.Printf("stream task error: session=%s, err=%v", sessionID, err)
		stream.SetStatus("error")
		stream.Publish(map[string]any{
			"type":       "error",
			"session_id": sessionID,
			"error":      err.Error(),
		})
		finishChat(recorder, sessionID, userDisplayText)
		return
	}

	stream.SetStatus("completed")
	finishChat(recorder, sessionID, userDisplayText)
	stream.Publish(map[string]any{
		"type":       "processing_end",
		"session_id": sessionID,
		"message":    "处理完成",
	})
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
		meta, err := fkevent.LoadMetadata(sessionDir)
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

		select {
		case stream.InterruptCh() <- req.Decision:
			OK(c, gin.H{"message": "approval submitted"})
		default:
			Fail(c, http.StatusConflict, "no pending approval request")
		}
	}
}

// StreamAskResponseHandler 提交 ask_questions 回答
func StreamAskResponseHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			SessionID string   `json:"session_id" binding:"required"`
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
			Selected: req.Selected,
			FreeText: req.FreeText,
		}
		select {
		case stream.InterruptCh() <- resp:
			OK(c, gin.H{"message": "response submitted"})
		default:
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
func buildStreamInterruptHandler(stream *taskstream.Stream, recorder *fkevent.HistoryRecorder, sessionID string) engine.InterruptHandler {
	channelHandler := engine.ChannelHandler(stream.InterruptCh())
	return func(ctx context.Context, interrupts []*adk.InterruptCtx) (map[string]any, error) {
		// 检查是否为 ask_questions 中断
		if info := extractAskInfo(interrupts); info != nil {
			recorder.RecordEvent(fkevent.Event{
				Type:       "action",
				ActionType: "ask_questions",
				Content:    info.Question,
			})
			stream.Publish(map[string]any{
				"type":         "ask_questions",
				"session_id":   sessionID,
				"question":     info.Question,
				"options":      info.Options,
				"multi_select": info.MultiSelect,
			})

			result, err := channelHandler(ctx, interrupts)
			if err == nil {
				recorder.RecordEvent(fkevent.Event{
					Type:       "action",
					ActionType: "ask_response",
					Content:    askResponseText(result),
				})
			}
			return result, err
		}

		// 默认审批流程
		msg := extractInterruptMessage(interrupts)

		recorder.RecordEvent(fkevent.Event{
			Type:       "action",
			ActionType: "approval_required",
			Content:    msg,
		})
		stream.Publish(map[string]any{
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
