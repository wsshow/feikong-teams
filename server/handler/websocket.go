package handler

import (
	"context"
	"encoding/json"
	"fkteams/agents/cmder"
	"fkteams/agents/coder"
	"fkteams/agents/custom"
	"fkteams/agents/discussant"
	"fkteams/agents/leader"
	"fkteams/agents/moderator"
	"fkteams/agents/searcher"
	"fkteams/agents/storyteller"
	"fkteams/agents/visitor"
	"fkteams/common"
	"fkteams/config"
	"fkteams/fkevent"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/supervisor"
	"github.com/cloudwego/eino/schema"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// WS 连接池管理
var (
	wsConnsMu sync.Mutex
	wsConns   = make(map[*websocket.Conn]context.CancelFunc)
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

// CloseAllWebSockets 服务退出时调用，主动关闭所有 WS 连接
func CloseAllWebSockets() {
	wsConnsMu.Lock()
	conns := make(map[*websocket.Conn]context.CancelFunc, len(wsConns))
	for c, cancel := range wsConns {
		conns[c] = cancel
	}
	wsConnsMu.Unlock()

	for conn, cancel := range conns {
		cancel() // 取消该连接关联的所有任务
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutting down"),
			time.Now().Add(500*time.Millisecond),
		)
		_ = conn.Close()
	}
}

// WebSocket 消息类型
type WSMessage struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id,omitempty"`
	Message   string `json:"message,omitempty"`
	Mode      string `json:"mode,omitempty"` // "supervisor" 或 "roundtable" 或 "custom"
}

// WebSocketHandler 处理 WebSocket 连接
func WebSocketHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Printf("WebSocket 升级失败: %v", err)
			return
		}

		// 为该连接创建可取消的 context
		connCtx, connCancel := context.WithCancel(c.Request.Context())
		registerConn(conn, connCancel)

		defer func() {
			connCancel()
			unregisterConn(conn)
			_ = conn.Close()
		}()

		// 监听 context 取消，主动关闭连接以打断 ReadMessage 阻塞
		go func() {
			<-connCtx.Done()
			_ = conn.Close()
		}()

		// 用于线程安全的写入
		var writeMu sync.Mutex
		writeJSON := func(v interface{}) error {
			writeMu.Lock()
			defer writeMu.Unlock()
			return conn.WriteJSON(v)
		}

		// 发送欢迎消息
		_ = writeJSON(map[string]interface{}{
			"type":    "connected",
			"message": "欢迎连接到非空小队",
		})

		for {
			// 读取客户端消息（连接被 Close 后会立刻返回错误，从而退出循环）
			_, msgBytes, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("WebSocket 读取错误: %v", err)
				}
				break
			}

			var wsMsg WSMessage
			if err := json.Unmarshal(msgBytes, &wsMsg); err != nil {
				_ = writeJSON(map[string]interface{}{
					"type":  "error",
					"error": "消息格式错误",
				})
				continue
			}

			// 处理不同类型的消息
			switch wsMsg.Type {
			case "chat":
				// 传入 connCtx，服务退出时可取消任务
				go handleChatMessage(connCtx, wsMsg, writeJSON)
			case "ping":
				_ = writeJSON(map[string]interface{}{
					"type": "pong",
				})
			default:
				_ = writeJSON(map[string]interface{}{
					"type":  "error",
					"error": "未知消息类型",
				})
			}
		}
	}
}

// handleChatMessage 处理聊天消息
func handleChatMessage(ctx context.Context, wsMsg WSMessage, writeJSON func(interface{}) error) {
	sessionID := wsMsg.SessionID
	if sessionID == "" {
		sessionID = "default"
	}

	input := wsMsg.Message
	mode := wsMsg.Mode
	if mode == "" {
		mode = "supervisor"
	}

	// 检查是否已取消
	select {
	case <-ctx.Done():
		return
	default:
	}

	// 准备输入消息
	var inputMessages []adk.Message
	historyFilePath := fmt.Sprintf("./history/chat_history/fkteams_chat_history_%s", sessionID)
	err := fkevent.GlobalHistoryRecorder.LoadFromFile(historyFilePath)
	if err == nil {
		log.Printf("自动加载聊天历史: [%s]", sessionID)
	}

	agentMessages := fkevent.GlobalHistoryRecorder.GetMessages()
	if len(agentMessages) > 0 {
		var historyMessage strings.Builder
		for _, agentMessage := range agentMessages {
			fmt.Fprintf(&historyMessage, "%s: %s\n", agentMessage.AgentName, agentMessage.Content)
		}
		inputMessages = append(inputMessages, schema.SystemMessage(fmt.Sprintf("以下是之前的对话历史:\n---\n%s\n---\n", historyMessage.String())))
	}
	inputMessages = append(inputMessages, schema.UserMessage(input))
	fkevent.GlobalHistoryRecorder.RecordUserInput(input)

	// 根据模式选择 runner（使用传入的 ctx 而非 context.Background()）
	var runner *adk.Runner
	switch mode {
	case "roundtable":
		runner = loopAgentModeWS(ctx)
	case "custom":
		runner = customSupervisorModeWS(ctx)
	default:
		runner = supervisorModeWS(ctx)
	}

	// 设置回调函数，通过 WebSocket 发送事件
	fkevent.Callback = func(event fkevent.Event) error {
		// 转换事件为前端可用的格式
		wsEvent := convertEventForWS(event)
		fkevent.GlobalHistoryRecorder.RecordEvent(event)
		return writeJSON(wsEvent)
	}

	// 发送开始处理的消息
	writeJSON(map[string]interface{}{
		"type":    "processing_start",
		"message": "开始处理您的请求...",
	})

	iter := runner.Run(ctx, inputMessages, adk.WithCheckPointID("fkteams"))
	for {
		// 每次迭代检查 ctx 是否已取消
		select {
		case <-ctx.Done():
			log.Printf("任务被取消: session=%s", sessionID)
			return
		default:
		}

		event, ok := iter.Next()
		if !ok {
			break
		}
		if err := fkevent.ProcessAgentEvent(ctx, event); err != nil {
			log.Printf("Error processing event: %v", err)
			_ = writeJSON(map[string]interface{}{
				"type":  "error",
				"error": err.Error(),
			})
			break
		}
	}

	// 再次检查，避免取消后还保存
	select {
	case <-ctx.Done():
		return
	default:
	}

	// 保存聊天历史
	log.Printf("任务完成，正在自动保存聊天历史到 %s ...", historyFilePath)
	err = fkevent.GlobalHistoryRecorder.SaveToFile(historyFilePath)
	if err != nil {
		log.Printf("保存聊天历史失败: %v", err)
	} else {
		log.Printf("成功保存聊天历史到文件: %s", historyFilePath)
	}

	// 发送完成消息
	_ = writeJSON(map[string]interface{}{
		"type":    "processing_end",
		"message": "处理完成",
	})
}

