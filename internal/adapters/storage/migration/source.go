// Package migration 读取旧版文件存储并创建独立、可核对的迁移备份。
package migration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	filememory "fkteams/internal/adapters/storage/file/memory"
	domainmigration "fkteams/internal/domain/migration"
	domainsession "fkteams/internal/domain/session"
	"fkteams/internal/runtime/atomicfile"
	"fkteams/internal/runtime/pathguard"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Source 只备份业务数据目录，不复制配置和登录凭据。
type Source struct{ Root string }

type backupEntry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// Prepare 创建备份并验证每个文件的 SHA-256，再从备份读取迁移输入。
func (s *Source) Prepare(ctx context.Context, progress func(string, int, int)) (domainmigration.Snapshot, error) {
	snapshot := domainmigration.Snapshot{}
	root, err := os.OpenRoot(s.Root)
	if err != nil {
		return snapshot, err
	}
	defer root.Close()
	backupParent := filepath.Join(s.Root, "backups")
	if err := pathguard.EnsureRootDirectory(root, "backups", 0700); err != nil {
		return snapshot, err
	}
	backup, err := os.MkdirTemp(backupParent, "storage-"+time.Now().UTC().Format("20060102T150405")+"-")
	if err != nil {
		return snapshot, err
	}
	snapshot.BackupPath = backup
	var files []string
	for _, directory := range []string{"sessions", "scheduler", "share", "javascript/state", "workspace/memory"} {
		if err := pathguard.RejectRootSymlinkComponents(root, directory); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return snapshot, err
		}
		err := fs.WalkDir(root.FS(), directory, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("migration source contains symbolic link: %s", path)
			}
			if entry.IsDir() {
				return nil
			}
			if !entry.Type().IsRegular() {
				return fmt.Errorf("migration source is not a regular file: %s", path)
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			return snapshot, err
		}
	}
	manifest := make([]backupEntry, 0, len(files))
	for i, path := range files {
		if err := ctx.Err(); err != nil {
			return snapshot, err
		}
		if err := pathguard.RejectRootSymlinkComponents(root, path); err != nil {
			return snapshot, err
		}
		input, err := root.Open(path)
		if err != nil {
			return snapshot, err
		}
		target := filepath.Join(backup, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			input.Close()
			return snapshot, err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			input.Close()
			return snapshot, err
		}
		digest := sha256.New()
		size, copyErr := io.Copy(io.MultiWriter(output, digest), input)
		closeInput := input.Close()
		syncErr := output.Sync()
		closeOutput := output.Close()
		for _, err := range []error{copyErr, closeInput, syncErr, closeOutput} {
			if err != nil {
				return snapshot, err
			}
		}
		check, err := os.Open(target)
		if err != nil {
			return snapshot, err
		}
		verification := sha256.New()
		_, err = io.Copy(verification, check)
		closeErr := check.Close()
		if err != nil {
			return snapshot, err
		}
		if closeErr != nil {
			return snapshot, closeErr
		}
		sum := hex.EncodeToString(digest.Sum(nil))
		if hex.EncodeToString(verification.Sum(nil)) != sum {
			return snapshot, fmt.Errorf("backup verification failed: %s", path)
		}
		manifest = append(manifest, backupEntry{Path: path, SHA256: sum, Size: size})
		if isDocument(path) {
			data, err := os.ReadFile(target)
			if err != nil {
				return snapshot, err
			}
			if err := validateDocument(path, data); err != nil {
				return snapshot, err
			}
			snapshot.Documents = append(snapshot.Documents, domainmigration.Document{Path: path, BackupFile: target, SHA256: sum})
		}
		progress("backup", i+1, len(files))
	}
	if err := completeSessionMetadata(backup, files, &snapshot); err != nil {
		return snapshot, err
	}
	if err := validateBackup(backup, snapshot.Documents); err != nil {
		return snapshot, err
	}
	snapshot.Memories, err = filememory.NewStore(filepath.Join(backup, "workspace", "memory")).Load(math.MaxInt)
	if err != nil {
		return snapshot, err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return snapshot, err
	}
	if err := atomicfile.WriteFile(filepath.Join(backup, "manifest.json"), data, 0600); err != nil {
		return snapshot, err
	}
	return snapshot, nil
}
func isDocument(path string) bool {
	if strings.HasPrefix(path, "sessions/") {
		parts := strings.Split(path, "/")
		if len(parts) == 3 {
			return parts[2] == "metadata.json" || parts[2] == "transcript.jsonl" || parts[2] == "queue.json" || parts[2] == "todos.json"
		}
		return len(parts) == 5 && parts[2] == "subagents" && (parts[4] == "metadata.json" || parts[4] == "transcript.jsonl")
	}
	return path == "scheduler/scheduled_tasks.json" || path == "share/share.json" || path == "share/session_share.json" || strings.HasPrefix(path, "javascript/state/") && strings.Count(path, "/") == 2 && strings.HasSuffix(path, ".json")
}
func validateDocument(path string, data []byte) error {
	if strings.HasSuffix(path, ".jsonl") {
		for index, line := range strings.Split(string(data), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			if !json.Valid([]byte(line)) {
				return fmt.Errorf("invalid transcript JSON: %s line %d", path, index+1)
			}
		}
	} else if !json.Valid(data) {
		return fmt.Errorf("invalid JSON document: %s", path)
	}
	return nil
}

// completeSessionMetadata 保留旧版仅有 transcript 或附件的会话，避免升级后从列表消失。
func completeSessionMetadata(backup string, files []string, snapshot *domainmigration.Snapshot) error {
	existing := map[string]bool{}
	for _, document := range snapshot.Documents {
		existing[document.Path] = true
	}
	sessions := map[string]bool{}
	for _, path := range files {
		parts := strings.Split(path, "/")
		if len(parts) > 2 && parts[0] == "sessions" && domainsession.ValidID(parts[1]) {
			sessions[parts[1]] = true
		}
	}
	for id := range sessions {
		path := "sessions/" + id + "/metadata.json"
		if existing[path] {
			continue
		}
		info, err := os.Stat(filepath.Join(backup, "sessions", id))
		if err != nil {
			return err
		}
		data, err := json.Marshal(domainsession.Metadata{ID: id, Title: id, Status: domainsession.StatusActive, CreatedAt: info.ModTime(), UpdatedAt: info.ModTime()})
		if err != nil {
			return err
		}
		snapshot.Documents = append(snapshot.Documents, domainmigration.Document{Path: path, Data: data})
	}
	return nil
}
