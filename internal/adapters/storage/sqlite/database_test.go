package sqlite

import (
	"context"
	domainmemory "fkteams/internal/domain/memory"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestMemorySnapshotRollbackAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data ? #", "business.db")
	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	entries := []domainmemory.MemoryEntry{{ID: "stable", Type: domainmemory.Fact, Summary: "信息", Detail: "多行\n正文", Tags: []string{"中文"}, SessionID: "source", CreatedAt: now, HitCount: 3, LastHitAt: &now}}
	store := NewMemoryStore(db)
	if err := store.Save(entries); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(append(entries, entries[0])); err == nil {
		t.Fatal("duplicate snapshot accepted")
	}
	if err := db.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	actual, err := NewMemoryStore(db).Load(500)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, entries) {
		t.Fatalf("snapshot changed: %#v", actual)
	}
}

func TestRejectDatabaseSymlinks(t *testing.T) {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		t.Run(suffix, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "outside")
			if err := os.WriteFile(target, []byte("untouched"), 0600); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "app.db")
			if err := os.Symlink(target, path+suffix); err != nil {
				t.Skip(err)
			}
			if db, err := Open(context.Background(), path); err == nil {
				db.Close()
				t.Fatal("symlink accepted")
			}
			data, err := os.ReadFile(target)
			if err != nil || string(data) != "untouched" {
				t.Fatal("outside file changed")
			}
		})
	}
}

func TestRejectFutureSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec("PRAGMA user_version=99"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if db, err = Open(context.Background(), path); err == nil {
		db.Close()
		t.Fatal("future schema accepted")
	}
}