// convertEventForWS 将事件转换为前端可用的格式
func convertEventForWS(event fkevent.Event) map[string]interface{} {
	result := map[string]interface{}{
		"type":       event.Type,
		"agent_name": event.AgentName,
	}

	if event.RunPath != "" {
		result["run_path"] = event.RunPath
	}

	if event.Content != "" {
		result["content"] = event.Content
	}

	if len(event.ToolCalls) > 0 {
		toolCalls := make([]map[string]interface{}, 0, len(event.ToolCalls))
		for _, tc := range event.ToolCalls {
			toolCall := map[string]interface{}{
				"name": tc.Function.Name,
			}
			if tc.Function.Arguments != "" {
				toolCall["arguments"] = tc.Function.Arguments
			}
			toolCalls = append(toolCalls, toolCall)
		}
		result["tool_calls"] = toolCalls
	}

	if event.ActionType != "" {
		result["action_type"] = event.ActionType
	}

	if event.Error != "" {
		result["error"] = event.Error
	}

	return result
}

// supervisorModeWS WebSocket 版本的 supervisor 模式
func supervisorModeWS(ctx context.Context) *adk.Runner {
	err := godotenv.Load()
	if err != nil {
		log.Println("Error loading .env file")
	}

	leaderAgent := leader.NewAgent()
	storytellerAgent := storyteller.NewAgent()
	searcherAgent := searcher.NewAgent()
	subAgents := []adk.Agent{searcherAgent, storytellerAgent}

	if os.Getenv("FEIKONG_CODER_ENABLED") == "true" {
		coderAgent := coder.NewAgent()
		subAgents = append(subAgents, coderAgent)
	}

	if os.Getenv("FEIKONG_CMDER_ENABLED") == "true" {
		cmderAgent := cmder.NewAgent()
		subAgents = append(subAgents, cmderAgent)
	}

	if os.Getenv("FEIKONG_SSH_VISITOR_ENABLED") == "true" {
		visitorAgent := visitor.NewAgent()
		defer visitor.CloseSSHClient()
		subAgents = append(subAgents, visitorAgent)
	}

	supervisorAgent, err := supervisor.New(ctx, &supervisor.Config{
		Supervisor: leaderAgent,
		SubAgents:  subAgents,
	})
	if err != nil {
		log.Fatal(err)
	}

	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           supervisorAgent,
		EnableStreaming: true,
		CheckPointStore: common.NewInMemoryStore(),
	})

	return runner
}

// loopAgentModeWS WebSocket 版本的 loop agent 模式
func loopAgentModeWS(ctx context.Context) *adk.Runner {
	teamConfig, err := config.Get()
	if err != nil {
		log.Fatal(err)
	}

	var subAgents []adk.Agent
	for _, member := range teamConfig.Roundtable.Members {
		agent := discussant.NewAgent(member)
		subAgents = append(subAgents, agent)
	}

	loopAgent, err := adk.NewLoopAgent(ctx, &adk.LoopAgentConfig{
		Name:          "Roundtable",
		Description:   "多智能体共同讨论并解决问题",
		SubAgents:     subAgents,
		MaxIterations: teamConfig.Roundtable.MaxIterations,
	})
	if err != nil {
		log.Fatal(err)
	}

	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           loopAgent,
		EnableStreaming: true,
		CheckPointStore: common.NewInMemoryStore(),
	})

	return runner
}

func customSupervisorModeWS(ctx context.Context) *adk.Runner {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	cfg, err := config.Get()
	if err != nil {
		log.Fatal(err)
	}

	moderatorAgent := moderator.NewAgent()
	storytellerAgent := storyteller.NewAgent()
	searcherAgent := searcher.NewAgent()
	subAgents := []adk.Agent{searcherAgent, storytellerAgent}

	for _, customAgent := range cfg.Custom.Agents {
		subAgents = append(subAgents, custom.NewAgent(custom.Config{
			Name:         customAgent.Name,
			Description:  customAgent.Description,
			SystemPrompt: customAgent.SystemPrompt,
			Model: custom.Model{
				Name:    customAgent.ModelName,
				APIKey:  customAgent.APIKey,
				BaseURL: customAgent.BaseURL,
			},
		}))
	}

	supervisorAgent, err := supervisor.New(ctx, &supervisor.Config{
		Supervisor: moderatorAgent,
		SubAgents:  subAgents,
	})
	if err != nil {
		log.Fatal(err)
	}

	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           supervisorAgent,
		EnableStreaming: true,
		CheckPointStore: common.NewInMemoryStore(),
	})

	return runner
}
