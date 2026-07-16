package ssh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fkteams/internal/runtime/atomicfile"
	"fkteams/internal/runtime/pathguard"

	"github.com/google/uuid"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const (
	maxKnownHostsBytes  int64 = 16 << 20
	maxSSHTransferBytes       = 1 << 30
)

type ClientOption func(*SshClient)

// WithKnownHostsFile 设置主机密钥数据库路径。
func WithKnownHostsFile(path string) ClientOption {
	return func(client *SshClient) { client.knownHosts = path }
}

// WithWorkDir 设置本地文件传输允许访问的工作区。
func WithWorkDir(path string) ClientOption {
	return func(client *SshClient) { client.workDir = path }
}

type SshClient struct {
	user       string
	pwd        string
	addr       string
	knownHosts string
	workDir    string
	sshClient  *ssh.Client
	sftpClient *sftp.Client
}

func NewClient(user, pwd, addr string, options ...ClientOption) *SshClient {
	workDir, _ := os.Getwd()
	client := &SshClient{
		user:    user,
		pwd:     pwd,
		addr:    addr,
		workDir: workDir,
	}
	for _, option := range options {
		if option != nil {
			option(client)
		}
	}
	return client
}

func (c *SshClient) Addr() string {
	return c.addr
}

func (c *SshClient) String() string {
	if c == nil {
		return "SshClient<nil>"
	}
	return fmt.Sprintf("SshClient{user:%q, addr:%q}", c.user, c.addr)
}

func (c *SshClient) Connect() error {
	defer func() { c.pwd = "" }()
	hostKeyCallback, err := loadHostKeyCallback(c.knownHosts)
	if err != nil {
		return err
	}
	config := &ssh.ClientConfig{
		User: c.user,
		Auth: []ssh.AuthMethod{
			ssh.Password(c.pwd),
		},
		HostKeyCallback: hostKeyCallback,
		Timeout:         15 * time.Second,
	}

	sshClient, err := ssh.Dial("tcp", c.addr, config)
	if err != nil {
		return err
	}
	c.sshClient = sshClient

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		closeErr := sshClient.Close()
		return errors.Join(err, closeErr)
	}
	c.sftpClient = sftpClient
	return nil
}

func loadHostKeyCallback(path string) (ssh.HostKeyCallback, error) {
	resolvedPath, err := resolveKnownHostsPath(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("inspect known hosts file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("known hosts path is not a regular file")
	}
	if info.Size() > maxKnownHostsBytes {
		return nil, fmt.Errorf("known hosts file exceeds %d bytes", maxKnownHostsBytes)
	}
	callback, err := knownhosts.New(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("load known hosts file: %w", err)
	}
	return callback, nil
}

func resolveKnownHostsPath(path string) (string, error) {
	if path == "" || path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home directory: %w", err)
		}
		switch {
		case path == "" || path == "~":
			path = filepath.Join(home, ".ssh", "known_hosts")
		default:
			path = filepath.Join(home, path[2:])
		}
	} else if strings.HasPrefix(path, "~") {
		return "", fmt.Errorf("unsupported home directory shorthand in known hosts path")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve known hosts path: %w", err)
	}
	return absPath, nil
}

func (c *SshClient) Close() error {
	var closeErrors []error
	if c.sftpClient != nil {
		closeErrors = append(closeErrors, c.sftpClient.Close())
		c.sftpClient = nil
	}
	if c.sshClient != nil {
		closeErrors = append(closeErrors, c.sshClient.Close())
		c.sshClient = nil
	}
	return errors.Join(closeErrors...)
}

func (c *SshClient) RunContext(ctx context.Context, shell string, outputLimit int64) ([]byte, bool, error) {
	if c == nil || c.sshClient == nil {
		return nil, false, fmt.Errorf("SSH client is not connected")
	}
	if outputLimit < 0 {
		return nil, false, fmt.Errorf("SSH output limit must not be negative")
	}
	session, err := c.sshClient.NewSession()
	if err != nil {
		return nil, false, err
	}
	defer session.Close()
	output := &limitedSSHOutput{remaining: outputLimit}
	session.Stdout = output
	session.Stderr = output
	if err := session.Start(shell); err != nil {
		return nil, false, err
	}
	done := make(chan error, 1)
	go func() {
		done <- session.Wait()
	}()
	select {
	case err := <-done:
		data, truncated := output.result()
		return data, truncated, err
	case <-ctx.Done():
		_ = session.Close()
		data, truncated := output.result()
		return data, truncated, ctx.Err()
	}
}

type limitedSSHOutput struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	remaining int64
	truncated bool
}

func (output *limitedSSHOutput) Write(data []byte) (int, error) {
	output.mu.Lock()
	defer output.mu.Unlock()
	originalLength := len(data)
	if int64(len(data)) > output.remaining {
		data = data[:output.remaining]
		output.truncated = true
	}
	if len(data) > 0 {
		_, _ = output.buffer.Write(data)
		output.remaining -= int64(len(data))
	}
	return originalLength, nil
}

func (output *limitedSSHOutput) result() ([]byte, bool) {
	output.mu.Lock()
	defer output.mu.Unlock()
	return bytes.Clone(output.buffer.Bytes()), output.truncated
}

