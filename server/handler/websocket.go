package handler

import (
	"context"
	"encoding/json"
	"fkteams/fkevent"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

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
	Type      string   `json:"type"`
	SessionID string   `json:"session_id,omitempty"`
	Message   string   `json:"message,omitempty"`
	Mode      string   `json:"mode,omitempty"`
	AgentName string   `json:"agent_name,omitempty"`
	FilePaths []string `json:"file_paths,omitempty"`
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
		tm := getTaskManager(conn)

		defer func() {
			connCancel()
			removeTaskManager(conn)
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
		writeJSON := func(v interface{}) error {
			writeMu.Lock()
			defer writeMu.Unlock()
			return conn.WriteJSON(v)
		}

		_ = writeJSON(map[string]interface{}{
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
				_ = writeJSON(map[string]interface{}{"type": "error", "error": "invalid message format"})
				continue
			}

			switch wsMsg.Type {
			case "chat":
				go handleChatMessage(connCtx, tm, wsMsg, writeJSON)

			case "cancel":
				tm.mu.Lock()
				if tm.taskCancel != nil {
					tm.taskCancel()
					tm.taskCancel = nil
				}
				tm.mu.Unlock()
				_ = writeJSON(map[string]interface{}{"type": "cancelled", "message": "任务已取消"})

			case "clear_history":
				handleClearHistory(wsMsg, writeJSON)

			case "load_history":
				handleLoadHistory(wsMsg, writeJSON)

			case "ping":
				_ = writeJSON(map[string]interface{}{"type": "pong"})

			default:
				_ = writeJSON(map[string]interface{}{"type": "error", "error": "unknown message type"})
			}
		}
	}
}

// handleClearHistory 清除指定会话的历史文件
func handleClearHistory(wsMsg WSMessage, writeJSON func(interface{}) error) {
	sessionID := wsMsg.SessionID
	if sessionID == "" {
		sessionID = "default"
	}
	filePath := fmt.Sprintf("%sfkteams_chat_history_%s", historyDir, sessionID)

	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		log.Printf("failed to delete history file: %v", err)
		_ = writeJSON(map[string]interface{}{"type": "error", "error": "清除历史失败"})
	} else {
		log.Printf("cleared session history: session=%s", sessionID)
		_ = writeJSON(map[string]interface{}{"type": "history_cleared", "message": "历史记录已清除"})
	}
}

// handleLoadHistory 加载指定的历史文件
func handleLoadHistory(wsMsg WSMessage, writeJSON func(interface{}) error) {
	filename := wsMsg.Message
	if filename == "" {
		_ = writeJSON(map[string]interface{}{"type": "error", "error": "文件名不能为空"})
		return
	}
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") {
		_ = writeJSON(map[string]interface{}{"type": "error", "error": "无效的文件名"})
		return
	}

	filePath := fmt.Sprintf("%s%s", historyDir, filename)
	if err := fkevent.GlobalHistoryRecorder.LoadFromFile(filePath); err != nil {
		log.Printf("failed to load history file: %v", err)
		_ = writeJSON(map[string]interface{}{"type": "error", "error": fmt.Sprintf("加载历史失败: %v", err)})
		return
	}

	sessionID := extractSessionID(filename)
	log.Printf("loaded history file: %s (session=%s)", filename, sessionID)
	_ = writeJSON(map[string]interface{}{
		"type":       "history_loaded",
		"message":    "历史记录已加载",
		"filename":   filename,
		"session_id": sessionID,
		"messages":   fkevent.GlobalHistoryRecorder.GetMessages(),
	})
}
