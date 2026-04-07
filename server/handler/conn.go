package handler

import (
	"context"
	"fkteams/server/handler/taskstream"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WS 连接池管理
var (
	wsConnsMu sync.Mutex
	wsConns   = make(map[*websocket.Conn]context.CancelFunc)
)

// sessionTask 单个会话的任务状态
type sessionTask struct {
	cancel   context.CancelFunc
	stream   *taskstream.Stream // 关联的 TaskStream（用于 Unsubscribe）
	subEpoch uint64             // Subscribe 时获得的 epoch（用于安全 Unsubscribe）
	id       uint64             // 唯一标识，用于区分同一 session 的新旧任务
}

// sessionManager 管理一个 WebSocket 连接上的所有并发会话任务
type sessionManager struct {
	mu     sync.Mutex
	tasks  map[string]*sessionTask // key: sessionID
	nextID uint64
}

var (
	sessionManagersMu sync.Mutex
	sessionManagers   = make(map[*websocket.Conn]*sessionManager)
)

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
func (sm *sessionManager) detachAll() {
	sm.mu.Lock()
	items := make([]taskstream.UnsubscribeItem, 0, len(sm.tasks))
	for _, t := range sm.tasks {
		if t.stream != nil {
			items = append(items, taskstream.UnsubscribeItem{Stream: t.stream, Epoch: t.subEpoch})
		}
	}
	sm.tasks = make(map[string]*sessionTask) // 清空连接级任务引用
	sm.mu.Unlock()
	// 使用当初 Subscribe 时保存的 epoch，确保不会误解绑新连接的订阅者
	GlobalStreams.UnsubscribeAll(items)
}

func registerConn(conn *websocket.Conn, cancel context.CancelFunc) {
	wsConnsMu.Lock()
	wsConns[conn] = cancel
	wsConnsMu.Unlock()
}

func unregisterConn(conn *websocket.Conn) {
	wsConnsMu.Lock()
	delete(wsConns, conn)
	wsConnsMu.Unlock()
}

func getSessionManager(conn *websocket.Conn) *sessionManager {
	sessionManagersMu.Lock()
	defer sessionManagersMu.Unlock()
	if sm, exists := sessionManagers[conn]; exists {
		return sm
	}
	sm := &sessionManager{tasks: make(map[string]*sessionTask)}
	sessionManagers[conn] = sm
	return sm
}

func removeSessionManager(conn *websocket.Conn) {
	sessionManagersMu.Lock()
	defer sessionManagersMu.Unlock()
	if sm, exists := sessionManagers[conn]; exists {
		sm.detachAll() // 分离任务而非取消，允许重连后恢复
		delete(sessionManagers, conn)
	}
}

// CloseAllWebSockets 服务退出时调用，主动关闭所有 WS 连接并取消所有任务
func CloseAllWebSockets() {
	// 先取消所有活跃任务（context.Background 创建的任务不会被连接关闭自动取消）
	GlobalStreams.CancelAll()

	wsConnsMu.Lock()
	conns := make(map[*websocket.Conn]context.CancelFunc, len(wsConns))
	for c, cancel := range wsConns {
		conns[c] = cancel
	}
	wsConnsMu.Unlock()

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
