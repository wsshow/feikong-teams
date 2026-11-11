package handler

import (
	"fkteams/internal/app/chat/taskstream"
	"github.com/gin-gonic/gin"
	"net/http"
)

func (rt *Runtime) queueForSessionResponse(c *gin.Context, sessionID string) ([]taskstream.QueuedMessage, bool) {
	unlock := rt.lockSessionOperation(sessionID)
	defer unlock()
	items, err := rt.Queues.Snapshot(sessionID)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "failed to load queued messages")
		return nil, false
	}
	return items, true
}

func (rt *Runtime) editQueue(c *gin.Context, id string, change func(*taskstream.Queue) error) ([]taskstream.QueuedMessage, bool) {
	if !validateSessionID(id) {
		Fail(c, http.StatusBadRequest, "invalid session ID")
		return nil, false
	}
	unlock := rt.lockSessionOperation(id)
	defer unlock()
	items, err := rt.Queues.Edit(id, change)
	if err != nil {
		writeTaskQueueError(c, err)
		return nil, false
	}
	return items, true
}
