package file

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditPreservesUntouchedBytes(t *testing.T) {
	ft, base := newTestFileTools(t)
	for _, content := range []string{"old\n\n", "old\r\nkeep\r\n", "old\n\n\n", "old", "old\r\n\r\n"} {
		path := filepath.Join(base, "edit.txt")
		if err := os.WriteFile(path, []byte(content), 0750); err != nil {
			t.Fatal(err)
		}
		resp, err := ft.FileEdit(context.Background(), &FileEditRequest{Filepath: "edit.txt", OldString: "old", NewString: "new"})
		if err != nil || resp.ErrorMessage != "" {
			t.Fatalf("edit: %#v %v", resp, err)
		}
		got, _ := os.ReadFile(path)
		if string(got) != strings.Replace(content, "old", "new", 1) {
			t.Fatalf("bytes changed: %q", got)
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0750 {
			t.Fatalf("permissions changed: %v %v", info, err)
		}
	}
}
func TestGrepContextKeepsAllMatchesAndLineNumbers(t *testing.T) {
	ft, base := newTestFileTools(t)
	if err := os.WriteFile(filepath.Join(base, "match.txt"), []byte("hit1\nhit2\nhit3\nhit4\n"), 0600); err != nil {
		t.Fatal(err)
	}
	resp, err := ft.Grep(context.Background(), &GrepRequest{Path: "match.txt", Pattern: "hit", Context: 1})
	if err != nil || len(resp.Matches) != 4 {
		t.Fatalf("matches: %#v %v", resp, err)
	}
	for i, match := range resp.Matches {
		if match.LineNumber != i+1 {
			t.Fatalf("wrong line: %#v", match)
		}
	}
	if !strings.Contains(resp.Matches[1].Context, "1: hit1") || !strings.Contains(resp.Matches[1].Context, "3: hit3") {
		t.Fatalf("wrong context: %#v", resp.Matches[1])
	}
}
func TestRecursiveGrepCannotReadOutsideSymlink(t *testing.T) {
	ft, base := newTestFileTools(t)
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("private-marker"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(base, "link.txt")); err != nil {
		t.Skip(err)
	}
	resp, err := ft.Grep(context.Background(), &GrepRequest{Path: ".", Pattern: "private-marker"})
	if err != nil || len(resp.Matches) != 0 {
		t.Fatalf("outside content leaked: %#v %v", resp, err)
	}
}
