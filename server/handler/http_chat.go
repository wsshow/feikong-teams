package handler

import (
	"context"
	"encoding/json"
	"fkteams/agents/middlewares/summary"
	"fkteams/chatutil"
	"fkteams/engine"
	"fkteams/fkevent"
	"fkteams/g"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/gin-gonic/gin"
)

// ChatRequest HTTP 聊天请求
type ChatRequest struct {
	SessionID string          `json:"session_id"`
	Message   string          `json:"message"`
	Mode      string          `json:"mode"`
	AgentName string          `json:"agent_name"`
	FilePaths []string        `json:"file_paths"`
	Stream    bool            `json:"stream"`
	Contents  []WSContentPart `json:"contents"` // 多模态内容
}

// ChatHandler HTTP POST 聊天处理器，支持普通 JSON 响应和 SSE 流式响应
func ChatHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ChatRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
			return
		}

		// 验证输入：纯文本或多模态内容至少提供一个
		if req.Message == "" && len(req.Contents) == 0 {
			Fail(c, http.StatusBadRequest, "message or contents is required")
			return
		}

		sessionID := req.SessionID
		if sessionID == "" {
			sessionID = "default"
		}
		mode := req.Mode
		if mode == "" {
			mode = "supervisor"
		}

		// 获取 runner
		ctx := c.Request.Context()
		var r *adk.Runner
		if req.AgentName != "" {
			r = getOrCreateAgentRunner(ctx, req.AgentName)
			if r == nil {
				Fail(c, http.StatusBadRequest, fmt.Sprintf("agent not found: %s", req.AgentName))
				return
			}
		} else {
			var err error
			r, err = getOrCreateRunner(ctx, mode)
			if err != nil {
				Fail(c, http.StatusInternalServerError, fmt.Sprintf("failed to create runner: %v", err))
				return
			}
		}

		// 获取该会话的 HistoryRecorder
		recorder := fkevent.GlobalSessionManager.GetOrCreate(sessionID, historyDir)

		// 构建输入消息，支持多模态
		var inputMessages []adk.Message
		var userDisplayText string

		if len(req.Contents) > 0 {
			parts := convertWSContentParts(req.Contents)
			userDisplayText = chatutil.ExtractTextFromParts(parts)
			if userDisplayText == "" {
				userDisplayText = req.Message
			}
			inputMessages = chatutil.BuildMultimodalInputMessages(recorder, userDisplayText, parts)
		} else {
			userDisplayText = req.Message
			inputMessages = chatutil.BuildInputMessages(recorder, req.Message)
		}

		countBeforeRun := recorder.GetMessageCount()
		recorder.RecordUserInput(userDisplayText)

		if req.Stream {
			handleStreamChat(c, ctx, r, recorder, inputMessages, countBeforeRun, sessionID)
		} else {
			handleSyncChat(c, ctx, r, recorder, inputMessages, countBeforeRun, sessionID)
		}
	}
}

// handleStreamChat SSE 流式聊天响应
func handleStreamChat(c *gin.Context, ctx context.Context, r *adk.Runner, recorder *fkevent.HistoryRecorder, inputMessages []adk.Message, countBeforeRun int, sessionID string) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	taskCtx, taskCancel := context.WithCancel(ctx)
	defer taskCancel()

	// 绑定事件回调
	taskCtx = fkevent.WithCallback(taskCtx, func(event fkevent.Event) error {
		recorder.RecordEvent(event)
		data, _ := json.Marshal(convertEventForWS(event))
		_, err := fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		c.Writer.Flush()
		return err
	})

	taskCtx = summary.WithSummaryPersistCallback(taskCtx, func(summaryText string) {
		recorder.SetSummary(summaryText, countBeforeRun)
	})

	// 执行 agent runner（HTTP 模式自动拒绝危险命令）
	historyFilePath := fmt.Sprintf("%sfkteams_chat_history_%s", historyDir, sessionID)

	_, err := engine.New(r, "fkteams").Run(taskCtx, inputMessages, engine.WithInterruptHandler(engine.AutoRejectHandler()))
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "closed network connection") ||
			strings.Contains(errMsg, "broken pipe") ||
			strings.Contains(errMsg, "connection reset") ||
			taskCtx.Err() != nil {
			log.Printf("connection closed, stopping: session=%s", sessionID)
			saveHistory(recorder, historyFilePath, sessionID)
			return
		}
		log.Printf("error processing event: %v", err)
	}

	saveHistory(recorder, historyFilePath, sessionID)

	if g.MemoryManager != nil {
		g.MemoryManager.ExtractFromRecorder(recorder, sessionID)
	}

	// 发送结束事件
	data, _ := json.Marshal(map[string]string{"type": "processing_end", "message": "处理完成"})
	fmt.Fprintf(c.Writer, "data: %s\n\n", data)
	c.Writer.Flush()

	defer func() {
		if err := g.Cleaner.ExecuteAndClear(); err != nil {
			fmt.Printf("failed to cleanup resources: %v\n", err)
		}
	}()
}

// handleSyncChat 同步聊天响应（收集完整结果后返回）
func handleSyncChat(c *gin.Context, ctx context.Context, r *adk.Runner, recorder *fkevent.HistoryRecorder, inputMessages []adk.Message, countBeforeRun int, sessionID string) {
	taskCtx, taskCancel := context.WithCancel(ctx)
	defer taskCancel()

	var events []fkevent.Event

	taskCtx = fkevent.WithCallback(taskCtx, func(event fkevent.Event) error {
		recorder.RecordEvent(event)
		events = append(events, event)
		return nil
	})

	taskCtx = summary.WithSummaryPersistCallback(taskCtx, func(summaryText string) {
		recorder.SetSummary(summaryText, countBeforeRun)
	})

	historyFilePath := fmt.Sprintf("%sfkteams_chat_history_%s", historyDir, sessionID)

	_, err := engine.New(r, "fkteams").Run(taskCtx, inputMessages, engine.WithInterruptHandler(engine.AutoRejectHandler()))
	if err != nil {
		log.Printf("error processing event: %v", err)
	}

	saveHistory(recorder, historyFilePath, sessionID)

	if g.MemoryManager != nil {
		g.MemoryManager.ExtractFromRecorder(recorder, sessionID)
	}

	defer func() {
		if err := g.Cleaner.ExecuteAndClear(); err != nil {
			fmt.Printf("failed to cleanup resources: %v\n", err)
		}
	}()

	// 收集最终文本内容
	var content strings.Builder
	for _, e := range events {
		if e.Content != "" {
			content.WriteString(e.Content)
		}
	}

	OK(c, gin.H{
		"session_id": sessionID,
		"content":    content.String(),
		"events":     events,
	})
}
