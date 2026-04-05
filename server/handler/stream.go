package handler

import (
	"context"
	"encoding/json"
	"fkteams/agents/middlewares/summary"
	"fkteams/engine"
	"fkteams/fkevent"
	"fkteams/tools/approval"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ==================== StreamBuffer: 会话级事件缓冲 ====================

// StreamEvent 带序号的流式事件
type StreamEvent struct {
	ID   uint64         `json:"id"`
	Data map[string]any `json:"data"`
}

// StreamBuffer 线程安全的事件缓冲区，支持实时订阅与历史回放。
// 每个事件按递增 ID 存储，订阅者可以从任意 offset 开始消费，
// 实现断线重连时的无损数据回放。
type StreamBuffer struct {
	mu      sync.RWMutex
	events  []StreamEvent
	nextID  uint64
	done    bool
	subs    map[uint64]chan struct{}
	nextSub uint64
}

func newStreamBuffer() *StreamBuffer {
	return &StreamBuffer{
		events: make([]StreamEvent, 0, 256),
		subs:   make(map[uint64]chan struct{}),
	}
}

// Append 追加事件并通知所有订阅者
func (b *StreamBuffer) Append(data map[string]any) uint64 {
	b.mu.Lock()
	id := b.nextID
	b.nextID++
	b.events = append(b.events, StreamEvent{ID: id, Data: data})
	for _, ch := range b.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	b.mu.Unlock()
	return id
}

// EventsSince 返回从 offset 开始的所有事件
func (b *StreamBuffer) EventsSince(offset uint64) []StreamEvent {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if offset >= uint64(len(b.events)) {
		return nil
	}
	result := make([]StreamEvent, len(b.events)-int(offset))
	copy(result, b.events[offset:])
	return result
}

// Subscribe 注册订阅，返回通知 channel 和取消函数
func (b *StreamBuffer) Subscribe() (notify <-chan struct{}, unsubscribe func()) {
	ch := make(chan struct{}, 1)
	b.mu.Lock()
	id := b.nextSub
	b.nextSub++
	b.subs[id] = ch
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs, id)
		b.mu.Unlock()
	}
}

// MarkDone 标记任务已完成，通知所有订阅者
func (b *StreamBuffer) MarkDone() {
	b.mu.Lock()
	b.done = true
	for _, ch := range b.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	b.mu.Unlock()
}

// IsDone 检查任务是否已完成
func (b *StreamBuffer) IsDone() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.done
}

// Len 返回已缓冲的事件数量
func (b *StreamBuffer) Len() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return uint64(len(b.events))
}

// ==================== StreamTaskManager: 全局任务管理 ====================

// StreamTaskState 流式任务状态
type StreamTaskState struct {
	SessionID  string
	Mode       string
	AgentName  string
	Status     string // processing, completed, error, cancelled
	Buffer     *StreamBuffer
	Cancel     context.CancelFunc
	ApprovalCh chan int
	CreatedAt  time.Time
	FinishedAt time.Time
}

// streamTaskManager 管理所有流式任务（与 WebSocket 连接解耦）。
// 缓存仅服务于运行中及刚完成的任务，用于实时 SSE 推送和断线重连。
// 已完成任务的数据由历史接口 (/sessions/:id) 提供，缓存过期后自动释放。
type streamTaskManager struct {
	mu    sync.RWMutex
	tasks map[string]*StreamTaskState
}

// 已完成任务在内存中的保留时间，供迟到的订阅者拉取
const streamTaskTTL = 5 * time.Minute

var globalStreamTasks = newStreamTaskManager()

func newStreamTaskManager() *streamTaskManager {
	m := &streamTaskManager{tasks: make(map[string]*StreamTaskState)}
	go m.cleanupLoop()
	return m
}

// cleanupLoop 后台定期清理已过期的已完成任务，释放内存
func (m *streamTaskManager) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		m.cleanup()
	}
}

