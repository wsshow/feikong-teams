package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// Documents 将应用数据根目录下的结构化文档映射到 SQLite 记录。
type Documents struct {
	database *Database
	root     string
}

// NewDocuments 绑定逻辑根目录，附件等实体文件仍由文件适配器管理。
func NewDocuments(database *Database, root string) (*Documents, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Documents{database: database, root: abs}, nil
}
func (s *Documents) key(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(s.root, abs)
	if err != nil || rel == "." || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("document path escapes data directory")
	}
	return filepath.ToSlash(rel), nil
}

// Read 读取文档快照及按顺序追加的记录。
func (s *Documents) Read(path string) ([]byte, error) {
	key, err := s.key(path)
	if err != nil {
		return nil, err
	}
	tx, err := s.database.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var data []byte
	err = tx.QueryRow("SELECT payload FROM records WHERE namespace='documents' AND key=?", key).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fs.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query("SELECT payload FROM document_fragments WHERE path=? ORDER BY sequence", key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var fragment []byte
		if err := rows.Scan(&fragment); err != nil {
			return nil, err
		}
		data = append(data, fragment...)
	}
	return data, rows.Err()
}

// Write 原子替换文档并清除旧追加记录。
func (s *Documents) Write(path string, data []byte) error {
	key, err := s.key(path)
	if err != nil {
		return err
	}
	tx, err := s.database.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM document_fragments WHERE path=?", key); err != nil {
		return err
	}
	if data == nil {
		data = []byte{}
	}
	if _, err = tx.Exec("INSERT INTO records(namespace,key,payload) VALUES('documents',?,?) ON CONFLICT(namespace,key) DO UPDATE SET payload=excluded.payload", key, data); err != nil {
		return err
	}
	return tx.Commit()
}

// Append 原子追加独立片段，避免每条历史消息重写整个会话。
func (s *Documents) Append(path string, data []byte) error {
	key, err := s.key(path)
	if err != nil {
		return err
	}
	tx, err := s.database.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("INSERT OR IGNORE INTO records(namespace,key,payload) VALUES('documents',?,x'')", key); err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO document_fragments(path,sequence,payload) SELECT ?,COALESCE(MAX(sequence),0)+1,? FROM document_fragments WHERE path=?", key, data, key); err != nil {
		return err
	}
	return tx.Commit()
}

// Remove 原子删除单个文档。
func (s *Documents) Remove(path string) error {
	key, err := s.key(path)
	if err != nil {
		return err
	}
	tx, err := s.database.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM document_fragments WHERE path=?", key); err != nil {
		return err
	}
	result, err := tx.Exec("DELETE FROM records WHERE namespace='documents' AND key=?", key)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return fs.ErrNotExist
	}
	return tx.Commit()
}

// List 列出目录下所有文档路径，不把用户路径当作 SQL 通配符。
func (s *Documents) List(directory string) ([]string, error) {
	prefix, err := s.key(filepath.Join(directory, "placeholder"))
	if err != nil {
		return nil, err
	}
	prefix = strings.TrimSuffix(prefix, "placeholder")
	rows, err := s.database.db.Query("SELECT key FROM records WHERE namespace='documents' AND substr(key,1,length(?))=? ORDER BY key", prefix, prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	paths := []string{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		paths = append(paths, filepath.Join(s.root, filepath.FromSlash(key)))
	}
	return paths, rows.Err()
}

// RemoveTree 在同一事务中移除目录文档，保留指定文档及其追加记录。
func (s *Documents) RemoveTree(directory string, preserve ...string) error {
	prefix, err := s.key(filepath.Join(directory, "placeholder"))
	if err != nil {
		return err
	}
	prefix = strings.TrimSuffix(prefix, "placeholder")
	args := []any{prefix, prefix}
	var exclusions []string
	for _, path := range preserve {
		key, err := s.key(path)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(key, prefix) {
			return fmt.Errorf("preserved document path escapes directory")
		}
		exclusions = append(exclusions, "?")
		args = append(args, key)
	}
	condition := ""
	if len(exclusions) > 0 {
		condition = " NOT IN (" + strings.Join(exclusions, ",") + ")"
	}
	tx, err := s.database.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	fragmentQuery := "DELETE FROM document_fragments WHERE substr(path,1,length(?))=?"
	recordQuery := "DELETE FROM records WHERE namespace='documents' AND substr(key,1,length(?))=?"
	if condition != "" {
		fragmentQuery += " AND path" + condition
		recordQuery += " AND key" + condition
	}
	if _, err = tx.Exec(fragmentQuery, args...); err != nil {
		return err
	}
	if _, err = tx.Exec(recordQuery, args...); err != nil {
		return err
	}
	return tx.Commit()
}
