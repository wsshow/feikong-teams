package handler

import (
	"context"
	"encoding/json"
	"fkteams/internal/runtime/log"
	"sync"
	"time"

	"fkteams/internal/adapters/storage/file/history"
	"fkteams/internal/adapters/transport/http/origin"
	"fkteams/internal/app/agent/catalog/toolmeta"
	"fkteams/internal/app/appstate"
	appchat "fkteams/internal/app/chat"
	"fkteams/internal/app/chat/taskstream"
	"fkteams/internal/app/tools/ask"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/events"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:     origin.IsAllowed,
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

const (
	webSocketWriteTimeout = 15 * time.Second
	webSocketReadLimit    = 32 << 20
)

// WSMessage WebSocket 消息类型
type WSMessage struct {
	Type        string        `json:"type"`
	SessionID   string        `json:"session_id,omitempty"`
	Offset      uint64        `json:"offset,omitempty"`
	Message     string        `json:"message,omitempty"`
	Mode        string        `json:"mode,omitempty"`
	AgentName   string        `json:"agent_name,omitempty"`
	Decision    int           `json:"decision,omitempty"`
	Contents    []ContentPart `json:"contents,omitempty"`
	AskID       string        `json:"ask_id,omitempty"`
	AskSelected []string      `json:"ask_selected,omitempty"`
	AskFreeText string        `json:"ask_free_text,omitempty"`
}

// WebSocketHandler 处理 WebSocket 连接

// WebSocketHandlerWithState 处理 WebSocket 连接并使用显式应用状态。

// WebSocketHandlerWithState 处理当前 HTTP runtime 的 WebSocket 连接。
func (rt *Runtime) WebSocketHandlerWithState(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Printf("websocket upgrade failed: %v", err)
			return
		}
		conn.SetReadLimit(webSocketReadLimit)

		connCtx, connCancel := context.WithCancel(c.Request.Context())
		rt.Connections.registerConn(conn, connCancel)
		sm := rt.Connections.getSessionManager(conn)

		defer func() {
			connCancel()
			rt.Connections.removeSessionManager(conn)
			rt.Connections.unregisterConn(conn)
			_ = conn.Close()
		}()

		// 监听 context 取消，主动关闭连接
		go func() {
			<-connCtx.Done()
			_ = conn.Close()
		}()

		// 提取 WS 连接时的 token，用于每条消息的二次校验
		wsToken := RequestAuthToken(c)

		// 线程安全的写入
		var writeMu sync.Mutex
		writeJSON := func(v any) error {
			writeMu.Lock()
			defer writeMu.Unlock()
			if err := conn.SetWriteDeadline(time.Now().Add(webSocketWriteTimeout)); err != nil {
				return err
			}
			return conn.WriteJSON(v)
		}

		_ = writeJSON(map[string]any{
			"type":    events.NotifyConnected,
			"message": "欢迎连接到非空小队",
		})

		for {
			_, msgBytes, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("websocket read error: %v", err)
				}
				break
			}

			var wsMsg WSMessage
			if err := json.Unmarshal(msgBytes, &wsMsg); err != nil {
				_ = writeJSON(errorEventPayload("", "invalid message format"))
				continue
			}

			// 每条消息二次校验 Token，防止长连接绕过认证热更新或过期限制。
			authEnabled, authErr := AuthEnabled()
			if authErr != nil || authEnabled && !ValidateToken(wsToken) {
				_ = writeJSON(errorEventPayload("", "登录已过期，请重新登录"))
				conn.Close()
				break
			}

			switch wsMsg.Type {
			case "chat", "follow_up":
				if wsMsg.SessionID == "" {
					_ = writeJSON(errorEventPayload("", "session_id is required"))
					continue
				}
				if !rt.Go(func() { rt.handleChatMessage(sm, wsMsg, writeJSON, state) }) {
					_ = writeJSON(errorEventPayload(wsMsg.SessionID, "HTTP runtime is shutting down"))
				}

			case "steer", "steering":
				rt.handleSteeringMessage(wsMsg, writeJSON)

			case "resume":
				sid := wsMsg.SessionID
				if sid == "" {
					_ = writeJSON(errorEventPayload("", "session_id is required"))
					continue
				}
				stream := rt.Streams.Get(sid)
				if stream != nil {
					ok, subID := stream.Subscribe(taskstream.FuncSubscriber(func(event taskstream.Event) error {
						return writeJSON(event)
					}), wsMsg.Offset)
					if ok {
						// 成功重新绑定并回放事件
						sm.attachSubscription(sid, stream, subID)
						log.Printf("task resumed: session=%s", sid)
					} else {
						_ = writeJSON(map[string]any{
							"type":       events.NotifyProcessingEnd,
							"session_id": sid,
							"message":    "任务已完成或不存在",
						})
					}
				} else {
					_ = writeJSON(map[string]any{
						"type":       events.NotifyProcessingEnd,
						"session_id": sid,
						"message":    "任务已完成或不存在",
					})
				}

			case "cancel":
				sid := wsMsg.SessionID
				if !validateSessionID(sid) {
					_ = writeJSON(errorEventPayload(sid, "invalid session ID"))
					continue
				}
				stream := rt.Streams.Get(sid)
				if stream == nil || !stream.CancelIfProcessing() {
					_ = writeJSON(errorEventPayload(sid, "no running task for this session"))
					continue
				}
				runID, _ := stream.CurrentTurn()
				stream.Publish(cancelledEventPayload(sid, runID, "任务已取消"))
				_ = writeJSON(map[string]any{"type": "cancellation_requested", "session_id": sid, "message": "取消请求已发送"})

			case "approval":
				sid := wsMsg.SessionID
				if stream := rt.Streams.Get(sid); stream != nil {
					_ = stream.SubmitInterrupt(taskstream.InterruptApproval, wsMsg.Decision)
				}

			case "ask_answered":
				sid := wsMsg.SessionID
				resp := &ask.AskResponse{
					AskID:    wsMsg.AskID,
					Selected: wsMsg.AskSelected,
					FreeText: wsMsg.AskFreeText,
				}
				if stream := rt.Streams.Get(sid); stream != nil {
					_ = stream.SubmitAskResponse(wsMsg.AskID, resp)
				}

			case "ping":
				_ = writeJSON(map[string]any{"type": events.NotifyPong})

			default:
				_ = writeJSON(errorEventPayload("", "unknown message type"))
			}
		}
	}
}

