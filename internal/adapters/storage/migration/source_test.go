package migration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBackupRetainsOriginalsAndRejectsCorruptJSON(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "sessions", "one")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "metadata.json")
	original := []byte(`{"id":"one"}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	source := &Source{Root: root}
	snapshot, err := source.Prepare(context.Background(), func(string, int, int) {})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Documents) != 1 {
		t.Fatal("document omitted")
	}
	backup, err := os.ReadFile(filepath.Join(snapshot.BackupPath, "sessions", "one", "metadata.json"))
	if err != nil || string(backup) != string(original) {
		t.Fatal("backup differs")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != string(original) {
		t.Fatal("source changed")
	}
	if err := os.WriteFile(path, []byte(`{"id":`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Prepare(context.Background(), func(string, int, int) {}); err == nil {
		t.Fatal("corrupt JSON accepted")
	}
}
func TestBackupRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sessions"), 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "sessions", "link")); err != nil {
		t.Skip(err)
	}
	if _, err := (&Source{Root: root}).Prepare(context.Background(), func(string, int, int) {}); err == nil {
		t.Fatal("symlink accepted")
	}
}

func TestMigrationRejectsInvalidRuntimeDocuments(t *testing.T) {
	for _, fixture := range []struct{ path, data string }{
		{"sessions/one/metadata.json", `{"id":"another"}`},
		{"sessions/one/queue.json", `{"version":99,"session_id":"one"}`},
		{"sessions/one/queue.json", `{"version":1,"session_id":"one","items":[{"queue_id":"q1","parts":[{"type":"image_url","url":"attachments/queue/missing.png"}]}]}`},
		{"scheduler/scheduled_tasks.json", `{"tasks":[{"id":"../../escape","task":"bad","status":"pending"}]}`},
		{"javascript/state/test.json", `[1,2]`},
	} {
		t.Run(fixture.path, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, filepath.FromSlash(fixture.path))
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(fixture.data), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := (&Source{Root: root}).Prepare(context.Background(), func(string, int, int) {}); err == nil {
				t.Fatal("invalid data accepted")
			}
		})
	}
}
