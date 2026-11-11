package handler

import (
	filequeue "fkteams/internal/adapters/storage/file/queue"
	"fkteams/internal/app/chat/taskstream"
	"fkteams/internal/runtime/log"
	"github.com/gin-gonic/gin"
	"net/http"
)

func (rt *Runtime) persistQueueSnapshot(sessionID string, stream *taskstream.Stream) {
	if stream == nil {
		return
	}
	if err := rt.savePersistentQueue(sessionID, stream.QueueSnapshot()); err != nil {
		log.Printf("failed to persist queue: session=%s, err=%v", sessionID, err)
	}
}

func (rt *Runtime) restorePersistentQueue(sessionID string, stream *taskstream.Stream) {
	if stream == nil {
		return
	}
	queue, err := rt.loadPersistentQueue(sessionID)
	if err != nil {
		log.Printf("failed to restore persisted queue: session=%s, err=%v", sessionID, err)
		return
	}
	if len(queue) == 0 {
		return
	}
	stream.RestoreQueue(queue)
}

func (rt *Runtime) queueForSessionResponse(sessionID string, stream *taskstream.Stream) []taskstream.QueuedMessage {
	if stream != nil {
		return stream.QueueSnapshot()
	}
	queue, err := rt.loadPersistentQueue(sessionID)
	if err != nil {
		log.Printf("failed to load persisted queue: session=%s, err=%v", sessionID, err)
		return nil
	}
	return queue
}

func (rt *Runtime) editQueue(c *gin.Context, sessionID string) (*taskstream.Stream, bool) {
	if !validateSessionID(sessionID) {
		Fail(c, http.StatusBadRequest, "invalid session ID")
		return nil, false
	}
	if stream := rt.Streams.Get(sessionID); stream != nil && stream.Status() == "processing" {
		return stream, true
	}
	queue, err := rt.loadPersistentQueue(sessionID)
	if err != nil {
		log.Printf("failed to load persisted queue: session=%s, err=%v", sessionID, err)
		Fail(c, http.StatusInternalServerError, "failed to load queued messages")
		return nil, false
	}
	if len(queue) == 0 {
		Fail(c, http.StatusNotFound, "no queued messages for this session")
		return nil, false
	}
	manager := taskstream.NewManager()
	stream := manager.Register(taskstream.StreamConfig{
		SessionID: sessionID,
		Cancel:    func() {},
	})
	stream.RestoreQueue(queue)
	return stream, true
}

func (rt *Runtime) savePersistentQueue(sessionID string, items []taskstream.QueuedMessage) error {
	return filequeue.NewStore(rt.HistoryDir).Save(sessionID, items)
}

func (rt *Runtime) loadPersistentQueue(sessionID string) ([]taskstream.QueuedMessage, error) {
	return filequeue.NewStore(rt.HistoryDir).Load(sessionID)
}
