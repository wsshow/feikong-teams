package handler

import (
	"context"
	"encoding/json"
	"fkteams/fkevent"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
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
	Type      string          `json:"type"`
	SessionID string          `json:"session_id,omitempty"`
	Message   string          `json:"message,omitempty"`
	Mode      string          `json:"mode,omitempty"`
	AgentName string          `json:"agent_name,omitempty"`
	Decision  int             `json:"decision,omitempty"`
	Contents  []WSContentPart `json:"contents,omitempty"`
}

// WSContentPart 多模态内容部分
type WSContentPart struct {
	Type       string `json:"type"`                  // text, image_url, image_base64, audio_url, video_url, file_url
	Text       string `json:"text,omitempty"`        // type=text 时的文本内容
	URL        string `json:"url,omitempty"`         // type=image_url/audio_url/video_url/file_url 时的 URL
	Base64Data string `json:"base64_data,omitempty"` // type=image_base64 时的 Base64 数据
	MIMEType   string `json:"mime_type,omitempty"`   // type=image_base64 时的 MIME 类型
	Detail     string `json:"detail,omitempty"`      // type=image_url 时的精度: high/low/auto
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
				go handleChatMessage(connCtx, tm, wsMsg, writeJSON)

			case "cancel":
				tm.mu.Lock()
				if tm.taskCancel != nil {
					tm.taskCancel()
					tm.taskCancel = nil
				}
				tm.mu.Unlock()
				_ = writeJSON(map[string]any{"type": "cancelled", "message": "任务已取消"})

			case "approval":
				tm.mu.Lock()
				ch := tm.approvalCh
				tm.mu.Unlock()
				if ch != nil {
					select {
					case ch <- wsMsg.Decision:
					default:
					}
				}

			case "clear_history":
				handleClearHistory(wsMsg, writeJSON)

			case "load_history":
				handleLoadHistory(wsMsg, writeJSON)

			case "ping":
				_ = writeJSON(map[string]any{"type": "pong"})

			default:
				_ = writeJSON(map[string]any{"type": "error", "error": "unknown message type"})
			}
		}
	}
}

// sessionMemoryOnly 特殊 sessionID 标记，仅清空内存历史
const sessionMemoryOnly = "__memory_only__"

// handleClearHistory 清除指定会话的历史文件
func handleClearHistory(wsMsg WSMessage, writeJSON func(any) error) {
	sessionID := wsMsg.SessionID
	if sessionID == "" {
		sessionID = "default"
	}

	if sessionID == sessionMemoryOnly {
		fkevent.GlobalSessionManager.ClearAll()
		log.Println("[SessionManager] cleared all in-memory session histories (new session)")
		_ = writeJSON(map[string]any{"type": "history_cleared", "message": "内存历史已清除"})
		return
	}

	filePath := chatHistoryPath(sessionID)

	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		log.Printf("failed to delete history file: %v", err)
		_ = writeJSON(map[string]any{"type": "error", "error": "清除历史失败"})
	} else {
		fkevent.GlobalSessionManager.Remove(sessionID)
		log.Printf("[SessionManager] cleared session history: session=%s", sessionID)
		_ = writeJSON(map[string]any{"type": "history_cleared", "message": "历史记录已清除"})
	}
}

// handleLoadHistory 加载指定的历史文件
func handleLoadHistory(wsMsg WSMessage, writeJSON func(any) error) {
	filename := wsMsg.Message
	if !validateFilename(filename) {
		_ = writeJSON(map[string]any{"type": "error", "error": "无效的文件名"})
		return
	}

	filePath := filepath.Join(historyDir, filename)
	sessionID := extractSessionID(filename)

	recorder, err := fkevent.GlobalSessionManager.LoadForSession(sessionID, filePath)
	if err != nil {
		log.Printf("failed to load history file: %v", err)
		_ = writeJSON(map[string]any{"type": "error", "error": fmt.Sprintf("加载历史失败: %v", err)})
		return
	}

	log.Printf("[SessionManager] loaded history file: %s (session=%s)", filename, sessionID)
	_ = writeJSON(map[string]any{
		"type":       "history_loaded",
		"message":    "历史记录已加载",
		"filename":   filename,
		"session_id": sessionID,
		"messages":   recorder.GetMessages(),
	})
}
