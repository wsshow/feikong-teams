package handler

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"fkteams/internal/adapters/storage/file/history"
	"fkteams/internal/adapters/transport/http/origin"
	"fkteams/internal/app/appstate"
	appchat "fkteams/internal/app/chat"
	"fkteams/internal/app/chat/taskstream"
	"fkteams/internal/app/tools/approval"
	"fkteams/internal/app/tools/ask"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/events"
	"fkteams/internal/runtime/turn"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:     origin.IsAllowed,
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

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
func WebSocketHandler() gin.HandlerFunc {
	return WebSocketHandlerWithState(nil)
}

// WebSocketHandlerWithState 处理 WebSocket 连接并使用显式应用状态。
func WebSocketHandlerWithState(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Printf("websocket upgrade failed: %v", err)
			return
		}

		connCtx, connCancel := context.WithCancel(c.Request.Context())
		registerConn(conn, connCancel)
		sm := getSessionManager(conn)

		defer func() {
			connCancel()
			removeSessionManager(conn)
			unregisterConn(conn)
			_ = conn.Close()
		}()

		// 监听 context 取消，主动关闭连接
		go func() {
			<-connCtx.Done()
			_ = conn.Close()
		}()

		// 提取 WS 连接时的 token，用于每条消息的二次校验
		wsToken := c.Query("token")
		if wsToken == "" {
			if cookie, err := c.Cookie("fk_token"); err == nil {
				wsToken = cookie
			}
		}
		authEnabled, _ := AuthEnabled()

		// 线程安全的写入
		var writeMu sync.Mutex
		writeJSON := func(v any) error {
			writeMu.Lock()
			defer writeMu.Unlock()
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

			// chat/steer 消息二次校验 token，防止 WS 长时间连接后 token 过期仍可操作
			if (wsMsg.Type == "chat" || wsMsg.Type == "follow_up" || wsMsg.Type == "steer" || wsMsg.Type == "steering") && authEnabled && !ValidateToken(wsToken) {
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
				go handleChatMessage(sm, wsMsg, writeJSON, state)

			case "steer", "steering":
				handleSteeringMessage(wsMsg, writeJSON)

			case "resume":
				sid := wsMsg.SessionID
				if sid == "" {
					_ = writeJSON(errorEventPayload("", "session_id is required"))
					continue
				}
				stream := GlobalStreams.Get(sid)
				if stream != nil {
					ok, subID := stream.Subscribe(taskstream.FuncSubscriber(writeJSON), wsMsg.Offset)
					if ok {
						// 成功重新绑定并回放事件
						sm.mu.Lock()
						sm.tasks[sid] = &sessionTask{cancel: stream.Cancel, stream: stream, subID: subID}
						sm.mu.Unlock()
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
				if sid != "" {
					GlobalStreams.CancelAndRemove(sid)
					sm.cancelTask(sid)
				}
				_ = writeJSON(map[string]any{"type": "cancellation_requested", "session_id": sid, "message": "取消请求已发送"})

			case "approval":
				sid := wsMsg.SessionID
				if stream := GlobalStreams.Get(sid); stream != nil {
					_ = stream.SubmitInterrupt(taskstream.InterruptApproval, wsMsg.Decision)
				}

			case "ask_response":
				sid := wsMsg.SessionID
				resp := &ask.AskResponse{
					AskID:    wsMsg.AskID,
					Selected: wsMsg.AskSelected,
					FreeText: wsMsg.AskFreeText,
				}
				if stream := GlobalStreams.Get(sid); stream != nil {
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
func buildInterruptHandler(recorder *eventlog.HistoryRecorder, sessionID string, writeJSON func(any) error, stream *taskstream.Stream) turn.InterruptHandler {
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
			payload := attachMemberPayload(map[string]any{
				"type":         events.NotifyAskQuestions,
				"session_id":   sessionID,
				"ask_id":       askID,
				"question":     info.Question,
				"options":      info.Options,
				"multi_select": info.MultiSelect,
			}, memberEvent)
			_ = writeJSON(payload)

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
		_ = writeJSON(map[string]any{
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

// --- WebSocket 事件回调 ---

// wsEventCallbackBuffered 构建支持断线缓冲的事件回调
func wsEventCallbackBuffered(recorder *eventlog.HistoryRecorder, sessionID string, stream *taskstream.Stream) func(events.Event) error {
	return func(event events.Event) error {
		if event.Type == events.EventAction && event.ActionType == events.ActionInterrupted {
			return nil
		}
		recorder.RecordEvent(event)
		data := convertEventToMap(event)
		data["session_id"] = sessionID
		stream.Publish(data)
		return nil
	}
}

// --- WebSocket 聊天处理 ---

// handleChatMessage 处理 WebSocket 聊天消息
func handleChatMessage(sm *sessionManager, wsMsg WSMessage, writeJSON func(any) error, state *appstate.State) {
	sessionID := wsMsg.SessionID
	mode := wsMsg.Mode
	if mode == "" {
		mode = "team"
	}
	if wsMsg.Message == "" && len(wsMsg.Contents) == 0 {
		_ = writeJSON(errorEventPayload(sessionID, "message or contents is required"))
		return
	}

	if existing := GlobalStreams.Get(sessionID); existing != nil && existing.Status() == "processing" {
		enqueueTaskMessage(existing, sessionID, taskstream.QueueFollowUp, wsMsg.Message, wsMsg.Contents)
		return
	}

	// 任务 context 独立于连接——断连不会自动取消任务
	taskCtx, taskCancel := context.WithCancel(appstate.WithState(context.Background(), state))
	defer taskCancel()

	// 注册到统一 TaskStream（支持断线重连 + Push/Pull 消费）
	stream := GlobalStreams.Register(taskstream.StreamConfig{
		SessionID:  sessionID,
		Cancel:     taskCancel,
		CleanupTTL: 5 * time.Minute,
	})
	// 绑定当前 WS 连接为 Push 订阅者
	_, subID := stream.Subscribe(taskstream.FuncSubscriber(writeJSON), 0)
	defer func() {
		stream.Done()
	}()

	// 同时在连接级 sessionManager 中注册（用于取消和断线 Unsubscribe）
	taskID := sm.startTask(sessionID, taskCancel)
	sm.mu.Lock()
	if t, exists := sm.tasks[sessionID]; exists && t.id == taskID {
		t.stream = stream
		t.subID = subID
	}
	sm.mu.Unlock()
	defer sm.removeTask(sessionID, taskID)

	// 获取 runner
	r, err := resolveRunner(taskCtx, mode, wsMsg.AgentName)
	if err != nil {
		log.Printf("failed to resolve runner: session=%s, err=%v", sessionID, err)
		stream.Publish(errorEventPayload(sessionID, err.Error()))
		return
	}

	// 构建输入消息
	recorder := eventlog.GlobalSessionManager.GetOrCreate(sessionID, historyDir)
	manager := memoryFromState(state)
	turnInput, userDisplayText := buildChatInput(recorder, wsMsg.Message, wsMsg.Contents, manager)
	currentRunID := newTurnRunID(sessionID)
	stream.Publish(attachContentParts(attachTurnMeta(map[string]any{
		"type":       events.NotifyUserMessage,
		"session_id": sessionID,
		"content":    userDisplayText,
	}, currentRunID), messageContentParts(turnInput.Message)))

	publishFn := func(v any) error { stream.Publish(v.(map[string]any)); return nil }
	interruptHandler := buildInterruptHandler(recorder, sessionID, publishFn, stream)
	steeringSource := buildSteeringSource(stream, recorder, sessionID, func() string { return currentRunID })
	currentInput := turnInput
	currentDisplayText := userDisplayText
	updateSessionTitleAndStatus(sessionID, currentDisplayText, "processing")
	stream.Publish(attachTurnMeta(map[string]any{
		"type":       events.NotifyProcessingStart,
		"session_id": sessionID,
		"message":    "开始处理您的请求...",
	}, currentRunID))

	chatService := appchat.NewService()
	for {
		_, runErr := chatService.RunTurn(taskCtx, appchat.TurnRequest{
			SessionID: sessionID,
			Runner:    r,
			Input:     currentInput,
		},
			appchat.WithRunID(currentRunID),
			appchat.OnEvent(wsEventCallbackBuffered(recorder, sessionID, stream)),
			appchat.WithHistory(recorder),
			appchat.OnInterrupt(runtimeport.InterruptHandler(interruptHandler)),
			appchat.NonInteractive(),
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
			if isConnectionClosed(taskCtx, runErr) {
				log.Printf("task cancelled: session=%s", sessionID)
				stream.SetStatus("cancelled")
				stream.Publish(map[string]any{
					"type":       events.NotifyCancelled,
					"session_id": sessionID,
					"message":    "任务已取消",
				})
				finishCancelledChat(recorder, sessionID, currentDisplayText)
				return
			}
			log.Printf("failed to run task: session=%s, err=%v", sessionID, runErr)
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
		extractChatMemory(manager, recorder, sessionID)
		return
	}
}

func handleSteeringMessage(wsMsg WSMessage, writeJSON func(any) error) {
	sessionID := wsMsg.SessionID
	if sessionID == "" {
		_ = writeJSON(errorEventPayload("", "session_id is required"))
		return
	}
	if wsMsg.Message == "" && len(wsMsg.Contents) == 0 {
		_ = writeJSON(errorEventPayload(sessionID, "message or contents is required"))
		return
	}
	stream := GlobalStreams.Get(sessionID)
	if stream == nil || stream.Status() != "processing" {
		_ = writeJSON(errorEventPayload(sessionID, "no running task to steer"))
		return
	}
	enqueueTaskMessage(stream, sessionID, taskstream.QueueSteering, wsMsg.Message, wsMsg.Contents)
}
