package ssh

import (
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// GetTools 获取所有 SSH 操作工具
// 注意: 调用此函数前必须先调用 InitSSHClient 初始化 SSH 连接信息
func GetTools() ([]tool.BaseTool, error) {
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
