package ssh

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestNewClientAndAddr(t *testing.T) {
	client := NewClient("user", "pwd", "127.0.0.1:22", WithKnownHostsFile("/tmp/known_hosts"), WithWorkDir("/tmp/work"))
	if client.user != "user" || client.pwd != "pwd" || client.Addr() != "127.0.0.1:22" || client.knownHosts != "/tmp/known_hosts" || client.workDir != "/tmp/work" {
		t.Fatalf("client = %#v", client)
	}
	if strings.Contains(client.String(), "pwd") {
		t.Fatalf("client string exposes password: %s", client)
	}
}

func TestResolveKnownHostsPathAndLimits(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path, err := resolveKnownHostsPath("~/.ssh/custom_hosts")
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(home, ".ssh", "custom_hosts") {
		t.Fatalf("resolved path = %q", path)
	}
	large := filepath.Join(t.TempDir(), "known_hosts")
	file, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxKnownHostsBytes + 1); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := loadHostKeyCallback(large); err == nil {
		t.Fatal("loadHostKeyCallback accepted oversized file")
	}
}

func TestLimitedSSHOutputEnforcesCombinedLimit(t *testing.T) {
	output := &limitedSSHOutput{remaining: 8}
	var wg sync.WaitGroup
	for _, value := range []string{"12345", "abcde"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := output.Write([]byte(value)); err != nil {
				t.Errorf("Write() error = %v", err)
			}
		}()
	}
	wg.Wait()
	data, truncated := output.result()
	if text := string(data); !truncated || (text != "12345abc" && text != "abcde123") {
		t.Fatalf("data = %q, truncated = %v", data, truncated)
	}
}

func TestResolveLocalTargetRejectsWorkspaceEscape(t *testing.T) {
	workspace := t.TempDir()
	client := NewClient("user", "pwd", "127.0.0.1:22", WithWorkDir(workspace))
	if _, root, err := client.resolveLocalTarget("../outside.txt"); err == nil {
		if root != nil {
			root.Close()
		}
		t.Fatal("resolveLocalTarget accepted workspace escape")
	}
	resolved, root, err := client.resolveLocalTarget("nested/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if resolved.AbsPath != filepath.Join(workspace, "nested", "file.txt") {
		t.Fatalf("resolved path = %q", resolved.AbsPath)
	}
}

func TestSSHContextReaderStopsAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reader := &sshContextReader{ctx: ctx, reader: strings.NewReader("data")}
	if _, err := reader.Read(make([]byte, 4)); !errors.Is(err, context.Canceled) {
		t.Fatalf("Read() error = %v, want context canceled", err)
	}
}

func TestCopyLocalFileToRemoteMissingLocalFile(t *testing.T) {
	client := NewClient("user", "pwd", "127.0.0.1:22")
	n, err := client.CopyLocalFileToRemote(context.Background(), filepath.Join(t.TempDir(), "missing.txt"), "/tmp/remote.txt")
	if err == nil {
		t.Fatalf("CopyLocalFileToRemote n=%d err=nil, want missing file error", n)
	}
}

func TestCloseNilClients(t *testing.T) {
	client := NewClient("user", "pwd", "127.0.0.1:22")
	if err := client.Close(); err != nil {
		t.Fatalf("Close with nil clients returned error: %v", err)
	}
}
