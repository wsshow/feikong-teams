package handler

import (
	"context"
	"sync"
	"time"

	"fkteams/internal/app/chat/taskstream"

	"github.com/gorilla/websocket"
)

// WebSocketHub 管理单个 HTTP runtime 的 WebSocket 连接和连接级任务引用。
type WebSocketHub struct {
	streams *taskstream.Manager

	wsConnsMu sync.Mutex
	wsConns   map[*websocket.Conn]context.CancelFunc

	sessionManagersMu sync.Mutex
	sessionManagers   map[*websocket.Conn]*sessionManager
}

func NewWebSocketHub(streams *taskstream.Manager) *WebSocketHub {
	return &WebSocketHub{
		streams:         streams,
		wsConns:         make(map[*websocket.Conn]context.CancelFunc),
		sessionManagers: make(map[*websocket.Conn]*sessionManager),
	}
}

// sessionTask 单个会话的任务状态
type sessionTask struct {
	cancel context.CancelFunc
	stream *taskstream.Stream // 关联的 TaskStream（用于 Unsubscribe）
	subID  taskstream.SubscriptionID
	id     uint64 // 唯一标识，用于区分同一 session 的新旧任务
}

// sessionManager 管理一个 WebSocket 连接上的所有并发会话任务
type sessionManager struct {
	mu     sync.Mutex
	tasks  map[string]*sessionTask // key: sessionID
	nextID uint64
}

// startTask 注册一个新的会话任务。如果该 session 已有运行中的任务则先取消旧任务。
// 返回任务 ID，用于 removeTask 时识别是否是自己注册的任务。
func (sm *sessionManager) startTask(sessionID string, cancel context.CancelFunc) uint64 {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if old, exists := sm.tasks[sessionID]; exists && old.cancel != nil {
		old.cancel()
	}
	sm.nextID++
	id := sm.nextID
	sm.tasks[sessionID] = &sessionTask{cancel: cancel, id: id}
	return id
}

// cancelTask 取消指定会话的任务
func (sm *sessionManager) cancelTask(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if t, exists := sm.tasks[sessionID]; exists && t.cancel != nil {
		t.cancel()
	}
}

// removeTask 移除已完成的会话任务，仅当 taskID 匹配时才删除（防止误删新任务）
func (sm *sessionManager) removeTask(sessionID string, taskID uint64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if t, exists := sm.tasks[sessionID]; exists && t.id == taskID {
		delete(sm.tasks, sessionID)
	}
}

// detachAll 分离所有运行中的任务（连接断开时调用，不取消任务）
func (sm *sessionManager) detachAll(streams *taskstream.Manager) {
	sm.mu.Lock()
	items := make([]taskstream.UnsubscribeItem, 0, len(sm.tasks))
	for _, t := range sm.tasks {
		if t.stream != nil {
			items = append(items, taskstream.UnsubscribeItem{Stream: t.stream, ID: t.subID})
		}
	}
	sm.tasks = make(map[string]*sessionTask) // 清空连接级任务引用
	sm.mu.Unlock()
	streams.UnsubscribeAll(items)
}

func (hub *WebSocketHub) registerConn(conn *websocket.Conn, cancel context.CancelFunc) {
	hub.wsConnsMu.Lock()
	hub.wsConns[conn] = cancel
	hub.wsConnsMu.Unlock()
}

func (hub *WebSocketHub) unregisterConn(conn *websocket.Conn) {
	hub.wsConnsMu.Lock()
	delete(hub.wsConns, conn)
	hub.wsConnsMu.Unlock()
}

func (hub *WebSocketHub) getSessionManager(conn *websocket.Conn) *sessionManager {
	hub.sessionManagersMu.Lock()
	defer hub.sessionManagersMu.Unlock()
	if sm, exists := hub.sessionManagers[conn]; exists {
		return sm
	}
	sm := &sessionManager{tasks: make(map[string]*sessionTask)}
	hub.sessionManagers[conn] = sm
	return sm
}

func (hub *WebSocketHub) removeSessionManager(conn *websocket.Conn) {
	hub.sessionManagersMu.Lock()
	defer hub.sessionManagersMu.Unlock()
	if sm, exists := hub.sessionManagers[conn]; exists {
		sm.detachAll(hub.streams) // 分离任务而非取消，允许重连后恢复
		delete(hub.sessionManagers, conn)
	}
}

// CloseAllWebSockets 服务退出时调用，主动关闭所有 WS 连接并取消所有任务。
func (hub *WebSocketHub) CloseAllWebSockets() {
	// 先取消所有活跃任务（context.Background 创建的任务不会被连接关闭自动取消）
	hub.streams.CancelAll()

	hub.wsConnsMu.Lock()
	conns := make(map[*websocket.Conn]context.CancelFunc, len(hub.wsConns))
	for c, cancel := range hub.wsConns {
		conns[c] = cancel
	}
	hub.wsConnsMu.Unlock()

	for conn, cancel := range conns {
		cancel()
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutting down"),
			time.Now().Add(500*time.Millisecond),
		)
		_ = conn.Close()
	}
}
