package sqlite

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"sync"
	"testing"
)

func TestDocumentsBoundariesAndAtomicAppend(t *testing.T) {
	root := t.TempDir()
	db, err := Open(context.Background(), filepath.Join(root, "data", "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	documents, err := NewDocuments(db, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := documents.Write(filepath.Join(root, "..", "outside"), []byte("bad")); err == nil {
		t.Fatal("escape accepted")
	}
	path := filepath.Join(root, "会话_%", "transcript.jsonl")
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := documents.Append(path, []byte("record\n")); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	data, err := documents.Read(path)
	if err != nil || len(data) != 30*7 {
		t.Fatalf("append: %d %v", len(data), err)
	}
	paths, err := documents.List(filepath.Dir(path))
	if err != nil || len(paths) != 1 {
		t.Fatalf("list: %v %v", paths, err)
	}
	if err := documents.Write(path, []byte("replaced")); err != nil {
		t.Fatal(err)
	}
	data, err = documents.Read(path)
	if err != nil || string(data) != "replaced" {
		t.Fatal("replace retained fragments")
	}
	if err := documents.RemoveTree(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	if _, err := documents.Read(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestRemoveTreePreservesDocumentsAndRollsBack(t *testing.T) {
	root := t.TempDir()
	db, err := Open(context.Background(), filepath.Join(root, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	documents, err := NewDocuments(db, root)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "session_%")
	metadata := filepath.Join(dir, "metadata.json")
	history := filepath.Join(dir, "transcript.jsonl")
	other := filepath.Join(root, "session_%other", "metadata.json")
	for _, path := range []string{metadata, history, other} {
		if err := documents.Append(path, []byte("saved")); err != nil {
			t.Fatal(err)
		}
	}
	for _, preserved := range []string{other, filepath.Join(root, "..", "outside")} {
		if err := documents.RemoveTree(dir, preserved); err == nil {
			t.Fatal("accepted invalid preserved path")
		}
	}
	if _, err := db.db.Exec(`CREATE TRIGGER reject_clear BEFORE DELETE ON records BEGIN SELECT RAISE(ABORT, 'unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if err := documents.RemoveTree(dir, metadata); err == nil {
		t.Fatal("acknowledged failed transaction")
	}
	if data, err := documents.Read(history); err != nil || string(data) != "saved" {
		t.Fatalf("rollback = %q, %v", data, err)
	}
	if _, err := db.db.Exec("DROP TRIGGER reject_clear"); err != nil {
		t.Fatal(err)
	}
	if err := documents.RemoveTree(dir, metadata); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{metadata, other} {
		if data, err := documents.Read(path); err != nil || string(data) != "saved" {
			t.Fatalf("preserved = %q, %v", data, err)
		}
	}
	if _, err := documents.Read(history); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("history retained: %v", err)
	}
}
