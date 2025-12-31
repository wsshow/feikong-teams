package ssh

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

type RemoteShellTool struct {
	Client *SshClient
}

var globalSSHConn *RemoteShellTool

// InitSSHClient 初始化 SSH 连接信息
func InitSSHClient(host, username, password string) error {
	if host == "" || username == "" || password == "" {
		return fmt.Errorf("host, username 和 password 都是必需的")
	}
	globalSSHConn = &RemoteShellTool{
		Client: NewClient(host, username, password),
	}
	globalSSHConn.Client = nil
	return nil
}

// CloseSSHClient 清除 SSH 连接信息
func CloseSSHClient() {
	globalSSHConn = nil
}

// SSHExecuteRequest 执行远程命令请求
type SSHExecuteRequest struct {
	Server  string `json:"server,omitempty" jsonschema:"description=服务器名称，如果不指定则使用默认服务器"`
	Command string `json:"command" jsonschema:"description=要在远程服务器执行的命令,required"`
	Timeout int    `json:"timeout,omitempty" jsonschema:"description=超时时间（秒），默认60秒，最大300秒"`
}

// SSHExecuteResponse 执行远程命令响应
type SSHExecuteResponse struct {
	Output        string `json:"output" jsonschema:"description=命令输出内容（包含 stdout 和 stderr）"`
	ExecutionTime string `json:"execution_time" jsonschema:"description=执行时长"`
	ErrorMessage  string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// SSHExecute 在远程服务器执行命令
func SSHExecute(ctx context.Context, req *SSHExecuteRequest) (*SSHExecuteResponse, error) {
	if globalSSHConn == nil {
		return &SSHExecuteResponse{
			ErrorMessage: "SSH 客户端未初始化，请先设置环境变量 SSH_HOST, SSH_USERNAME, SSH_PASSWORD",
		}, nil
	}

	if req.Command == "" {
		return &SSHExecuteResponse{
			ErrorMessage: "command 参数是必需的",
		}, nil
	}

	// 1. 安全过滤
	if isDangerous(req.Command) {
		return &SSHExecuteResponse{
			ErrorMessage: "命令执行被拒绝：检测到危险命令",
		}, nil
	}

	// 2. 设置超时时间
	timeout := 60 * time.Second
	if req.Timeout > 0 && req.Timeout <= 300 {
		timeout = time.Duration(req.Timeout) * time.Second
	} else if req.Timeout > 300 {
		return &SSHExecuteResponse{
			ErrorMessage: "超时时间不能超过 300 秒",
		}, nil
	}

	// 3. 创建 SSH 连接
	client := globalSSHConn.Client
	if err := client.Connect(); err != nil {
		return &SSHExecuteResponse{
			ErrorMessage: fmt.Sprintf("SSH 连接失败: %v", err),
		}, nil
	}
	defer client.Close()

	// 4. 执行命令并计时
	startTime := time.Now()
	output, err := executeWithTimeout(client, req.Command, timeout)
	executionTime := time.Since(startTime)

	if err != nil {
		return &SSHExecuteResponse{
			Output:        string(output),
			ExecutionTime: executionTime.String(),
			ErrorMessage:  fmt.Sprintf("命令执行失败: %v", err),
		}, nil
	}

	return &SSHExecuteResponse{
		Output:        string(output),
		ExecutionTime: executionTime.String(),
	}, nil
}

// executeWithTimeout 带超时的命令执行
func executeWithTimeout(client *SshClient, cmd string, timeout time.Duration) ([]byte, error) {
	done := make(chan error, 1)
	output := make(chan []byte, 1)

	go func() {
		out, err := client.Run(cmd)
		output <- out
		done <- err
	}()

	select {
	case err := <-done:
		out := <-output
		return out, err
	case <-time.After(timeout):
		return nil, fmt.Errorf("命令执行超时（%v）", timeout)
	}
}

// SSHFileUploadRequest 文件上传请求
type SSHFileUploadRequest struct {
	LocalPath  string `json:"local_path" jsonschema:"description=本地文件路径,required"`
	RemotePath string `json:"remote_path" jsonschema:"description=远程文件路径,required"`
}

// SSHFileUploadResponse 文件上传响应
type SSHFileUploadResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	BytesWritten int64  `json:"bytes_written" jsonschema:"description=写入的字节数"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// SSHFileUpload 上传文件到远程服务器
func SSHFileUpload(ctx context.Context, req *SSHFileUploadRequest) (*SSHFileUploadResponse, error) {
	if globalSSHConn == nil {
		return &SSHFileUploadResponse{
			ErrorMessage: "SSH 客户端未初始化，请先设置环境变量 SSH_HOST, SSH_USERNAME, SSH_PASSWORD",
		}, nil
	}

	if req.LocalPath == "" || req.RemotePath == "" {
		return &SSHFileUploadResponse{
			ErrorMessage: "local_path 和 remote_path 参数都是必需的",
		}, nil
	}

	// 创建 SSH 连接
	client := globalSSHConn.Client
	if err := client.Connect(); err != nil {
		return &SSHFileUploadResponse{
			ErrorMessage: fmt.Sprintf("SSH 连接失败: %v", err),
		}, nil
	}
	defer client.Close()

	n, err := client.CopyLocalFileToRemote(req.LocalPath, req.RemotePath)
	if err != nil {
		return &SSHFileUploadResponse{
			ErrorMessage: fmt.Sprintf("文件上传失败: %v", err),
		}, nil
	}

	return &SSHFileUploadResponse{
		Message:      fmt.Sprintf("成功上传 %d 字节到远程服务器", n),
		BytesWritten: n,
	}, nil
}

// SSHFileDownloadRequest 文件下载请求
type SSHFileDownloadRequest struct {
	RemotePath string `json:"remote_path" jsonschema:"description=远程文件路径,required"`
	LocalPath  string `json:"local_path" jsonschema:"description=本地文件路径,required"`
}

// SSHFileDownloadResponse 文件下载响应
type SSHFileDownloadResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	BytesRead    int64  `json:"bytes_read" jsonschema:"description=读取的字节数"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// SSHFileDownload 从远程服务器下载文件
func SSHFileDownload(ctx context.Context, req *SSHFileDownloadRequest) (*SSHFileDownloadResponse, error) {
	if globalSSHConn == nil {
		return &SSHFileDownloadResponse{
			ErrorMessage: "SSH 客户端未初始化，请先设置环境变量 SSH_HOST, SSH_USERNAME, SSH_PASSWORD",
		}, nil
	}

	if req.RemotePath == "" || req.LocalPath == "" {
		return &SSHFileDownloadResponse{
			ErrorMessage: "remote_path 和 local_path 参数都是必需的",
		}, nil
	}

	// 创建 SSH 连接
	client := globalSSHConn.Client
	if err := client.Connect(); err != nil {
		return &SSHFileDownloadResponse{
			ErrorMessage: fmt.Sprintf("SSH 连接失败: %v", err),
		}, nil
	}
	defer client.Close()

	n, err := client.CopyRemoteFileToLocal(req.RemotePath, req.LocalPath)
	if err != nil {
		return &SSHFileDownloadResponse{
			ErrorMessage: fmt.Sprintf("文件下载失败: %v", err),
		}, nil
	}

	return &SSHFileDownloadResponse{
		Message:   fmt.Sprintf("成功从远程服务器下载 %d 字节", n),
		BytesRead: n,
	}, nil
}

// SSHListDirRequest 列出远程目录请求
type SSHListDirRequest struct {
	RemotePath string `json:"remote_path" jsonschema:"description=远程目录路径,required"`
}

// SSHListDirResponse 列出远程目录响应
type SSHListDirResponse struct {
	Content      string `json:"content" jsonschema:"description=目录内容列表"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// SSHListDir 列出远程目录的内容
func SSHListDir(ctx context.Context, req *SSHListDirRequest) (*SSHListDirResponse, error) {
	if globalSSHConn == nil {
		return &SSHListDirResponse{
			ErrorMessage: "SSH 客户端未初始化，请先设置环境变量 SSH_HOST, SSH_USERNAME, SSH_PASSWORD",
		}, nil
	}

	if req.RemotePath == "" {
		return &SSHListDirResponse{
			ErrorMessage: "remote_path 参数是必需的",
		}, nil
	}

	// 创建 SSH 连接
	client := globalSSHConn.Client
	if err := client.Connect(); err != nil {
		return &SSHListDirResponse{
			ErrorMessage: fmt.Sprintf("SSH 连接失败: %v", err),
		}, nil
	}
	defer client.Close()

	fileNames, err := client.ReadRemoteDir(req.RemotePath)
	if err != nil {
		return &SSHListDirResponse{
			ErrorMessage: fmt.Sprintf("列出目录失败: %v", err),
		}, nil
	}

	content := fmt.Sprintf("目录 %s 下的文件和文件夹：\n", req.RemotePath)
	for _, name := range fileNames {
		content += name + "\n"
	}

	return &SSHListDirResponse{
		Content: content,
	}, nil
}

// GetSSHTools 获取所有 SSH 操作工具
// 注意: 调用此函数前必须先调用 InitSSHClient 初始化 SSH 连接信息
func GetSSHTools() ([]tool.BaseTool, error) {
	if globalSSHConn == nil {
		return nil, fmt.Errorf("SSH 客户端未初始化，请先调用 InitSSHClient")
	}

	var tools []tool.BaseTool

	// 执行远程命令工具
	executeTool, err := utils.InferTool("ssh_execute", "在远程服务器执行 shell 命令。命令执行前会进行安全检查，拒绝执行危险命令。支持设置超时时间（默认60秒，最大300秒）", SSHExecute)
	if err != nil {
		return nil, err
	}
	tools = append(tools, executeTool)

	// 文件上传工具
	uploadTool, err := utils.InferTool("ssh_upload", "上传本地文件到远程服务器。支持单个文件上传", SSHFileUpload)
	if err != nil {
		return nil, err
	}
	tools = append(tools, uploadTool)

	// 文件下载工具
	downloadTool, err := utils.InferTool("ssh_download", "从远程服务器下载文件到本地。支持单个文件下载", SSHFileDownload)
	if err != nil {
		return nil, err
	}
	tools = append(tools, downloadTool)

	// 列出远程目录工具
	listDirTool, err := utils.InferTool("ssh_list_dir", "列出远程服务器指定目录下的文件和文件夹", SSHListDir)
	if err != nil {
		return nil, err
	}
	tools = append(tools, listDirTool)

	return tools, nil
}
