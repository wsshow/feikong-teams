package handler

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fkteams/internal/runtime/log"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"fkteams/internal/app/chat/taskstream"
	domainmessage "fkteams/internal/domain/message"
	"fkteams/internal/runtime/atomicfile"

	"github.com/gin-gonic/gin"
)

const queueSnapshotFileName = "queue.json"

type queueSnapshotFile struct {
	Version   int                        `json:"version"`
	SessionID string                     `json:"session_id"`
	UpdatedAt time.Time                  `json:"updated_at"`
	Items     []taskstream.QueuedMessage `json:"items"`
}

func (rt *Runtime) persistQueueSnapshot(sessionID string, stream *taskstream.Stream) {
	if stream == nil {
		return
	}
	if err := rt.savePersistentQueue(sessionID, stream.QueueSnapshot()); err != nil {
		log.Printf("failed to persist queue: session=%s, err=%v", sessionID, err)
	}
}

func (rt *Runtime) savePersistentQueue(sessionID string, queue []taskstream.QueuedMessage) error {
	if !validateSessionID(sessionID) {
		return fmt.Errorf("invalid session ID")
	}
	sessionDir := rt.sessionDirPath(sessionID)
	queuePath := persistentQueuePath(sessionDir)
	if len(queue) == 0 {
		if err := os.Remove(queuePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove queue snapshot: %w", err)
		}
		if err := os.RemoveAll(filepath.Join(sessionDir, "attachments", "queue")); err != nil {
			return fmt.Errorf("remove queue attachments: %w", err)
		}
		return nil
	}

	items := make([]taskstream.QueuedMessage, len(queue))
	copy(items, queue)
	for i := range items {
		item, err := externalizeQueueAttachments(sessionDir, items[i])
		if err != nil {
			return err
		}
		items[i] = item
	}

	data, err := json.MarshalIndent(queueSnapshotFile{
		Version:   1,
		SessionID: sessionID,
		UpdatedAt: time.Now(),
		Items:     items,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal queue snapshot: %w", err)
	}
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}
	if err := atomicfile.WriteFile(queuePath, data, 0644); err != nil {
		return err
	}
	return cleanupQueueAttachmentDirs(sessionDir, items)
}

func (rt *Runtime) loadPersistentQueue(sessionID string) ([]taskstream.QueuedMessage, error) {
	if !validateSessionID(sessionID) {
		return nil, fmt.Errorf("invalid session ID")
	}
	sessionDir := rt.sessionDirPath(sessionID)
	data, err := os.ReadFile(persistentQueuePath(sessionDir))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read queue snapshot: %w", err)
	}
	var snapshot queueSnapshotFile
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("decode queue snapshot: %w", err)
	}
	items := make([]taskstream.QueuedMessage, 0, len(snapshot.Items))
	for _, item := range snapshot.Items {
		restored, err := restoreQueueAttachments(sessionDir, item)
		if err != nil {
			return nil, err
		}
		items = append(items, restored)
	}
	return items, nil
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

func persistentQueuePath(sessionDir string) string {
	return filepath.Join(sessionDir, queueSnapshotFileName)
}

func externalizeQueueAttachments(sessionDir string, item taskstream.QueuedMessage) (taskstream.QueuedMessage, error) {
	if len(item.Parts) == 0 {
		return item, nil
	}
	parts := make([]domainmessage.ContentPart, len(item.Parts))
	copy(parts, item.Parts)
	for i := range parts {
		part := parts[i]
		if part.Type != domainmessage.ContentPartImageURL || part.Base64Data == "" {
			continue
		}
		data, err := decodeBase64Payload(part.Base64Data)
		if err != nil {
			return item, fmt.Errorf("decode queued image: %w", err)
		}
		relPath := queueAttachmentRelPath(item.ID, i, part)
		fullPath := filepath.Join(sessionDir, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return item, fmt.Errorf("create queue attachment dir: %w", err)
		}
		if err := atomicfile.WriteFile(fullPath, data, 0644); err != nil {
			return item, fmt.Errorf("write queue attachment: %w", err)
		}
		part.URL = relPath
		part.Base64Data = ""
		parts[i] = part
	}
	item.Parts = parts
	return item, nil
}

func restoreQueueAttachments(sessionDir string, item taskstream.QueuedMessage) (taskstream.QueuedMessage, error) {
	if len(item.Parts) == 0 {
		return item, nil
	}
	parts := make([]domainmessage.ContentPart, len(item.Parts))
	copy(parts, item.Parts)
	for i := range parts {
		part := parts[i]
		if part.Type != domainmessage.ContentPartImageURL || part.Base64Data != "" || !isQueueAttachmentPath(part.URL) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(sessionDir, filepath.Clean(part.URL)))
		if err != nil {
			return item, fmt.Errorf("read queue attachment: %w", err)
		}
		part.Base64Data = base64.StdEncoding.EncodeToString(data)
		parts[i] = part
	}
	item.Parts = parts
	return item, nil
}

func decodeBase64Payload(value string) ([]byte, error) {
	if comma := strings.IndexByte(value, ','); comma >= 0 {
		value = value[comma+1:]
	}
	value = strings.TrimSpace(value)
	if data, err := base64.StdEncoding.DecodeString(value); err == nil {
		return data, nil
	}
	return base64.RawStdEncoding.DecodeString(value)
}

func queueAttachmentRelPath(queueID string, index int, part domainmessage.ContentPart) string {
	if queueID == "" {
		queueID = "unknown"
	}
	name := safeAttachmentName(part.Name)
	ext := filepath.Ext(name)
	if ext == "" {
		ext = extensionForMIMEType(part.MIMEType)
		name += ext
	}
	return filepath.ToSlash(filepath.Join("attachments", "queue", filepath.Base(queueID), fmt.Sprintf("%02d-%s", index, name)))
}

func cleanupQueueAttachmentDirs(sessionDir string, queue []taskstream.QueuedMessage) error {
	queueDir := filepath.Join(sessionDir, "attachments", "queue")
	entries, err := os.ReadDir(queueDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read queue attachment dir: %w", err)
	}
	active := make(map[string]bool, len(queue))
	for _, item := range queue {
		if item.ID != "" {
			active[filepath.Base(item.ID)] = true
		}
	}
	for _, entry := range entries {
		if !entry.IsDir() || active[entry.Name()] {
			continue
		}
		if err := os.RemoveAll(filepath.Join(queueDir, entry.Name())); err != nil {
			return fmt.Errorf("remove stale queue attachment dir: %w", err)
		}
	}
	return nil
}

func extensionForMIMEType(mimeType string) string {
	if mimeType == "" {
		return ".bin"
	}
	exts, err := mime.ExtensionsByType(mimeType)
	if err != nil || len(exts) == 0 {
		return ".bin"
	}
	return exts[0]
}

var unsafeAttachmentNameChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func safeAttachmentName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = "image"
	}
	name = unsafeAttachmentNameChars.ReplaceAllString(name, "_")
	name = strings.Trim(name, "._-")
	if name == "" {
		return "image"
	}
	return name
}

func isQueueAttachmentPath(value string) bool {
	clean := filepath.ToSlash(filepath.Clean(value))
	return clean != "." && !strings.HasPrefix(clean, "../") && strings.HasPrefix(clean, "attachments/queue/")
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
