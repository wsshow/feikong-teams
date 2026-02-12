package handler

import (
	"context"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WS 连接池管理
var (
	wsConnsMu sync.Mutex
	wsConns   = make(map[*websocket.Conn]context.CancelFunc)
)

// taskManager 任务取消管理（每个连接一个）
type taskManager struct {
	mu         sync.Mutex
	taskCancel context.CancelFunc
}

var (
	taskManagersMu sync.Mutex
	taskManagers   = make(map[*websocket.Conn]*taskManager)
)

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

func getTaskManager(conn *websocket.Conn) *taskManager {
	taskManagersMu.Lock()
	defer taskManagersMu.Unlock()
	if tm, exists := taskManagers[conn]; exists {
		return tm
	}
	tm := &taskManager{}
	taskManagers[conn] = tm
	return tm
}

func removeTaskManager(conn *websocket.Conn) {
	taskManagersMu.Lock()
	defer taskManagersMu.Unlock()
	if tm, exists := taskManagers[conn]; exists {
		tm.mu.Lock()
		if tm.taskCancel != nil {
			tm.taskCancel()
		}
		tm.mu.Unlock()
		delete(taskManagers, conn)
	}
}

// CloseAllWebSockets 服务退出时调用，主动关闭所有 WS 连接
func CloseAllWebSockets() {
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
