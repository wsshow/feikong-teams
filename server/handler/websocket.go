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
	"fkteams/g"
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

// 任务取消管理（每个连接一个）
type taskManager struct {
	mu         sync.Mutex
	taskCancel context.CancelFunc
}

var (
	taskManagersMu sync.Mutex
	taskManagers   = make(map[*websocket.Conn]*taskManager)
)

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

		// 获取该连接的任务管理器
		tm := getTaskManager(conn)

		defer func() {
			connCancel()
			removeTaskManager(conn)
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
				// 传入连接 context 和任务管理器
				go handleChatMessage(connCtx, tm, wsMsg, writeJSON)
			case "cancel":
				// 只取消当前任务，不关闭连接
				tm.mu.Lock()
				if tm.taskCancel != nil {
					tm.taskCancel()
					tm.taskCancel = nil
				}
				tm.mu.Unlock()
				_ = writeJSON(map[string]interface{}{
					"type":    "cancelled",
					"message": "任务已取消",
				})
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
func handleChatMessage(connCtx context.Context, tm *taskManager, wsMsg WSMessage, writeJSON func(interface{}) error) {
	sessionID := wsMsg.SessionID
	if sessionID == "" {
		sessionID = "default"
	}

	input := wsMsg.Message
	mode := wsMsg.Mode
	if mode == "" {
		mode = "supervisor"
	}

	// 为这个任务创建独立的 context
	taskCtx, taskCancel := context.WithCancel(connCtx)
	defer taskCancel()

	// 注册任务取消函数
	tm.mu.Lock()
	tm.taskCancel = taskCancel
	tm.mu.Unlock()

	// 任务结束时清理
	defer func() {
		tm.mu.Lock()
		tm.taskCancel = nil
		tm.mu.Unlock()
	}()

	// 检查是否已取消
	select {
	case <-taskCtx.Done():
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

	// 根据模式选择 runner
	var runner *adk.Runner
	switch mode {
	case "roundtable":
		runner = loopAgentModeWS(taskCtx)
	case "custom":
		runner = customSupervisorModeWS(taskCtx)
	default:
		runner = supervisorModeWS(taskCtx)
	}

	defer func() {
		err = g.Cleaner.ExecuteAndClear()
		if err != nil {
			fmt.Printf("清理资源失败: %v\n", err)
		}
	}()

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

	iter := runner.Run(taskCtx, inputMessages, adk.WithCheckPointID("fkteams"))
	for {
		// 每次迭代检查 taskCtx 是否已取消
		select {
		case <-taskCtx.Done():
			log.Printf("任务被取消: session=%s", sessionID)
			return
		default:
		}

		event, ok := iter.Next()
		if !ok {
			break
		}
		if err := fkevent.ProcessAgentEvent(taskCtx, event); err != nil {
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
	case <-taskCtx.Done():
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
		g.Cleaner.Add(func() error {
			visitor.CloseSSHClient()
			return nil
		})
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

	cfg, err := config.Get()
	if err != nil {
		log.Fatal(err)
	}

	var moderatorAgent adk.Agent
	var subAgents []adk.Agent

	if cfg.Custom.Moderator.Name != "" {
		moderatorAgent = custom.NewAgent(custom.Config{
			Name:         cfg.Custom.Moderator.Name,
			Description:  cfg.Custom.Moderator.Desc,
			SystemPrompt: cfg.Custom.Moderator.SystemPrompt,
			Model: custom.Model{
				Name:    cfg.Custom.Moderator.ModelName,
				APIKey:  cfg.Custom.Moderator.APIKey,
				BaseURL: cfg.Custom.Moderator.BaseURL,
			},
			ToolNames: cfg.Custom.Moderator.Tools,
		})
	} else {
		moderatorAgent = moderator.NewAgent()
	}

	for _, customAgent := range cfg.Custom.Agents {
		subAgents = append(subAgents, custom.NewAgent(custom.Config{
			Name:         customAgent.Name,
			Description:  customAgent.Desc,
			SystemPrompt: customAgent.SystemPrompt,
			Model: custom.Model{
				Name:    customAgent.ModelName,
				APIKey:  customAgent.APIKey,
				BaseURL: customAgent.BaseURL,
			},
			ToolNames: customAgent.Tools,
		}))
	}

	var names []string
	for _, subAgent := range subAgents {
		names = append(names, subAgent.Name(ctx))
	}
	fmt.Println(strings.Join(names, ", "))

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
