package cli

import (
	"os"
)

// GetWorkspaceDir 获取工作目录路径
func GetWorkspaceDir() string {
	dir := os.Getenv("FEIKONG_WORKSPACE_DIR")
	if dir == "" {
		dir = "./workspace"
	}
	return dir
}