// --- WebSocket HITL 中断处理器 ---

// buildInterruptHandler 构建 WebSocket 聊天的 HITL 中断处理器
func buildInterruptHandler(recorder *eventlog.HistoryRecorder, sessionID string, publish func(taskstream.Event) error, stream *taskstream.Stream) runtimeport.InterruptHandler {
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
			_ = publish(standardEventPayload(sessionID, askEvent, nil))

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
		_ = publish(payload)

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

// --- WebSocket 事件回调 ---

// wsEventCallbackBuffered 构建支持断线缓冲的事件发布回调。
func wsEventCallbackBuffered(sessionID string, stream *taskstream.Stream, resolver toolmeta.Resolver) func(events.Event) error {
	return func(event events.Event) error {
		data := convertEventToMapWithResolver(event, resolver)
		data["session_id"] = sessionID
		stream.Publish(taskstream.Event(data))
		return nil
	}
}

// --- WebSocket 聊天处理 ---

// handleChatMessage 处理 WebSocket 聊天消息
func (rt *Runtime) handleChatMessage(sm *sessionManager, wsMsg WSMessage, writeJSON func(any) error, state *appstate.State) {
	sessionID := wsMsg.SessionID
	mode := wsMsg.Mode
	if mode == "" {
		mode = "team"
	}
	if wsMsg.Message == "" && len(wsMsg.Contents) == 0 {
		_ = writeJSON(errorEventPayload(sessionID, "message or contents is required"))
		return
	}
	if !validateSessionID(sessionID) {
		_ = writeJSON(errorEventPayload(sessionID, "invalid session ID"))
		return
	}
	unlockSession := rt.lockSessionOperation(sessionID)
	defer unlockSession()

	if existing := rt.Streams.Get(sessionID); existing != nil && existing.Status() == "processing" {
		if _, ok := rt.enqueueTaskMessage(existing, sessionID, taskstream.QueueFollowUp, wsMsg.Message, wsMsg.Contents); ok {
			return
		}
	}

	// 任务 context 独立于连接——断连不会自动取消任务
	taskCtx, taskCancel := context.WithCancel(appstate.WithState(context.Background(), state))
	defer taskCancel()
	taskCtx = rt.withExecutionDependencies(taskCtx)

	// 注册到统一 TaskStream（支持断线重连 + Push/Pull 消费）
	stream, created := rt.Streams.RegisterIfIdle(taskstream.StreamConfig{
		SessionID:  sessionID,
		Cancel:     taskCancel,
		CleanupTTL: 5 * time.Minute,
	})
	if !created {
		if _, ok := rt.enqueueTaskMessage(stream, sessionID, taskstream.QueueFollowUp, wsMsg.Message, wsMsg.Contents); !ok {
			_ = writeJSON(errorEventPayload(sessionID, "task is finishing; retry the request"))
		}
		return
	}
	unlockSession()
	rt.restorePersistentQueue(sessionID, stream)
	// 绑定当前 WS 连接为 Push 订阅者
	_, subID := stream.Subscribe(taskstream.FuncSubscriber(func(event taskstream.Event) error {
		return writeJSON(event)
	}), 0)
	defer func() {
		stream.Done()
	}()

	// 同时在连接级 sessionManager 中注册（用于取消和断线 Unsubscribe）
	taskID := sm.startTask(sessionID, taskCancel, stream, subID)
	defer sm.removeTask(sessionID, taskID)

	// 获取 runner
	r, err := rt.resolveRunner(taskCtx, mode, wsMsg.AgentName)
	if err != nil {
		log.Printf("failed to resolve runner: session=%s, err=%v", sessionID, err)
		stream.SetStatus("error")
		stream.Publish(errorEventPayload(sessionID, err.Error()))
		return
	}

	// 构建输入消息
	recorder, releaseRecorder := rt.acquireRecorder(sessionID)
	defer releaseRecorder()
	manager := memoryFromState(state)
	turnInput, userDisplayText := buildChatInput(recorder, wsMsg.Message, wsMsg.Contents, manager)
	currentRunID := newTurnRunID(sessionID)
	currentTurnID := turnIDForRun(currentRunID)
	stream.SetTurn(currentRunID, currentTurnID)

	publishFn := func(event taskstream.Event) error {
		stream.Publish(event)
		return nil
	}
	interruptHandler := buildInterruptHandler(recorder, sessionID, publishFn, stream)
	steeringSource := buildSteeringSource(stream, recorder, sessionID, func() string { return currentRunID }, func() {
		rt.persistQueueSnapshot(sessionID, stream)
	})
	currentInput := turnInput
	currentDisplayText := userDisplayText
	rt.updateSessionTitleAndStatus(sessionID, currentDisplayText, "processing")
	stream.Publish(standardMessageEventPayload(sessionID, currentRunID, currentTurnID, "开始处理您的请求..."))

	chatService := appchat.NewService()
	for {
		_, runErr := chatService.RunTurn(taskCtx, appchat.TurnRequest{
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
					return nil
				}
				recorder.RecordEvent(event)
				return wsEventCallbackBuffered(sessionID, stream, rt.ToolDisplays)(event)
			},
		})
		if runErr != nil {
			if isConnectionClosed(taskCtx, runErr) {
				log.Printf("task cancelled: session=%s", sessionID)
				if stream.Status() != "cancelled" {
					stream.SetStatus("cancelled")
					stream.Publish(cancelledEventPayload(sessionID, currentRunID, "任务已取消"))
				}
				rt.finishCancelledChat(recorder, sessionID, currentDisplayText)
				return
			}
			log.Printf("failed to run task: session=%s, err=%v", sessionID, runErr)
			stream.SetStatus("error")
			stream.Publish(errorEventPayload(sessionID, runErr.Error()))
			rt.finishErrorChat(recorder, sessionID, currentDisplayText, runErr)
			return
		}

		recorder.FinalizeCurrent()
		rt.saveTurnHistory(recorder, sessionID)
		if queued, ok := stream.DequeueNextMessageOrComplete(); ok {
			publishQueueUpdated(stream, sessionID)
			rt.persistQueueSnapshot(sessionID, stream)
			currentDisplayText = queued.DisplayText
			currentInput = buildQueuedChatInput(recorder, queued, manager)
			currentRunID = queuedTurnRunID(sessionID, queued)
			rt.updateSessionTitleAndStatus(sessionID, currentDisplayText, "processing")
			publishQueuedExecutionStart(stream, sessionID, queued, currentRunID)
			continue
		}

		if stream.Status() != "completed" {
			if stream.Status() == "cancelled" {
				rt.finishCancelledChat(recorder, sessionID, currentDisplayText)
			}
			return
		}
		stream.Publish(processingEndEventPayload(sessionID, currentRunID, "处理完成"))
		rt.finishChat(recorder, sessionID, currentDisplayText, manager)
		return
	}
}

func (rt *Runtime) handleSteeringMessage(wsMsg WSMessage, writeJSON func(any) error) {
	sessionID := wsMsg.SessionID
	if sessionID == "" {
		_ = writeJSON(errorEventPayload("", "session_id is required"))
		return
	}
	if wsMsg.Message == "" && len(wsMsg.Contents) == 0 {
		_ = writeJSON(errorEventPayload(sessionID, "message or contents is required"))
		return
	}
	stream := rt.Streams.Get(sessionID)
	if stream == nil || stream.Status() != "processing" {
		_ = writeJSON(errorEventPayload(sessionID, "no running task to steer"))
		return
	}
	if _, ok := rt.enqueueTaskMessage(stream, sessionID, taskstream.QueueSteering, wsMsg.Message, wsMsg.Contents); !ok {
		_ = writeJSON(errorEventPayload(sessionID, "task is finishing; steering was not queued"))
	}
}
