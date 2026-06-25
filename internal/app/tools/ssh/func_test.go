package ssh

import (
	"context"
	"strings"
	"testing"
)

func TestNewSSHToolsValidationAndConnectError(t *testing.T) {
	if tools, err := NewSSHTools("", "user", "pwd"); err == nil || tools != nil {
		t.Fatalf("NewSSHTools missing host tools=%#v err=%v, want error", tools, err)
	}
	if tools, err := NewSSHTools("bad-address", "user", "pwd"); err == nil || tools != nil || !strings.Contains(err.Error(), "SSH 连接失败") {
		t.Fatalf("NewSSHTools connect tools=%#v err=%v, want connection error", tools, err)
	}
}

func TestSSHToolsCloseNil(t *testing.T) {
	var tools *SSHTools
	tools.Close()
	(&SSHTools{}).Close()
}

func TestSSHExecuteValidation(t *testing.T) {
	var nilTools *SSHTools
	resp, err := nilTools.SSHExecute(context.Background(), &SSHExecuteRequest{Command: "echo ok"})
	if err != nil {
		t.Fatalf("SSHExecute nil returned error: %v", err)
	}
	if !strings.Contains(resp.ErrorMessage, "未初始化") {
		t.Fatalf("nil response = %#v", resp)
	}

	tools := &SSHTools{client: NewClient("user", "pwd", "127.0.0.1:22")}
	resp, err = tools.SSHExecute(context.Background(), &SSHExecuteRequest{})
	if err != nil || !strings.Contains(resp.ErrorMessage, "command") {
		t.Fatalf("empty command resp=%#v err=%v", resp, err)
	}
	resp, err = tools.SSHExecute(context.Background(), &SSHExecuteRequest{Command: "rm -rf /tmp/foo"})
	if err != nil || !strings.Contains(resp.ErrorMessage, "危险命令") {
		t.Fatalf("dangerous command resp=%#v err=%v", resp, err)
	}
	resp, err = tools.SSHExecute(context.Background(), &SSHExecuteRequest{Command: "echo ok", Timeout: 301})
	if err != nil || !strings.Contains(resp.ErrorMessage, "不能超过 300 秒") {
		t.Fatalf("timeout resp=%#v err=%v", resp, err)
	}
}

func TestSSHFileAndListValidation(t *testing.T) {
	var nilTools *SSHTools

	uploadResp, err := nilTools.SSHFileUpload(context.Background(), &SSHFileUploadRequest{LocalPath: "a", RemotePath: "b"})
	if err != nil || !strings.Contains(uploadResp.ErrorMessage, "未初始化") {
		t.Fatalf("nil upload resp=%#v err=%v", uploadResp, err)
	}
	downloadResp, err := nilTools.SSHFileDownload(context.Background(), &SSHFileDownloadRequest{RemotePath: "a", LocalPath: "b"})
	if err != nil || !strings.Contains(downloadResp.ErrorMessage, "未初始化") {
		t.Fatalf("nil download resp=%#v err=%v", downloadResp, err)
	}
	listResp, err := nilTools.SSHListDir(context.Background(), &SSHListDirRequest{RemotePath: "/"})
	if err != nil || !strings.Contains(listResp.ErrorMessage, "未初始化") {
		t.Fatalf("nil list resp=%#v err=%v", listResp, err)
	}

	tools := &SSHTools{client: NewClient("user", "pwd", "127.0.0.1:22")}
	uploadResp, err = tools.SSHFileUpload(context.Background(), &SSHFileUploadRequest{})
	if err != nil || !strings.Contains(uploadResp.ErrorMessage, "local_path") {
		t.Fatalf("empty upload resp=%#v err=%v", uploadResp, err)
	}
	downloadResp, err = tools.SSHFileDownload(context.Background(), &SSHFileDownloadRequest{})
	if err != nil || !strings.Contains(downloadResp.ErrorMessage, "remote_path") {
		t.Fatalf("empty download resp=%#v err=%v", downloadResp, err)
	}
	listResp, err = tools.SSHListDir(context.Background(), &SSHListDirRequest{})
	if err != nil || !strings.Contains(listResp.ErrorMessage, "remote_path") {
		t.Fatalf("empty list resp=%#v err=%v", listResp, err)
	}

	uploadResp, err = tools.SSHFileUpload(context.Background(), &SSHFileUploadRequest{LocalPath: "/missing/local", RemotePath: "/tmp/remote"})
	if err != nil || !strings.Contains(uploadResp.ErrorMessage, "文件上传失败") {
		t.Fatalf("missing local upload resp=%#v err=%v", uploadResp, err)
	}
}
