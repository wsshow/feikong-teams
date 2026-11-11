package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	domainmigration "fkteams/internal/domain/migration"
	"fmt"
	"io/fs"
	"os"
	"time"
)

const fileMigrationID = "file-to-sqlite-v1"

// Completed 判断文件存储升级是否已经原子提交。
func (d *Database) Completed(ctx context.Context) (bool, error) {
	var count int
	err := d.db.QueryRowContext(ctx, "SELECT count(*) FROM migrations WHERE id=?", fileMigrationID).Scan(&count)
	return count == 1, err
}

// Import 在同一事务中导入并逐条回读校验，完成标记与业务数据一起提交。
func (d *Database) Import(ctx context.Context, snapshot domainmigration.Snapshot, progress func(string, int, int)) error {
	if done, err := d.Completed(ctx); err != nil {
		return err
	} else if done {
		return nil
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var existing int
	if err := tx.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM records)+(SELECT count(*) FROM memories)+(SELECT count(*) FROM document_fragments)").Scan(&existing); err != nil {
		return err
	}
	if existing != 0 {
		return fmt.Errorf("refusing to overwrite database without completed migration marker")
	}
	total := len(snapshot.Documents) + len(snapshot.Memories)
	for i, document := range snapshot.Documents {
		if !fs.ValidPath(document.Path) {
			return fmt.Errorf("invalid migration document path")
		}
		data, err := migrationDocument(document)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO records(namespace,key,payload) VALUES('documents',?,?)", document.Path, data); err != nil {
			return err
		}
		progress("import", i+1, total)
	}
	for i, entry := range snapshot.Memories {
		if entry.ID == "" {
			return fmt.Errorf("memory ID is required")
		}
		data, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO memories(id,position,payload) VALUES(?,?,?)", entry.ID, i, string(data)); err != nil {
			return err
		}
		progress("import", len(snapshot.Documents)+i+1, total)
	}
	for i, document := range snapshot.Documents {
		var data []byte
		if err := tx.QueryRowContext(ctx, "SELECT payload FROM records WHERE namespace='documents' AND key=?", document.Path).Scan(&data); err != nil {
			return err
		}
		expected, err := migrationDocument(document)
		if err != nil {
			return err
		}
		if sha256.Sum256(data) != sha256.Sum256(expected) {
			return fmt.Errorf("document verification failed: %s", document.Path)
		}
		progress("verify", i+1, total)
	}
	for i, entry := range snapshot.Memories {
		expected, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		var actual string
		if err := tx.QueryRowContext(ctx, "SELECT payload FROM memories WHERE id=?", entry.ID).Scan(&actual); err != nil {
			return err
		}
		if !bytes.Equal(expected, []byte(actual)) {
			return fmt.Errorf("memory verification failed")
		}
		progress("verify", len(snapshot.Documents)+i+1, total)
	}
	var integrity string
	if err := tx.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		return err
	}
	if integrity != "ok" {
		return fmt.Errorf("database integrity check failed")
	}
	manifest, err := json.Marshal(map[string]any{"backup_path": snapshot.BackupPath, "documents": len(snapshot.Documents), "memories": len(snapshot.Memories)})
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO migrations(id,completed_at,manifest) VALUES(?,?,?)", fileMigrationID, time.Now().UTC().Format(time.RFC3339Nano), string(manifest)); err != nil {
		return err
	}
	return tx.Commit()
}

func migrationDocument(document domainmigration.Document) ([]byte, error) {
	if document.BackupFile == "" {
		return document.Data, nil
	}
	data, err := os.ReadFile(document.BackupFile)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != document.SHA256 {
		return nil, fmt.Errorf("backup changed after verification: %s", document.Path)
	}
	return data, nil
}

// MigrationStatus 恢复已提交升级的结果和备份位置，供启动后的界面查询。
func (d *Database) MigrationStatus(ctx context.Context) (domainmigration.Status, error) {
	var manifest []byte
	if err := d.db.QueryRowContext(ctx, "SELECT manifest FROM migrations WHERE id=?", fileMigrationID).Scan(&manifest); err != nil {
		return domainmigration.Status{}, err
	}
	var result struct {
		BackupPath string `json:"backup_path"`
	}
	if err := json.Unmarshal(manifest, &result); err != nil {
		return domainmigration.Status{}, err
	}
	return domainmigration.Status{State: "completed", Stage: "completed", BackupPath: result.BackupPath}, nil
}