func (m *streamTaskManager) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for id, t := range m.tasks {
		if t.Status != "processing" && !t.FinishedAt.IsZero() && now.Sub(t.FinishedAt) > streamTaskTTL {
			delete(m.tasks, id)
			log.Printf("[stream] released expired task buffer: session=%s", id)
		}
	}
}

func (m *streamTaskManager) get(sessionID string) *StreamTaskState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tasks[sessionID]
}

func (m *streamTaskManager) set(sessionID string, t *StreamTaskState) {
	m.mu.Lock()
	m.tasks[sessionID] = t
	m.mu.Unlock()
}

func (m *streamTaskManager) remove(sessionID string) {
	m.mu.Lock()
	delete(m.tasks, sessionID)
	m.mu.Unlock()
}

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
		if existing := globalStreamTasks.get(sessionID); existing != nil && existing.Status == "processing" {
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

		// 创建任务状态
		taskCtx, taskCancel := context.WithCancel(ctx)
		buf := newStreamBuffer()
		approvalCh := make(chan int, 1)

		state := &StreamTaskState{
			SessionID:  sessionID,
			Mode:       mode,
			AgentName:  req.AgentName,
			Status:     "processing",
			Buffer:     buf,
			Cancel:     taskCancel,
			ApprovalCh: approvalCh,
			CreatedAt:  time.Now(),
		}
		globalStreamTasks.set(sessionID, state)

		updateSessionTitleAndStatus(sessionID, userDisplayText, "processing")

		buf.Append(map[string]any{
			"type":       "processing_start",
			"session_id": sessionID,
			"message":    "开始处理您的请求...",
		})

		// 后台执行任务
		go runStreamTask(taskCtx, state, r, recorder, inputMessages, countBeforeRun, userDisplayText)

		OK(c, gin.H{
			"session_id": sessionID,
			"status":     "processing",
			"message":    "task started",
		})
	}
}

