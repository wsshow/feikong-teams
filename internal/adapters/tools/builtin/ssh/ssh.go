package ssh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type SshClient struct {
	user       string
	pwd        string
	addr       string
	sshClient  *ssh.Client
	sftpClient *sftp.Client
}

func NewClient(user, pwd, addr string) *SshClient {
	return &SshClient{
		user: user,
		pwd:  pwd,
		addr: addr,
	}
}

func (c *SshClient) Addr() string {
	return c.addr
}

func (c *SshClient) Connect() error {
	config := &ssh.ClientConfig{
		User: c.user,
		Auth: []ssh.AuthMethod{
			ssh.Password(c.pwd),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
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

func (c *SshClient) IsRemotePathExist(remotePath string) bool {
	_, err := c.sftpClient.Stat(remotePath)
	return err == nil
}

func (c *SshClient) NotExistToMkdirRemote(remotePath string) error {
	if c.IsRemotePathExist(remotePath) {
		return nil
	}
	err := c.sftpClient.MkdirAll(remotePath)
	if err != nil {
		return err
	}
	return c.sftpClient.Chmod(remotePath, 0755)
}

func (c *SshClient) NotExistToMkdirLocal(localPath string) error {
	if c.IsLocalPathExist(localPath) {
		return nil
	}
	err := os.MkdirAll(localPath, 0755)
	if err != nil {
		return err
	}
	return os.Chmod(localPath, 0755)
}

func (c *SshClient) CopyRemoteFileToLocal(remotePath, localPath string) (int64, error) {
	if !c.IsRemotePathExist(remotePath) {
		return 0, fmt.Errorf("remote file not exist: %s", remotePath)
	}
	source, err := c.sftpClient.Open(remotePath)
	if err != nil {
		return 0, fmt.Errorf("sftp client open file error: %w", err)
	}
	defer source.Close()

	target, err := os.OpenFile(localPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return 0, fmt.Errorf("open local file error: %w", err)
	}
	defer target.Close()

	n, err := io.Copy(target, source)
	if err != nil {
		return 0, fmt.Errorf("copy file error: %w", err)
	}
	return n, nil
}

func (c *SshClient) IsLocalPathExist(localPath string) bool {
	_, err := os.Stat(localPath)
	if err == nil {
		return true // 文件存在
	}
	if errors.Is(err, os.ErrNotExist) {
		return false // 文件明确不存在
	}
	return false
}

func (c *SshClient) CopyLocalFileToRemote(localPath, remotePath string) (int64, error) {
	if !c.IsLocalPathExist(localPath) {
		return 0, fmt.Errorf("local file not exist: %s", localPath)
	}
	source, err := os.Open(localPath)
	if err != nil {
		return 0, fmt.Errorf("open local file error: %w", err)
	}
	defer source.Close()

	target, err := c.sftpClient.Create(remotePath)
	if err != nil {
		return 0, fmt.Errorf("sftp client create file error: %w", err)
	}
	defer target.Close()

	n, err := io.Copy(target, source)
	if err != nil {
		return 0, fmt.Errorf("copy file error: %w", err)
	}
	return n, nil
}

func (c *SshClient) CopyRemoteDirToLocal(remotePath, localPath string) error {
	fis, err := c.sftpClient.ReadDir(remotePath)
	if err != nil {
		return fmt.Errorf("sftp client read dir error: %w", err)
	}
	err = c.NotExistToMkdirLocal(localPath)
	if err != nil {
		return fmt.Errorf("mkdir error: %w", err)
	}
	for _, fi := range fis {
		rp, lp := fmt.Sprintf("%s/%s", remotePath, fi.Name()), filepath.Join(localPath, fi.Name())
		if fi.IsDir() {
			err = c.CopyRemoteDirToLocal(rp, lp)
			if err != nil {
				return fmt.Errorf("copy remote dir to local error: %w", err)
			}
			continue
		}
		_, err = c.CopyRemoteFileToLocal(rp, lp)
		if err != nil {
			return fmt.Errorf("copy remote file to local error: %w", err)
		}
	}
	return nil
}

func (c *SshClient) CopyLocalDirToRemote(localPath, remotePath string) error {
	if !c.IsLocalPathExist(localPath) {
		return fmt.Errorf("local path not exist: %s", localPath)
	}
	err := c.NotExistToMkdirRemote(remotePath)
	if err != nil {
		return err
	}
	de, err := os.ReadDir(localPath)
	if err != nil {
		return err
	}
	for _, fi := range de {
		lp, rp := filepath.Join(localPath, fi.Name()), fmt.Sprintf("%s/%s", remotePath, fi.Name())
		if fi.IsDir() {
			err = c.CopyLocalDirToRemote(lp, rp)
			if err != nil {
				return fmt.Errorf("copy local dir to remote error: %w", err)
			}
			continue
		}
		_, err = c.CopyLocalFileToRemote(lp, rp)
		if err != nil {
			return fmt.Errorf("copy local file to remote error: %w", err)
		}
	}
	return nil
}

func (c *SshClient) ReadRemoteDir(remotePath string) (fileNameList []string, err error) {
	if !c.IsRemotePathExist(remotePath) {
		return fileNameList, fmt.Errorf("remote path not exist: %s", remotePath)
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

func (c *SshClient) RemoveRemoteDir(remotePath string) error {
	return c.sftpClient.RemoveAll(remotePath)
}
