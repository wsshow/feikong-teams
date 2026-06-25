package ssh

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewClientAndAddr(t *testing.T) {
	client := NewClient("user", "pwd", "127.0.0.1:22")
	if client.user != "user" || client.pwd != "pwd" || client.Addr() != "127.0.0.1:22" {
		t.Fatalf("client = %#v", client)
	}
}

func TestLocalPathHelpers(t *testing.T) {
	client := NewClient("user", "pwd", "127.0.0.1:22")
	dir := filepath.Join(t.TempDir(), "nested", "dir")

	if client.IsLocalPathExist(dir) {
		t.Fatalf("path %s should not exist yet", dir)
	}
	if err := client.NotExistToMkdirLocal(dir); err != nil {
		t.Fatalf("NotExistToMkdirLocal returned error: %v", err)
	}
	if !client.IsLocalPathExist(dir) {
		t.Fatalf("path %s should exist", dir)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("%s should be a dir", dir)
	}
	if err := client.NotExistToMkdirLocal(dir); err != nil {
		t.Fatalf("NotExistToMkdirLocal existing returned error: %v", err)
	}
}

func TestCopyLocalFileToRemoteMissingLocalFile(t *testing.T) {
	client := NewClient("user", "pwd", "127.0.0.1:22")
	n, err := client.CopyLocalFileToRemote(filepath.Join(t.TempDir(), "missing.txt"), "/tmp/remote.txt")
	if err == nil {
		t.Fatalf("CopyLocalFileToRemote n=%d err=nil, want missing file error", n)
	}
}

func TestCopyLocalDirToRemoteMissingLocalPath(t *testing.T) {
	client := NewClient("user", "pwd", "127.0.0.1:22")
	err := client.CopyLocalDirToRemote(filepath.Join(t.TempDir(), "missing"), "/tmp/remote")
	if err == nil {
		t.Fatal("CopyLocalDirToRemote missing path should return error")
	}
}

func TestCloseNilClients(t *testing.T) {
	client := NewClient("user", "pwd", "127.0.0.1:22")
	if err := client.Close(); err != nil {
		t.Fatalf("Close with nil clients returned error: %v", err)
	}
}