// runStreamTask 后台执行流式任务
func runStreamTask(ctx context.Context, state *StreamTaskState, r *adk.Runner, recorder *fkevent.HistoryRecorder, inputMessages []adk.Message, countBeforeRun int, userDisplayText string) {
	sessionID := state.SessionID
	buf := state.Buffer

	defer func() {
		state.FinishedAt = time.Now()
		buf.MarkDone()
	}()

	ctx = fkevent.WithNonInteractive(ctx)
	ctx = fkevent.WithCallback(ctx, func(event fkevent.Event) error {
		// interrupted 事件由 interruptHandler 记录为 approval_required 并推送，此处跳过
		if event.Type == "action" && event.ActionType == "interrupted" {
			return nil
		}
		recorder.RecordEvent(event)
		data := convertEventToMap(event)
		data["session_id"] = sessionID
		buf.Append(data)
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

	interruptHandler := buildStreamInterruptHandler(buf, recorder, sessionID, state.ApprovalCh)
	_, err := engine.New(r, "fkteams").Run(ctx, inputMessages, engine.WithInterruptHandler(interruptHandler))

	if err != nil {
		if isConnectionClosed(ctx, err) {
			log.Printf("stream task cancelled: session=%s", sessionID)
			state.Status = "cancelled"
			buf.Append(map[string]any{
				"type":       "cancelled",
				"session_id": sessionID,
				"message":    "任务已取消",
			})
			saveHistory(recorder, chatHistoryPath(sessionID), sessionID)
			ensureSessionMetadata(sessionID, userDisplayText)
			return
		}
		log.Printf("stream task error: session=%s, err=%v", sessionID, err)
		state.Status = "error"
		buf.Append(map[string]any{
			"type":       "error",
			"session_id": sessionID,
			"error":      err.Error(),
		})
		finishChat(recorder, sessionID, userDisplayText)
		return
	}

	state.Status = "completed"
	finishChat(recorder, sessionID, userDisplayText)
	buf.Append(map[string]any{
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

		state := globalStreamTasks.get(sessionID)
		if state == nil {
			Fail(c, http.StatusNotFound, "no task found for this session")
			return
		}
		if state.Status != "processing" {
			Fail(c, http.StatusConflict, fmt.Sprintf("task is not running, current status: %s", state.Status))
			return
		}

		state.Cancel()
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

		state := globalStreamTasks.get(sessionID)
		if state == nil {
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

		buf := state.Buffer
		notify, unsub := buf.Subscribe()
		defer unsub()

		ctx := c.Request.Context()
		flusher := c.Writer

		writeSSE := func(event StreamEvent) bool {
			data, _ := json.Marshal(event.Data)
			_, err := fmt.Fprintf(flusher, "id: %d\ndata: %s\n\n", event.ID, data)
			flusher.Flush()
			return err == nil
		}

		for {
			events := buf.EventsSince(offset)
			for _, e := range events {
				if !writeSSE(e) {
					return
				}
				offset = e.ID + 1
			}

			if buf.IsDone() {
				// 任务已结束，最后再读一次确保不丢失尾部事件
				for _, e := range buf.EventsSince(offset) {
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
// 返回 has_task 标识是否有活跃的流式任务缓存可订阅。
// 前端据此决定是直接加载历史还是接入实时流。
func StreamStatusHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")
		if !validateSessionID(sessionID) {
			Fail(c, http.StatusBadRequest, "invalid session ID")
			return
		}

		state := globalStreamTasks.get(sessionID)
		if state != nil {
			result := gin.H{
				"session_id":  sessionID,
				"status":      state.Status,
				"has_task":    true,
				"mode":        state.Mode,
				"event_count": state.Buffer.Len(),
				"created_at":  state.CreatedAt,
			}
			if !state.FinishedAt.IsZero() {
				result["finished_at"] = state.FinishedAt
			}
			if state.AgentName != "" {
				result["agent_name"] = state.AgentName
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

		state := globalStreamTasks.get(req.SessionID)
		if state == nil || state.Status != "processing" {
			Fail(c, http.StatusNotFound, "no running task for this session")
			return
		}

		select {
		case state.ApprovalCh <- req.Decision:
			OK(c, gin.H{"message": "approval submitted"})
		default:
			Fail(c, http.StatusConflict, "no pending approval request")
		}
	}
}

// StreamEventsHandler 获取当前任务的已缓冲事件（非 SSE，一次性拉取）。
// 仅对内存中有缓存的任务有效；已完成的历史数据应通过 GET /sessions/:id 获取。
func StreamEventsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")
		if !validateSessionID(sessionID) {
			Fail(c, http.StatusBadRequest, "invalid session ID")
			return
		}

		state := globalStreamTasks.get(sessionID)
		if state == nil {
			Fail(c, http.StatusNotFound, "no active task for this session")
			return
		}

		var offset uint64
		if offsetStr := c.Query("offset"); offsetStr != "" {
			if parsed, err := strconv.ParseUint(offsetStr, 10, 64); err == nil {
				offset = parsed
			}
		}

		events := state.Buffer.EventsSince(offset)
		OK(c, gin.H{
			"session_id":  sessionID,
			"status":      state.Status,
			"events":      events,
			"event_count": state.Buffer.Len(),
			"done":        state.Buffer.IsDone(),
		})
	}
}

// ==================== 内部辅助 ====================

// buildStreamInterruptHandler 构建流式任务的 HITL 中断处理器
func buildStreamInterruptHandler(buf *StreamBuffer, recorder *fkevent.HistoryRecorder, sessionID string, approvalCh <-chan int) engine.InterruptHandler {
	channelHandler := engine.ChannelHandler(approvalCh)
	return func(ctx context.Context, interrupts []*adk.InterruptCtx) (map[string]any, error) {
		msg := extractInterruptMessage(interrupts)

		recorder.RecordEvent(fkevent.Event{
			Type:       "action",
			ActionType: "approval_required",
			Content:    msg,
		})
		buf.Append(map[string]any{
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
