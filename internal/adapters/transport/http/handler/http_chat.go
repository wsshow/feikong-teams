package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"fkteams/internal/adapters/storage/file/history"
	"fkteams/internal/app/appstate"
	appchat "fkteams/internal/app/chat"
	domainmessage "fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/events"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ChatRequest HTTP 聊天请求
type ChatRequest struct {
	SessionID string        `json:"session_id"`
	Message   string        `json:"message"`
	Mode      string        `json:"mode"`
	AgentName string        `json:"agent_name"`
	Stream    bool          `json:"stream"`
	Contents  []ContentPart `json:"contents"`
}

// ChatHandler HTTP POST 聊天处理器，支持普通 JSON 响应和 SSE 流式响应
func ChatHandler() gin.HandlerFunc {
	return NewRuntime().ChatHandlerWithState(nil)
}

// ChatHandlerWithState HTTP POST 聊天处理器，使用显式应用状态。
func ChatHandlerWithState(state *appstate.State) gin.HandlerFunc {
	return NewRuntime().ChatHandlerWithState(state)
}

// ChatHandlerWithState HTTP POST 聊天处理器，使用当前 HTTP runtime 的显式依赖。
func (rt *Runtime) ChatHandlerWithState(state *appstate.State) gin.HandlerFunc {
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
			mode = "team"
		}

		ctx := appstate.WithState(c.Request.Context(), state)
		r, err := rt.resolveRunner(ctx, mode, req.AgentName)
		if err != nil {
			log.Printf("failed to resolve runner: mode=%s, agent=%s, err=%v", mode, req.AgentName, err)
			status := http.StatusInternalServerError
			if req.AgentName != "" {
				status = http.StatusBadRequest
			}
			Fail(c, status, err.Error())
			return
		}

		recorder := rt.recorder(sessionID)
		manager := memoryFromState(state)
		turnInput, userDisplayText := buildChatInput(recorder, req.Message, req.Contents, manager)

		if req.Stream {
			rt.handleStreamChat(c, ctx, r, recorder, turnInput, sessionID, userDisplayText, manager)
		} else {
			rt.handleSyncChat(c, ctx, r, recorder, turnInput, sessionID, userDisplayText, manager)
		}
	}
}

// handleStreamChat SSE 流式聊天响应
func (rt *Runtime) handleStreamChat(c *gin.Context, ctx context.Context, r runtimeport.Runner, recorder *eventlog.HistoryRecorder, turnInput domainmessage.TurnInput, sessionID, userDisplayText string, manager appstate.MemoryManager) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	taskCtx, taskCancel := context.WithCancel(ctx)
	defer taskCancel()

	_, runErr := appchat.NewService().RunTurn(taskCtx, appchat.TurnRequest{
		SessionID: sessionID,
		Runner:    r,
		Input:     turnInput,
	},
		appchat.OnEvent(func(event events.Event) error {
			data, _ := json.Marshal(convertEventToMap(event))
			_, err := fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			c.Writer.Flush()
			return err
		}),
		appchat.WithEventRecorder(recorder),
		appchat.WithHistory(recorder),
		appchat.OnFinish(func(ctx context.Context, _ *runtimeport.RunResult, err error) {
			if err != nil {
				if isConnectionClosed(ctx, err) {
					log.Printf("connection closed, stopping: session=%s", sessionID)
					rt.saveTurnHistory(recorder, sessionID)
					return
				}
				log.Printf("error processing event: %v", err)
				rt.finishErrorChat(recorder, sessionID, userDisplayText, err)
				data, _ := json.Marshal(errorEventPayload("", err.Error()))
				fmt.Fprintf(c.Writer, "data: %s\n\n", data)
				c.Writer.Flush()
				return
			}
			rt.finishChat(recorder, sessionID, userDisplayText, manager)
			data, _ := json.Marshal(map[string]string{"type": string(events.NotifyProcessingEnd), "message": "处理完成"})
			fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			c.Writer.Flush()
		}),
	)
	if runErr != nil && !isConnectionClosed(taskCtx, runErr) {
		log.Printf("stream chat failed: session=%s, err=%v", sessionID, runErr)
	}
}

// handleSyncChat 同步聊天响应（收集完整结果后返回）
func (rt *Runtime) handleSyncChat(c *gin.Context, ctx context.Context, r runtimeport.Runner, recorder *eventlog.HistoryRecorder, turnInput domainmessage.TurnInput, sessionID, userDisplayText string, manager appstate.MemoryManager) {
	taskCtx, taskCancel := context.WithCancel(ctx)
	defer taskCancel()

	var collectedEvents []events.Event

	_, runErr := appchat.NewService().RunTurn(taskCtx, appchat.TurnRequest{
		SessionID: sessionID,
		Runner:    r,
		Input:     turnInput,
	},
		appchat.OnEvent(func(event events.Event) error {
			collectedEvents = append(collectedEvents, event)
			return nil
		}),
		appchat.WithEventRecorder(recorder),
		appchat.WithHistory(recorder),
		appchat.OnFinish(func(ctx context.Context, _ *runtimeport.RunResult, err error) {
			if err != nil {
				log.Printf("error processing event: %v", err)
				rt.finishErrorChat(recorder, sessionID, userDisplayText, err)
				return
			}
			rt.finishChat(recorder, sessionID, userDisplayText, manager)
		}),
	)
	if runErr != nil {
		Fail(c, http.StatusInternalServerError, runErr.Error())
		return
	}

	var content strings.Builder
	for _, e := range collectedEvents {
		if e.Content != "" {
			content.WriteString(e.Content)
		}
	}

	OK(c, gin.H{
		"session_id": sessionID,
		"content":    content.String(),
		"events":     collectedEvents,
	})
}
