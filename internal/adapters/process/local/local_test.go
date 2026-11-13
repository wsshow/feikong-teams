package local

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	domain "fkteams/internal/domain/process"
	port "fkteams/internal/ports/process"
	"github.com/google/uuid"
)

func newBackend(t *testing.T) *Backend {
	t.Helper()
	b, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := b.Close(); err != nil {
			t.Error(err)
		}
	})
	return b
}
func id() string { return "proc_" + uuid.NewString() }

func TestCircularLogRetainsLatestBytesAndReportsCursorGap(t *testing.T) {
	b := newBackend(t)
	taskID := id()
	file, err := b.open(taskID, ".log", os.O_CREATE|os.O_RDWR)
	if err != nil {
		t.Fatal(err)
	}
	l := &logFile{file: file}
	writer := streamWriter{log: l}
	first := bytes.Repeat([]byte("a"), logCapacity-3)
	second := []byte("0123456789")
	if _, err := writer.Write(first); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(second); err != nil {
		t.Fatal(err)
	}
	if stat, _ := file.Stat(); stat.Size() > logCapacity+8 {
		t.Fatalf("unbounded log size %d", stat.Size())
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := b.Read(taskID, 0, 0, 12)
	if err != nil || !out.Truncated || out.From != 7 || len(out.Text) != 12 {
		t.Fatalf("gap: %+v %v", out, err)
	}
	out, err = b.Read(taskID, 0, -1, 10)
	if err != nil || out.Text != string(second) || out.Next != int64(len(first)+len(second)) {
		t.Fatalf("tail: %+v %v", out, err)
	}
	if _, err := b.Read(taskID, 0, -2, 32); err == nil {
		t.Fatal("negative cursor accepted")
	}
}

func TestSingleWriteLargerThanRingKeepsTail(t *testing.T) {
	b := newBackend(t)
	taskID := id()
	file, err := b.open(taskID, ".log", os.O_CREATE|os.O_RDWR)
	if err != nil {
		t.Fatal(err)
	}
	l := &logFile{file: file}
	writer := streamWriter{log: l}
	data := append(bytes.Repeat([]byte("x"), 2*logCapacity+17), []byte("latest")...)
	if _, err := writer.Write(data); err != nil {
		t.Fatal(err)
	}
	file.Close()
	out, err := b.Read(taskID, 0, -1, 6)
	if err != nil || out.Text != "latest" {
		t.Fatalf("tail: %+v %v", out, err)
	}
	if len(l.stdout) > tailCapacity {
		t.Fatalf("unbounded memory tail %d", len(l.stdout))
	}
}

func TestTaskStorageRejectsEscapesAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	b, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := b.Save(domain.Record{ID: "../outside"}); err == nil {
		t.Fatal("record escape")
	}
	if _, err := b.Read("../outside", 0, 0, 32); err == nil {
		t.Fatal("log escape")
	}
	if runtime.GOOS == "windows" {
		return
	}
	taskID := id()
	if err := os.Symlink(outside, filepath.Join(dir, taskID+".log")); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Read(taskID, 0, 0, 32); err == nil {
		t.Fatal("log symlink escape")
	}
	data, _ := os.ReadFile(outside)
	if string(data) != "keep" {
		t.Fatal("external file modified")
	}
}

func TestGracefulStopPreservesTrapOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix signal fixture")
	}
	b := newBackend(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, err := b.Start(ctx, port.Spec{ID: id(), Directory: t.TempDir(), Command: "trap 'printf stopped; exit 0' TERM; printf ready; while :; do sleep 0.05; done"})
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan port.Result, 1)
	go func() { finished <- p.Wait() }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		out, err := p.Read(0, 100)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.Text, "ready") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("not ready")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case result := <-finished:
		if !strings.Contains(result.Stdout, "stopped") {
			t.Fatalf("trap did not flush output: %+v", result)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("stop did not drain")
	}
	if _, err := p.Read(0, 100); err != nil {
		t.Fatalf("read after process exit: %v", err)
	}
}

func TestStopEscalatesWhenProcessIgnoresTERM(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix signal fixture")
	}
	b := newBackend(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, err := b.Start(ctx, port.Spec{ID: id(), Directory: t.TempDir(), Command: "trap '' TERM; printf ready; sleep 20"})
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan port.Result, 1)
	go func() { finished <- p.Wait() }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		out, err := p.Read(0, 100)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.Text, "ready") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("not ready")
		}
		time.Sleep(5 * time.Millisecond)
	}
	started := time.Now()
	cancel()
	select {
	case result := <-finished:
		if result.ExitCode == 0 || time.Since(started) < 2800*time.Millisecond {
			t.Fatalf("did not use escalation grace period: %+v after %s", result, time.Since(started))
		}
	case <-time.After(7 * time.Second):
		t.Fatal("ignored TERM without escalation")
	}
}

func TestCursorPaginationPreservesUTF8AcrossPages(t *testing.T) {
	b := newBackend(t)
	taskID := id()
	file, err := b.open(taskID, ".log", os.O_CREATE|os.O_RDWR)
	if err != nil {
		t.Fatal(err)
	}
	text := "中文日志读取"
	writer := streamWriter{log: &logFile{file: file}}
	if _, err := writer.Write([]byte(text)); err != nil {
		t.Fatal(err)
	}
	file.Close()
	var output strings.Builder
	cursor := int64(0)
	for cursor < int64(len(text)) {
		page, err := b.Read(taskID, 0, cursor, 5)
		if err != nil {
			t.Fatal(err)
		}
		if page.Next <= cursor {
			t.Fatal("cursor did not advance")
		}
		output.WriteString(page.Text)
		cursor = page.Next
	}
	if output.String() != text {
		t.Fatalf("UTF-8 pagination lost bytes: %q", output.String())
	}
}