func (c *SshClient) CopyRemoteFileToLocal(ctx context.Context, remotePath, localPath string) (int64, error) {
	if c == nil || c.sftpClient == nil {
		return 0, fmt.Errorf("SFTP client is not connected")
	}
	source, err := c.sftpClient.Open(remotePath)
	if err != nil {
		return 0, fmt.Errorf("sftp client open file error: %w", err)
	}
	info, err := source.Stat()
	if err != nil {
		source.Close()
		return 0, fmt.Errorf("stat remote file error: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxSSHTransferBytes {
		source.Close()
		return 0, fmt.Errorf("remote file exceeds transfer limits")
	}
	resolved, root, err := c.resolveLocalTarget(localPath)
	if err != nil {
		source.Close()
		return 0, err
	}
	reader := &sshContextReader{ctx: ctx, reader: source}
	written, writeErr := atomicfile.WriteReaderInRoot(root, resolved.RelPath, reader, maxSSHTransferBytes, 0644)
	closeErr := source.Close()
	rootCloseErr := root.Close()
	if writeErr != nil {
		return written, fmt.Errorf("save local file: %w", errors.Join(writeErr, closeErr, rootCloseErr))
	}
	if closeErr != nil {
		return written, fmt.Errorf("close remote file: %w", closeErr)
	}
	if rootCloseErr != nil {
		return written, fmt.Errorf("close workspace root: %w", rootCloseErr)
	}
	return written, nil
}

func (c *SshClient) CopyLocalFileToRemote(ctx context.Context, localPath, remotePath string) (int64, error) {
	if c == nil || c.sftpClient == nil {
		return 0, fmt.Errorf("SFTP client is not connected")
	}
	resolved, err := pathguard.ResolveWorkspace(c.workDir, localPath)
	if err != nil {
		return 0, fmt.Errorf("resolve local file: %w", err)
	}
	info, err := os.Stat(resolved.AbsPath)
	if err != nil {
		return 0, fmt.Errorf("stat local file: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxSSHTransferBytes {
		return 0, fmt.Errorf("local file exceeds transfer limits")
	}
	source, err := os.Open(resolved.AbsPath)
	if err != nil {
		return 0, fmt.Errorf("open local file error: %w", err)
	}
	defer source.Close()

	temporaryPath := remotePath + ".fkteams-upload-" + uuid.NewString()
	target, err := c.sftpClient.OpenFile(temporaryPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return 0, fmt.Errorf("create remote temporary file: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = c.sftpClient.Remove(temporaryPath)
		}
	}()
	reader := &sshContextReader{ctx: ctx, reader: source}
	written, copyErr := io.Copy(target, io.LimitReader(reader, maxSSHTransferBytes+1))
	if copyErr == nil && written > maxSSHTransferBytes {
		copyErr = fmt.Errorf("local file exceeds transfer limits")
	}
	if copyErr == nil {
		copyErr = target.Chmod(info.Mode().Perm())
	}
	if copyErr == nil {
		copyErr = target.Sync()
	}
	closeErr := target.Close()
	if copyErr != nil {
		return written, fmt.Errorf("upload remote file: %w", copyErr)
	}
	if closeErr != nil {
		return written, fmt.Errorf("close remote temporary file: %w", closeErr)
	}
	if _, ok := c.sftpClient.HasExtension("posix-rename@openssh.com"); ok {
		err = c.sftpClient.PosixRename(temporaryPath, remotePath)
	} else {
		err = c.sftpClient.Rename(temporaryPath, remotePath)
	}
	if err != nil {
		return written, fmt.Errorf("replace remote file: %w", err)
	}
	cleanup = false
	return written, nil
}

func (c *SshClient) resolveLocalTarget(localPath string) (pathguard.ResolvedPath, *os.Root, error) {
	resolved, err := pathguard.ResolveWorkspace(c.workDir, localPath)
	if err != nil {
		return pathguard.ResolvedPath{}, nil, fmt.Errorf("resolve local file: %w", err)
	}
	root, err := os.OpenRoot(resolved.BaseAbs)
	if err != nil {
		return pathguard.ResolvedPath{}, nil, fmt.Errorf("open workspace root: %w", err)
	}
	parent := filepath.Dir(resolved.RelPath)
	if parent != "." {
		if err := pathguard.EnsureRootDirectory(root, parent, 0755); err != nil {
			root.Close()
			return pathguard.ResolvedPath{}, nil, fmt.Errorf("create local directory: %w", err)
		}
	}
	return resolved, root, nil
}

type sshContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *sshContextReader) Read(data []byte) (int, error) {
	select {
	case <-reader.ctx.Done():
		return 0, reader.ctx.Err()
	default:
		return reader.reader.Read(data)
	}
}

func (c *SshClient) ReadRemoteDir(remotePath string) (fileNameList []string, err error) {
	if c == nil || c.sftpClient == nil {
		return nil, fmt.Errorf("SFTP client is not connected")
	}
	fis, err := c.sftpClient.ReadDir(remotePath)
	if err != nil {
		return fileNameList, err
	}
	for _, fi := range fis {
		fileNameList = append(fileNameList, fi.Name())
	}
	return
}
