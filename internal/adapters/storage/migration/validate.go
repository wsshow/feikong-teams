package migration

import (
	"encoding/json"
	"fkteams/internal/adapters/scheduler/filecron"
	eventlog "fkteams/internal/adapters/storage/file/history"
	filequeue "fkteams/internal/adapters/storage/file/queue"
	fileshare "fkteams/internal/adapters/storage/file/share"
	"fkteams/internal/adapters/tools/builtin/todo"
	"fkteams/internal/adapters/tools/javascript"
	domainmigration "fkteams/internal/domain/migration"
	domainsession "fkteams/internal/domain/session"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// validateBackup 使用运行时相同的解码器检查备份，失败时禁止提交迁移标记。
func validateBackup(root string, documents []domainmigration.Document) error {
	for _, document := range documents {
		parts := strings.Split(document.Path, "/")
		var err error
		switch {
		case parts[0] == "sessions":
			if !domainsession.ValidID(parts[1]) {
				return fmt.Errorf("invalid session directory: %s", document.Path)
			}
			switch {
			case len(parts) == 3 && parts[2] == "metadata.json":
				var meta *domainsession.Metadata
				if document.BackupFile == "" {
					meta = &domainsession.Metadata{}
					err = json.Unmarshal(document.Data, meta)
				} else {
					meta, err = eventlog.LoadMetadata(filepath.Dir(document.BackupFile))
				}
				if err == nil && meta.ID != parts[1] {
					err = fmt.Errorf("session metadata ID does not match directory")
				}
			case len(parts) == 3 && parts[2] == "todos.json":
				var data []byte
				data, err = os.ReadFile(document.BackupFile)
				if err == nil {
					err = todo.ValidateSnapshot(data)
				}
			case len(parts) == 3 && parts[2] == "queue.json":
				_, err = filequeue.NewStore(filepath.Join(root, "sessions")).Load(parts[1])
			case strings.HasSuffix(document.Path, "/transcript.jsonl"):
				_, err = eventlog.LoadTranscriptFromFile(document.BackupFile)
			case len(parts) == 5 && parts[4] == "metadata.json":
				var data []byte
				data, err = os.ReadFile(document.BackupFile)
				if err == nil {
					var meta eventlog.SubagentMetadata
					err = json.Unmarshal(data, &meta)
				}
			}
		case document.Path == "share/share.json":
			err = fileshare.NewPreviewLinkStore(document.BackupFile).LoadError()
		case document.Path == "share/session_share.json":
			err = fileshare.NewSessionShareStore(document.BackupFile).LoadError()
		case document.Path == "scheduler/scheduled_tasks.json":
			var data []byte
			data, err = os.ReadFile(document.BackupFile)
			if err == nil {
				err = filecron.ValidateSnapshot(data)
			}
		case strings.HasPrefix(document.Path, "javascript/state/"):
			var state *javascript.FileStateStore
			state, err = javascript.NewFileStateStore(filepath.Dir(document.BackupFile))
			if err == nil {
				_, err = state.Keys(strings.TrimSuffix(parts[2], ".json"))
			}
		}
		if err != nil {
			return fmt.Errorf("validate %s: %w", document.Path, err)
		}
	}
	return nil
}
