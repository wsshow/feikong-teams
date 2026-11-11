// Package sqlite 提供实例级 SQLite 持久化，不依赖全局驱动注册表。
package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	modernsqlite "modernc.org/sqlite"
)

type connector struct {
	driver *modernsqlite.Driver
	dsn    string
}

func (c *connector) Connect(ctx context.Context) (driver.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.driver.Open(c.dsn)
}
func (c *connector) Driver() driver.Driver { return c.driver }

// Database 持有应用实例的连接池；使用方退出后才能关闭。
type Database struct{ db *sql.DB }

// Open 创建并校验业务数据库，拒绝数据库文件及直接父目录的符号链接。
func Open(ctx context.Context, path string) (*Database, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return nil, err
	}
	for _, p := range []string{parent, path, path + "-wal", path + "-shm"} {
		info, err := os.Lstat(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("database path contains symbolic link: %s", p)
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	q := u.Query()
	for _, pragma := range []string{"busy_timeout(5000)", "foreign_keys(1)", "synchronous(FULL)"} {
		q.Add("_pragma", pragma)
	}
	u.RawQuery = q.Encode()
	db := sql.OpenDB(&connector{driver: &modernsqlite.Driver{}, dsn: u.String()})
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	fail := func(err error) (*Database, error) { _ = db.Close(); return nil, err }
	if err := db.PingContext(ctx); err != nil {
		return fail(err)
	}
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fail(err)
	}
	if version > 1 {
		return fail(fmt.Errorf("unsupported database schema version %d", version))
	}
	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		return fail(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS memories (
 id TEXT PRIMARY KEY, position INTEGER NOT NULL, payload TEXT NOT NULL CHECK(json_valid(payload))
 );
 CREATE TABLE IF NOT EXISTS records (
 namespace TEXT NOT NULL, key TEXT NOT NULL, payload BLOB NOT NULL,
 PRIMARY KEY(namespace,key)
 );
 CREATE TABLE IF NOT EXISTS migrations (
 id TEXT PRIMARY KEY, completed_at TEXT NOT NULL, manifest TEXT NOT NULL CHECK(json_valid(manifest))
 );
 PRAGMA user_version=1;`); err != nil {
		return fail(err)
	}
	if err = tx.Commit(); err != nil {
		return fail(err)
	}
	return &Database{db: db}, nil
}

// Close 等待连接归还并释放数据库。
func (d *Database) Close() error { return d.db.Close() }

// Check 验证数据库页和外键完整性。
func (d *Database) Check(ctx context.Context) error {
	var result string
	if err := d.db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return fmt.Errorf("database integrity check failed: %s", result)
	}
	rows, err := d.db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return fmt.Errorf("database foreign key check failed")
	}
	return rows.Err()
}
