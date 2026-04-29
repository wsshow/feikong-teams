package handler

import (
	"context"
	"encoding/json"
	"fkteams/agents/middlewares/summary"
	"fkteams/common"
	"fkteams/engine"
	"fkteams/fkevent"
	"fkteams/server/handler/taskstream"
	"fkteams/tools/approval"
	"fkteams/tools/ask"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// WSMessage WebSocket 消息类型
type WSMessage struct {
	Type        string        `json:"type"`
	SessionID   string        `json:"session_id,omitempty"`
	Message     string        `json:"message,omitempty"`
	Mode        string        `json:"mode,omitempty"`
	AgentName   string        `json:"agent_name,omitempty"`
	Decision    int           `json:"decision,omitempty"`
	Contents    []ContentPart `json:"contents,omitempty"`
	AskSelected []string      `json:"ask_selected,omitempty"`
	AskFreeText string        `json:"ask_free_text,omitempty"`
}

// WebSocketHandler 处理 WebSocket 连接
func WebSocketHandler() gin.HandlerFunc {
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

		// 线程安全的写入
		var writeMu sync.Mutex
		writeJSON := func(v any) error {
			writeMu.Lock()
			defer writeMu.Unlock()
			return conn.WriteJSON(v)
		}

		_ = writeJSON(map[string]any{
			"type":    "connected",
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
				_ = writeJSON(map[string]any{"type": "error", "error": "invalid message format"})
				continue
			}

			switch wsMsg.Type {
			case "chat":
				if wsMsg.SessionID == "" {
					_ = writeJSON(map[string]any{"type": "error", "error": "session_id is required"})
					continue
				}
				go handleChatMessage(sm, wsMsg, writeJSON)

			case "resume":
				sid := wsMsg.SessionID
				if sid == "" {
					_ = writeJSON(map[string]any{"type": "error", "error": "session_id is required"})
					continue
				}
				stream := GlobalStreams.Get(sid)
				if stream != nil {
					ok, epoch := stream.Subscribe(taskstream.FuncSubscriber(writeJSON))
					if ok {
						// 成功重新绑定并回放事件
						sm.mu.Lock()
						sm.tasks[sid] = &sessionTask{cancel: stream.Cancel, stream: stream, subEpoch: epoch}
						sm.mu.Unlock()
						log.Printf("task resumed: session=%s", sid)
					} else {
						_ = writeJSON(map[string]any{
							"type":       "processing_end",
							"session_id": sid,
							"message":    "任务已完成或不存在",
						})
					}
				} else {
					_ = writeJSON(map[string]any{
						"type":       "processing_end",
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
				_ = writeJSON(map[string]any{"type": "cancelled", "session_id": sid, "message": "任务已取消"})

			case "approval":
				sid := wsMsg.SessionID
				if stream := GlobalStreams.Get(sid); stream != nil {
					select {
					case stream.InterruptCh() <- wsMsg.Decision:
					default:
					}
				}

			case "ask_response":
				sid := wsMsg.SessionID
				resp := &ask.AskResponse{
					Selected: wsMsg.AskSelected,
					FreeText: wsMsg.AskFreeText,
				}
				if stream := GlobalStreams.Get(sid); stream != nil {
					select {
					case stream.InterruptCh() <- resp:
					default:
					}
				}

			case "ping":
				_ = writeJSON(map[string]any{"type": "pong"})

			default:
				_ = writeJSON(map[string]any{"type": "error", "error": "unknown message type"})
			}
		}
	}
}

// --- WebSocket HITL 中断处理器 ---

// buildInterruptHandler 构建 WebSocket 聊天的 HITL 中断处理器
func buildInterruptHandler(recorder *fkevent.HistoryRecorder, sessionID string, writeJSON func(any) error, approvalCh <-chan any) engine.InterruptHandler {
	channelHandler := engine.ChannelHandler(approvalCh)
	return func(ctx context.Context, interrupts []*adk.InterruptCtx) (map[string]any, error) {
		// 检查是否为 ask_questions 中断
		if info := extractAskInfo(interrupts); info != nil {
			recorder.RecordEvent(fkevent.Event{
				Type:       "action",
				ActionType: "ask_questions",
				Content:    info.Question,
			})
			payload := map[string]any{
				"type":         "ask_questions",
				"session_id":   sessionID,
				"question":     info.Question,
				"options":      info.Options,
				"multi_select": info.MultiSelect,
			}
			_ = writeJSON(payload)

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
		_ = writeJSON(map[string]any{
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

// --- WebSocket 事件回调 ---

// wsEventCallbackBuffered 构建支持断线缓冲的事件回调
func wsEventCallbackBuffered(recorder *fkevent.HistoryRecorder, sessionID string, stream *taskstream.Stream) func(fkevent.Event) error {
	return func(event fkevent.Event) error {
		if event.Type == "action" && event.ActionType == "interrupted" {
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
func handleChatMessage(sm *sessionManager, wsMsg WSMessage, writeJSON func(any) error) {
	sessionID := wsMsg.SessionID
	mode := wsMsg.Mode
	if mode == "" {
		mode = "supervisor"
	}

	// 任务 context 独立于连接——断连不会自动取消任务
	taskCtx, taskCancel := context.WithCancel(context.Background())
	defer taskCancel()

	// 注册到统一 TaskStream（支持断线重连 + Push/Pull 消费）
	stream := GlobalStreams.Register(taskstream.StreamConfig{
		SessionID:   sessionID,
		Cancel:      taskCancel,
		GracePeriod: 60 * time.Second,
		CleanupTTL:  60 * time.Second,
	})
	// 绑定当前 WS 连接为 Push 订阅者
	_, subEpoch := stream.Subscribe(taskstream.FuncSubscriber(writeJSON))
	defer func() {
		stream.Done()
	}()

	// 同时在连接级 sessionManager 中注册（用于取消和断线 Unsubscribe）
	taskID := sm.startTask(sessionID, taskCancel)
	sm.mu.Lock()
	if t, exists := sm.tasks[sessionID]; exists && t.id == taskID {
		t.stream = stream
		t.subEpoch = subEpoch
	}
	sm.mu.Unlock()
	defer sm.removeTask(sessionID, taskID)

	// 获取 runner
	r, err := resolveRunner(taskCtx, mode, wsMsg.AgentName)
	if err != nil {
		log.Printf("failed to resolve runner: session=%s, err=%v", sessionID, err)
		stream.Publish(map[string]any{"type": "error", "session_id": sessionID, "error": err.Error()})
		return
	}

	// 构建输入消息
	recorder := fkevent.GlobalSessionManager.GetOrCreate(sessionID, historyDir)
	inputMessages, userDisplayText := buildChatInput(recorder, wsMsg.Message, wsMsg.Contents)
	countBeforeRun := recorder.GetMessageCount()
	recorder.RecordUserInput(userDisplayText)

	// 装配 context —— 事件回调通过 stream.Publish 发送（支持缓冲）
	taskCtx = fkevent.WithNonInteractive(taskCtx)
	taskCtx = fkevent.WithCallback(taskCtx, wsEventCallbackBuffered(recorder, sessionID, stream))
	taskCtx = summary.WithSummaryPersistCallback(taskCtx, func(summaryText string) {
		recorder.SetSummary(summaryText, countBeforeRun)
	})
	taskCtx = approval.WithRegistry(taskCtx, approval.NewRegistry(
		approval.StoreConfig{Name: approval.StoreCommand},
		approval.StoreConfig{Name: approval.StoreFile, Matcher: approval.DirMatchFunc},
		approval.StoreConfig{Name: approval.StoreDispatch},
	))

	taskCtx = common.WithSessionID(taskCtx, sessionID)

	// 更新会话标题和状态
	updateSessionTitleAndStatus(sessionID, userDisplayText, "processing")

	stream.Publish(map[string]any{
		"type":       "processing_start",
		"session_id": sessionID,
		"message":    "开始处理您的请求...",
	})

	// 执行——中断处理使用 stream 的审批通道
	publishFn := func(v any) error { stream.Publish(v.(map[string]any)); return nil }
	interruptHandler := buildInterruptHandler(recorder, sessionID, publishFn, stream.InterruptCh())
	_, err = engine.New(r, "fkteams").Run(taskCtx, inputMessages, engine.WithInterruptHandler(interruptHandler))
	if err != nil {
		if taskCtx.Err() != nil {
			log.Printf("task cancelled: session=%s", sessionID)
			stream.SetStatus("cancelled")
			saveHistory(recorder, chatHistoryPath(sessionID), sessionID)
			ensureSessionMetadata(sessionID, userDisplayText)
			return
		}
		log.Printf("failed to run task: session=%s, err=%v", sessionID, err)
		stream.SetStatus("error")
		stream.Publish(map[string]any{"type": "error", "session_id": sessionID, "error": err.Error()})
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
