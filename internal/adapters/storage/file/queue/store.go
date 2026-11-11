// Package queue 保存可恢复的会话输入队列及其附件。
package queue

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	domainmessage "fkteams/internal/domain/message"
	domainsession "fkteams/internal/domain/session"
	"fkteams/internal/runtime/atomicfile"
	"fkteams/internal/runtime/pathguard"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Store 管理会话目录中的队列快照。
type Store struct{ dir string }

// NewStore 创建队列存储，错误由读写操作返回。
func NewStore(dir string) *Store { return &Store{dir: dir} }

const (
	queueSnapshotFileName             = "queue.json"
	maxPersistentQueueSnapshotBytes   = 8 << 20
	maxPersistentQueueAttachmentBytes = 4 << 20
	maxPersistentQueueRestoredBytes   = 4 << 20
	maxPersistentQueueItems           = 100
)

type queueSnapshotFile struct {
	Version   int                           `json:"version"`
	SessionID string                        `json:"session_id"`
	UpdatedAt time.Time                     `json:"updated_at"`
	Items     []domainmessage.QueuedMessage `json:"items"`
}

func (s *Store) Save(sessionID string, queue []domainmessage.QueuedMessage) error {
	if !domainsession.ValidID(sessionID) {
		return fmt.Errorf("invalid session ID")
	}
	root, err := s.openSession(sessionID, len(queue) > 0)
	if errors.Is(err, os.ErrNotExist) && len(queue) == 0 {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	if len(queue) == 0 {
		if err := root.Remove(queueSnapshotFileName); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove queue snapshot: %w", err)
		}
		if err := pathguard.RejectRootSymlinkComponents(root, "attachments/queue"); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		return root.RemoveAll("attachments/queue")
	}
	if len(queue) > maxPersistentQueueItems {
		return fmt.Errorf("queue snapshot contains too many items")
	}

	items := make([]domainmessage.QueuedMessage, len(queue))
	copy(items, queue)
	for i := range items {
		if !domainsession.ValidID(items[i].ID) {
			return fmt.Errorf("invalid queue ID")
		}
		item, err := externalizeQueueAttachments(root, items[i])
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
	if len(data) > maxPersistentQueueSnapshotBytes {
		return fmt.Errorf("queue snapshot is too large")
	}
	if err := atomicfile.WriteFileInRoot(root, queueSnapshotFileName, data, 0644); err != nil {
		return err
	}
	return cleanupQueueAttachmentDirs(root, items)
}

func (s *Store) Load(sessionID string) ([]domainmessage.QueuedMessage, error) {
	if !domainsession.ValidID(sessionID) {
		return nil, fmt.Errorf("invalid session ID")
	}
	root, err := s.openSession(sessionID, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if err := pathguard.RejectRootSymlinkComponents(root, queueSnapshotFileName); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	file, err := root.Open(queueSnapshotFileName)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read queue snapshot: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxPersistentQueueSnapshotBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read queue snapshot: %w", err)
	}
	if len(data) > maxPersistentQueueSnapshotBytes {
		return nil, fmt.Errorf("queue snapshot is too large")
	}
	var snapshot queueSnapshotFile
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("decode queue snapshot: %w", err)
	}
	if snapshot.Version != 1 || snapshot.SessionID != sessionID {
		return nil, fmt.Errorf("invalid queue snapshot metadata")
	}
	if len(snapshot.Items) > maxPersistentQueueItems {
		return nil, fmt.Errorf("queue snapshot contains too many items")
	}
	items := make([]domainmessage.QueuedMessage, 0, len(snapshot.Items))
	restoredBytes := 0
	for _, item := range snapshot.Items {
		restored, err := restoreQueueAttachments(root, item)
		if err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(restored)
		if err != nil {
			return nil, fmt.Errorf("measure restored queue item: %w", err)
		}
		if restoredBytes+len(encoded) > maxPersistentQueueRestoredBytes {
			return nil, fmt.Errorf("restored queue is too large")
		}
		restoredBytes += len(encoded)
		items = append(items, restored)
	}
	return items, nil
}

func persistentQueuePath(sessionDir string) string {
	return filepath.Join(sessionDir, queueSnapshotFileName)
}

func externalizeQueueAttachments(root *os.Root, item domainmessage.QueuedMessage) (domainmessage.QueuedMessage, error) {
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
		if len(data) > maxPersistentQueueAttachmentBytes {
			return item, fmt.Errorf("queued image is too large")
		}
		relPath := queueAttachmentRelPath(item.ID, i, part)
		if err := pathguard.EnsureRootDirectory(root, filepath.Dir(relPath), 0755); err != nil {
			return item, fmt.Errorf("create queue attachment dir: %w", err)
		}
		if err := atomicfile.WriteFileInRoot(root, relPath, data, 0644); err != nil {
			return item, fmt.Errorf("write queue attachment: %w", err)
		}
		part.URL = relPath
		part.Base64Data = ""
		parts[i] = part
	}
	item.Parts = parts
	return item, nil
}

func restoreQueueAttachments(root *os.Root, item domainmessage.QueuedMessage) (domainmessage.QueuedMessage, error) {
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
		if err := pathguard.RejectRootSymlinkComponents(root, part.URL); err != nil {
			return item, err
		}
		file, err := root.Open(filepath.Clean(part.URL))
		if err != nil {
			return item, fmt.Errorf("read queue attachment: %w", err)
		}
		data, readErr := io.ReadAll(io.LimitReader(file, maxPersistentQueueAttachmentBytes+1))
		closeErr := file.Close()
		if readErr != nil {
			return item, fmt.Errorf("read queue attachment: %w", readErr)
		}
		if closeErr != nil {
			return item, fmt.Errorf("close queue attachment: %w", closeErr)
		}
		if len(data) > maxPersistentQueueAttachmentBytes {
			return item, fmt.Errorf("queue attachment is too large")
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

func cleanupQueueAttachmentDirs(root *os.Root, queue []domainmessage.QueuedMessage) error {
	queueDir := filepath.Join("attachments", "queue")
	if err := pathguard.RejectRootSymlinkComponents(root, queueDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	dir, err := root.Open(queueDir)
	if err != nil {
		return err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
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
		if err := root.RemoveAll(filepath.Join(queueDir, entry.Name())); err != nil {
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

// openSession 固定根目录，后续读写和清理均使用相对路径。
func (s *Store) openSession(sessionID string, create bool) (*os.Root, error) {
	if create {
		if err := os.MkdirAll(s.dir, 0755); err != nil {
			return nil, err
		}
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if create {
		err = pathguard.EnsureRootDirectory(root, sessionID, 0755)
	} else {
		err = pathguard.RejectRootSymlinkComponents(root, sessionID)
	}
	if err != nil {
		return nil, err
	}
	return root.OpenRoot(sessionID)
}
