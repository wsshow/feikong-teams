package task

import (
	"context"
	"path/filepath"
	"testing"

	"fkteams/internal/adapters/storage/sqlite"
	apptask "fkteams/internal/app/task"
	domaintask "fkteams/internal/domain/task"
)

func TestTaskStoreRestartsAndRejectsOutsideKeys(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "data.db")
	db, err := sqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	docs, err := sqlite.NewDocuments(db, root)
	if err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(root, "assistant", "tasks.json")
	service := apptask.NewService(NewStore(docs, key))
	saved, err := service.Save("session", apptask.SaveRequest{Key: "report", Title: "报告", Goal: "写出报告", Acceptance: "可以直接阅读", Status: domaintask.Pending})
	if err != nil {
		t.Fatal(err)
	}
	if err := NewStore(docs, filepath.Join(root, "..", "outside.json")).Save([]domaintask.Task{saved}); err == nil {
		t.Fatal("outside key accepted")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	docs, err = sqlite.NewDocuments(db, root)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := apptask.NewService(NewStore(docs, key)).Query(apptask.QueryRequest{ID: saved.ID})
	if err != nil || len(loaded.Tasks) != 1 || loaded.Tasks[0].Acceptance != "可以直接阅读" {
		t.Fatalf("restart: %+v %v", loaded, err)
	}
	if err := docs.Write(key, []byte("invalid")); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(docs, key).Load(); err == nil {
		t.Fatal("corrupt snapshot treated as empty")
	}
}
