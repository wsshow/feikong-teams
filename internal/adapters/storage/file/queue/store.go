// Package queue 保存可恢复的会话输入队列及其附件。
package queue

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	domainmessage "fkteams/internal/domain/message"
	domainsession "fkteams/internal/domain/session"
	storageport "fkteams/internal/ports/storage"
	"fkteams/internal/runtime/atomicfile"
	"fkteams/internal/runtime/log"
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
type Store struct {
	dir       string
	Documents storageport.Documents
}

// NewStore 创建队列存储，错误由读写操作返回。
func NewStore(dir string, stores ...storageport.Documents) *Store {
	s := &Store{dir: dir}
	if len(stores) > 0 {
		s.Documents = stores[0]
	}
	return s
}

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
		if s.Documents != nil {
			if removeErr := s.removeSnapshot(nil, sessionID); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				return removeErr
			}
		}
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	if len(queue) == 0 {
		// 先验证清理边界，提交之后的垃圾回收失败不能否定快照提交。
		if err := pathguard.RejectRootSymlinkComponents(root, "attachments/queue"); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := s.removeSnapshot(root, sessionID); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove queue snapshot: %w", err)
		}
		if err := root.RemoveAll("attachments/queue"); err != nil {
			log.Printf("queue attachment cleanup failed: %v", err)
		}
		return nil
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
	if err := s.writeSnapshot(root, sessionID, data); err != nil {
		return err
	}
	if err := cleanupQueueAttachmentDirs(root, items); err != nil {
		log.Printf("queue attachment cleanup failed: %v", err)
	}
	return nil
}

func (s *Store) Load(sessionID string) ([]domainmessage.QueuedMessage, error) {
	if !domainsession.ValidID(sessionID) {
		return nil, fmt.Errorf("invalid session ID")
	}
	root, err := s.openSession(sessionID, false)
	if errors.Is(err, os.ErrNotExist) {
		if s.Documents != nil {
			if _, readErr := s.readSnapshot(nil, sessionID); !errors.Is(readErr, os.ErrNotExist) {
				if readErr != nil {
					return nil, readErr
				}
				return nil, fmt.Errorf("queue attachment directory is missing")
			}
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	data, err := s.readSnapshot(root, sessionID)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
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
		if !domainsession.ValidID(item.ID) {
			return nil, fmt.Errorf("invalid queue ID")
		}
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
		// 内容寻址使未提交的附件写入不会覆盖上一版本引用的文件。
		relPath = filepath.Join(filepath.Dir(relPath), fmt.Sprintf("%x-%s", sha256.Sum256(data), filepath.Base(relPath)))
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
	referenced := make(map[string]bool)
	active := make(map[string]bool, len(queue))
	for _, item := range queue {
		active[item.ID] = true
		for _, part := range item.Parts {
			if isQueueAttachmentPath(part.URL) {
				referenced[filepath.Clean(part.URL)] = true
			}
		}
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		directory := filepath.Join(queueDir, entry.Name())
		if !active[entry.Name()] {
			if err := root.RemoveAll(directory); err != nil {
				return fmt.Errorf("remove stale queue attachment dir: %w", err)
			}
			continue
		}
		if err := pathguard.RejectRootSymlinkComponents(root, directory); err != nil {
			return err
		}
		dir, err := root.Open(directory)
		if err != nil {
			return err
		}
		files, readErr := dir.ReadDir(-1)
		closeErr := dir.Close()
		if err := errors.Join(readErr, closeErr); err != nil {
			return err
		}
		for _, file := range files {
			path := filepath.Join(directory, file.Name())
			if referenced[path] || file.IsDir() {
				continue
			}
			// Remove 只移除当前目录项，不跟随符号链接。
			if err := root.Remove(path); err != nil {
				return fmt.Errorf("remove stale queue attachment: %w", err)
			}
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

func (s *Store) readSnapshot(root *os.Root, sessionID string) ([]byte, error) {
	if s.Documents != nil {
		return s.Documents.Read(filepath.Join(s.dir, sessionID, queueSnapshotFileName))
	}
	if err := pathguard.RejectRootSymlinkComponents(root, queueSnapshotFileName); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	file, err := root.Open(queueSnapshotFileName)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("read queue snapshot: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxPersistentQueueSnapshotBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read queue snapshot: %w", err)
	}
	return data, nil
}

func (s *Store) writeSnapshot(root *os.Root, sessionID string, data []byte) error {
	if s.Documents != nil {
		return s.Documents.Write(filepath.Join(s.dir, sessionID, queueSnapshotFileName), data)
	}
	return atomicfile.WriteFileInRoot(root, queueSnapshotFileName, data, 0644)
}
func (s *Store) removeSnapshot(root *os.Root, sessionID string) error {
	if s.Documents != nil {
		return s.Documents.Remove(filepath.Join(s.dir, sessionID, queueSnapshotFileName))
	}
	return root.Remove(queueSnapshotFileName)
}
