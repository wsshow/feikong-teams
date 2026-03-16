package cli

import (
	"fkteams/common"
)

// GetWorkspaceDir 获取工作目录路径
func GetWorkspaceDir() string {
	return common.WorkspaceDir()
}
