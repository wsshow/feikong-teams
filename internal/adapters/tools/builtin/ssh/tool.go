package ssh

import (
	runtimeport "fkteams/internal/ports/runtime"
	"fmt"
)

// GetTools 获取所有 SSH 操作工具
// st 是SSH工具实例，包含SSH连接配置
func (st *SSHTools) GetTools() ([]runtimeport.Tool, error) {
	if st == nil || st.client == nil {
		return nil, fmt.Errorf("SSH 工具未初始化")
	}
	// 执行远程命令工具

	return runtimeport.NewTools([]runtimeport.ToolDefinition{
		{Name: "ssh_execute", Description: "在远程服务器执行 shell 命令。命令执行前会进行安全检查，拒绝执行危险命令。支持设置超时时间（默认60秒，最大300秒）", Handler: st.SSHExecute},
		{Name: "ssh_upload", Description: "上传工作区内的本地文件到远程服务器。支持最大 1 GiB 的单文件上传", Handler: st.SSHFileUpload},
		{Name: "ssh_download", Description: "从远程服务器下载文件到工作区。支持最大 1 GiB 的单文件原子下载", Handler: st.SSHFileDownload},
		{Name: "ssh_list_dir", Description: "列出远程服务器指定目录下的文件和文件夹，最多返回 10000 项或 1 MiB 文本", Handler: st.SSHListDir},
	})
}
