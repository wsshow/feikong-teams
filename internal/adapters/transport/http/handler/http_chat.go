package handler

import (
	"context"
	"errors"
	"fkteams/internal/app/chat/taskstream"
	"fkteams/internal/domain/apperror"
	"fkteams/internal/runtime/log"
	"fmt"
	"net/http"
	"os"
	"time"

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
		if !validateSessionID(sessionID) {
			Fail(c, http.StatusBadRequest, "invalid session ID")
			return
		}
		if req.Stream {
			Fail(c, http.StatusBadRequest, "stream=true is not supported on /api/fkteams/chat; use /api/fkteams/stream/start")
			return
		}
		mode := req.Mode
		if mode == "" {
			mode = "team"
		}

		releaseTask, accepted := rt.beginTask()
		if !accepted {
			Fail(c, http.StatusServiceUnavailable, "HTTP runtime is shutting down")
			return
		}
		defer releaseTask()
		ctx, cancelRequest := context.WithCancel(appstate.WithState(c.Request.Context(), state))
		stop := context.AfterFunc(rt.taskContext, cancelRequest)
		defer stop()
		defer cancelRequest()
		releaseExecution := rt.ExecutionGate.Begin()
		defer releaseExecution()
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

		unlock := rt.lockSessionOperation(sessionID)
		taskCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		stream, admitted, queueErr := rt.Queues.RegisterIfIdle(taskstream.StreamConfig{SessionID: sessionID, Cancel: cancel, Mode: mode, AgentName: req.AgentName, CleanupTTL: 5 * time.Minute, QueueDisabled: true})
		if queueErr != nil {
			unlock()
			writeTaskQueueError(c, queueErr)
			return
		}
		if !admitted {
			unlock()
			Fail(c, http.StatusConflict, "session is already processing")
			return
		}
		defer stream.Done()
		recorder, releaseRecorder := rt.acquireRecorderLocked(sessionID)
		defer releaseRecorder()
		records, viewErr := eventlog.LoadSessionTranscriptRecords(rt.sessionDirPath(sessionID), rt.Documents)
		if viewErr != nil && !errors.Is(viewErr, os.ErrNotExist) {
			unlock()
			publishStreamFailure(stream, sessionID, "failed to load conversation")
			Fail(c, http.StatusInternalServerError, "failed to load conversation")
			return
		}
		projector, viewErr := rt.transcriptPresentation(sessionID, records)
		if viewErr != nil {
			unlock()
			publishStreamFailure(stream, sessionID, apperror.PublicMessage(viewErr))
			FailError(c, viewErr)
			return
		}
		stream.SetPresentation(projector)
		unlock()
		if err := rt.updateSessionExecutionMetadata(sessionID, req.Message, mode, req.AgentName); err != nil {
			publishStreamFailure(stream, sessionID, err.Error())
			Fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		manager := memoryFromState(state)
		turnInput, userDisplayText := buildChatInput(recorder, req.Message, req.Contents, manager)

		rt.handleSyncChat(c, taskCtx, r, recorder, turnInput, sessionID, userDisplayText, manager, stream)
	}
}

// handleSyncChat 同步聊天响应（收集完整结果后返回）
func (rt *Runtime) handleSyncChat(c *gin.Context, ctx context.Context, r runtimeport.Runner, recorder *eventlog.HistoryRecorder, turnInput domainmessage.TurnInput, sessionID, userDisplayText string, manager appstate.MemoryManager, stream *taskstream.Stream) {
	taskCtx, taskCancel := context.WithCancel(ctx)
	defer taskCancel()
	taskCtx = rt.withExecutionDependencies(taskCtx)

	var collectedEvents []events.Event

	request := func(input domainmessage.TurnInput, runID string) appchat.TurnRequest {
		return appchat.TurnRequest{
			SessionID:        sessionID,
			RunID:            runID,
			Runner:           r,
			Input:            input,
			Summary:          recorder,
			History:          recorder,
			NonInteractive:   true,
			ApprovalRegistry: configuredApprovalRegistry(),
			EventSink: func(event events.Event) error {
				collectedEvents = append(collectedEvents, event)
				stream.PublishRuntime(event, standardEventPayload(sessionID, event, rt.ToolDisplays))
				return nil
			},
			OnFinish: func(ctx context.Context, _ *runtimeport.RunResult, err error) error {
				return rt.finishChat(ctx, sessionID, userDisplayText, err)
			},
		}
	}
	runErr := appchat.NewService().RunQueued(taskCtx, appchat.QueueRequest{
		Stream: stream, Queues: rt.Queues, First: request(turnInput, ""),
		Next: func(queued taskstream.QueuedMessage) (appchat.TurnRequest, error) {
			runID := appchat.QueuedRunID(sessionID, queued)
			publishQueuedExecutionStart(stream, sessionID, queued, runID)
			return request(buildQueuedChatInput(recorder, queued, manager), runID), nil
		},
	})
	stream.SetStatus(string(appchat.CompletionStatus(runErr)))
	records, projectionErr := eventlog.LoadSessionTranscriptRecords(rt.sessionDirPath(sessionID), rt.Documents)
	if projectionErr != nil && !errors.Is(projectionErr, os.ErrNotExist) {
		publishStreamFailure(stream, sessionID, "failed to read committed conversation")
		Fail(c, http.StatusInternalServerError, "failed to read committed conversation")
		return
	}
	projector, projectionErr := rt.transcriptPresentation(sessionID, records)
	if projectionErr != nil {
		publishStreamFailure(stream, sessionID, "failed to project committed conversation")
		Fail(c, http.StatusInternalServerError, "failed to project committed conversation")
		return
	}
	terminal := processingEndEventPayload(sessionID, "", "处理完成")
	terminal["status"] = string(appchat.CompletionStatus(runErr))
	stream.PublishPresentation(projector, terminal)

	if runErr != nil {
		Fail(c, http.StatusInternalServerError, runErr.Error())
		return
	}

	OK(c, gin.H{
		"session_id": sessionID,
		"content":    appchat.AssistantText(collectedEvents),
		"view":       projector.Snapshot(),
		"events":     collectedEvents,
	})
}
