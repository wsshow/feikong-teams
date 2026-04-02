package handler

import (
	"context"
	"encoding/json"
	"fkteams/agents/middlewares/summary"
	"fkteams/engine"
	"fkteams/fkevent"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ChatRequest HTTP 聊天请求
type ChatRequest struct {
	SessionID string          `json:"session_id"`
	Message   string          `json:"message"`
	Mode      string          `json:"mode"`
	AgentName string          `json:"agent_name"`
	Stream    bool            `json:"stream"`
	Contents  []WSContentPart `json:"contents"`
}

// ChatHandler HTTP POST 聊天处理器，支持普通 JSON 响应和 SSE 流式响应
func ChatHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ChatRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
			return
		}

		if req.Message == "" && len(req.Contents) == 0 {
			Fail(c, http.StatusBadRequest, "message or contents is required")
			return
		}

		sessionID := req.SessionID
		if sessionID == "" {
			sessionID = uuid.New().String()
		}
		mode := req.Mode
		if mode == "" {
			mode = "supervisor"
		}

		ctx := c.Request.Context()
		r, err := resolveRunner(ctx, mode, req.AgentName)
		if err != nil {
			log.Printf("failed to resolve runner: mode=%s, agent=%s, err=%v", mode, req.AgentName, err)
			status := http.StatusInternalServerError
			if req.AgentName != "" {
				status = http.StatusBadRequest
			}
			Fail(c, status, err.Error())
			return
		}

		recorder := fkevent.GlobalSessionManager.GetOrCreate(sessionID, historyDir)
		inputMessages, userDisplayText := buildChatInput(recorder, req.Message, req.Contents)
		countBeforeRun := recorder.GetMessageCount()
		recorder.RecordUserInput(userDisplayText)

		if req.Stream {
			handleStreamChat(c, ctx, r, recorder, inputMessages, countBeforeRun, sessionID, userDisplayText)
		} else {
			handleSyncChat(c, ctx, r, recorder, inputMessages, countBeforeRun, sessionID, userDisplayText)
		}
	}
}

// handleStreamChat SSE 流式聊天响应
func handleStreamChat(c *gin.Context, ctx context.Context, r *adk.Runner, recorder *fkevent.HistoryRecorder, inputMessages []adk.Message, countBeforeRun int, sessionID, userDisplayText string) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	taskCtx, taskCancel := context.WithCancel(ctx)
	defer taskCancel()

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

	// HTTP 模式自动拒绝危险命令
	_, err := engine.New(r, "fkteams").Run(taskCtx, inputMessages, engine.WithInterruptHandler(engine.AutoRejectHandler()))
	if err != nil {
		if isConnectionClosed(taskCtx, err) {
			log.Printf("connection closed, stopping: session=%s", sessionID)
			saveHistory(recorder, chatHistoryPath(sessionID), sessionID)
			return
		}
		log.Printf("error processing event: %v", err)
	}

	finishChat(recorder, sessionID, userDisplayText)

	data, _ := json.Marshal(map[string]string{"type": "processing_end", "message": "处理完成"})
	fmt.Fprintf(c.Writer, "data: %s\n\n", data)
	c.Writer.Flush()
}

// handleSyncChat 同步聊天响应（收集完整结果后返回）
func handleSyncChat(c *gin.Context, ctx context.Context, r *adk.Runner, recorder *fkevent.HistoryRecorder, inputMessages []adk.Message, countBeforeRun int, sessionID, userDisplayText string) {
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

	_, err := engine.New(r, "fkteams").Run(taskCtx, inputMessages, engine.WithInterruptHandler(engine.AutoRejectHandler()))
	if err != nil {
		log.Printf("error processing event: %v", err)
	}

	finishChat(recorder, sessionID, userDisplayText)

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
