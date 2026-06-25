package file

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestFileTools(t *testing.T) (*FileTools, string) {
	t.Helper()
	base := t.TempDir()
	ft, err := NewFileTools(base)
	if err != nil {
		t.Fatalf("NewFileTools failed: %v", err)
	}
	return ft, base
}

func TestFileWriteReadAppendAndList(t *testing.T) {
	ft, base := newTestFileTools(t)
	ctx := context.Background()

	writeResp, err := ft.FileWrite(ctx, &FileWriteRequest{
		Filepath: filepath.Join("notes", "todo.txt"),
		Content:  "alpha\nbeta\n",
	})
	if err != nil {
		t.Fatalf("FileWrite returned error: %v", err)
	}
	if writeResp.ErrorMessage != "" || writeResp.TotalLines != 2 {
		t.Fatalf("FileWrite = %#v, want two lines", writeResp)
	}

	appendResp, err := ft.FileAppend(ctx, &FileAppendRequest{
		Filepath: filepath.Join("notes", "todo.txt"),
		Content:  "gamma\n",
	})
	if err != nil {
		t.Fatalf("FileAppend returned error: %v", err)
	}
	if appendResp.ErrorMessage != "" || appendResp.TotalLines != 1 {
		t.Fatalf("FileAppend = %#v, want one appended line", appendResp)
	}

	readResp, err := ft.FileRead(ctx, &FileReadRequest{
		Filepath:  filepath.Join("notes", "todo.txt"),
		StartLine: 2,
		EndLine:   3,
	})
	if err != nil {
		t.Fatalf("FileRead returned error: %v", err)
	}
	if readResp.ErrorMessage != "" || readResp.Content != "beta\ngamma" || readResp.ReadRange != "2-3" {
		t.Fatalf("FileRead = %#v, want lines 2-3", readResp)
	}

	listResp, err := ft.FileList(ctx, &FileListRequest{Dirpath: "notes"})
	if err != nil {
		t.Fatalf("FileList returned error: %v", err)
	}
	if listResp.ErrorMessage != "" || !strings.Contains(listResp.Content, "todo.txt") {
		t.Fatalf("FileList = %#v, want todo.txt", listResp)
	}

	if _, err := os.Stat(filepath.Join(base, "notes", "todo.txt")); err != nil {
		t.Fatalf("written file not found in workspace: %v", err)
	}
}

func TestGrepGlobAndEdit(t *testing.T) {
	ft, _ := newTestFileTools(t)
	ctx := context.Background()

	files := map[string]string{
		filepath.Join("src", "main.go"):   "package main\nfunc main() {\n\tprintln(\"target\")\n}\n",
		filepath.Join("src", "readme.md"): "# target\nplain text\n",
		filepath.Join("docs", "note.txt"): "target in docs\n",
	}
	for path, content := range files {
		if resp, err := ft.FileWrite(ctx, &FileWriteRequest{Filepath: path, Content: content}); err != nil || resp.ErrorMessage != "" {
			t.Fatalf("FileWrite(%s) resp=%#v err=%v", path, resp, err)
		}
	}

	globResp, err := ft.Glob(ctx, &GlobRequest{Path: "src", Pattern: "**/*.go"})
	if err != nil {
		t.Fatalf("Glob returned error: %v", err)
	}
	if globResp.ErrorMessage != "" || globResp.TotalFiles != 1 || filepath.ToSlash(globResp.Files[0]) != "src/main.go" {
		t.Fatalf("Glob = %#v, want src/main.go", globResp)
	}

	grepResp, err := ft.Grep(ctx, &GrepRequest{Path: ".", Pattern: "target", Include: "*.go", Context: 1})
	if err != nil {
		t.Fatalf("Grep returned error: %v", err)
	}
	if grepResp.ErrorMessage != "" || grepResp.TotalMatches != 1 {
		t.Fatalf("Grep = %#v, want one Go match", grepResp)
	}
	if !strings.Contains(grepResp.Matches[0].Context, "func main") {
		t.Fatalf("grep context = %q, want previous context", grepResp.Matches[0].Context)
	}

	editResp, err := ft.FileEdit(ctx, &FileEditRequest{
		Filepath:  filepath.Join("src", "main.go"),
		OldString: "target",
		NewString: "updated",
	})
	if err != nil {
		t.Fatalf("FileEdit returned error: %v", err)
	}
	if editResp.ErrorMessage != "" {
		t.Fatalf("FileEdit failed: %#v", editResp)
	}
	readResp, err := ft.FileRead(ctx, &FileReadRequest{Filepath: filepath.Join("src", "main.go")})
	if err != nil {
		t.Fatalf("FileRead returned error: %v", err)
	}
	if !strings.Contains(readResp.Content, "updated") {
		t.Fatalf("content = %q, want updated text", readResp.Content)
	}
}

func TestFileReadValidation(t *testing.T) {
	ft, _ := newTestFileTools(t)
	ctx := context.Background()

	if resp, err := ft.FileRead(ctx, &FileReadRequest{}); err != nil || !strings.Contains(resp.ErrorMessage, "filepath") {
		t.Fatalf("FileRead missing path resp=%#v err=%v", resp, err)
	}

	if resp, err := ft.FileRead(ctx, &FileReadRequest{Filepath: "missing.txt"}); err != nil || !strings.Contains(resp.ErrorMessage, "读取文件失败") {
		t.Fatalf("FileRead missing file resp=%#v err=%v", resp, err)
	}

	if resp, err := ft.FileWrite(ctx, &FileWriteRequest{Filepath: "big.txt", Content: strings.Repeat("x\n", 3)}); err != nil || resp.ErrorMessage != "" {
		t.Fatalf("FileWrite resp=%#v err=%v", resp, err)
	}
	if resp, err := ft.FileRead(ctx, &FileReadRequest{Filepath: "big.txt", StartLine: 10}); err != nil || !strings.Contains(resp.ErrorMessage, "超出文件总行数") {
		t.Fatalf("FileRead out of range resp=%#v err=%v", resp, err)
	}
	if resp, err := ft.FileRead(ctx, &FileReadRequest{Filepath: "big.txt", StartLine: 1, EndLine: maxReadLines + 1}); err != nil || !strings.Contains(resp.ErrorMessage, "读取范围过大") {
		t.Fatalf("FileRead large range resp=%#v err=%v", resp, err)
	}
}
