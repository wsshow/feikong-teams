package ssh

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

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
		return err
	}
	c.sftpClient = sftpClient
	return nil
}

func (c *SshClient) Close() (err error) {
	if c.sftpClient != nil {
		err = c.sftpClient.Close()
	}
	if c.sshClient != nil {
		err = c.sshClient.Close()
	}
	return err
}

func (c *SshClient) Run(shell string) ([]byte, error) {
	session, err := c.sshClient.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()
	return session.CombinedOutput(shell)
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
