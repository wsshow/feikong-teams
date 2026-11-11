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
