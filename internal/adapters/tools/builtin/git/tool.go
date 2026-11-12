package git

import (
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/resources"
)

// GetTools 返回所有Git相关工具
func (gt *GitTools) GetTools() ([]runtimeport.Tool, error) {

	return runtimeport.NewTools([]runtimeport.ToolDefinition{
		{Name: "git_init", Description: "初始化一个新的Git仓库", Handler: resources.Serialize(gt.locks, gt.baseDir, gt.GitInit)},
		{Name: "git_status", Description: "获取Git仓库的状态", Handler: resources.Read(gt.locks, gt.baseDir, gt.GitStatus)},
		{Name: "git_add", Description: "添加文件到暂存区", Handler: resources.Serialize(gt.locks, gt.baseDir, gt.GitAdd)},
		{Name: "git_commit", Description: "创建一个新的提交", Handler: resources.Serialize(gt.locks, gt.baseDir, gt.GitCommit)},
		{Name: "git_log", Description: "查看提交历史", Handler: resources.Read(gt.locks, gt.baseDir, gt.GitLog)},
		{Name: "git_branch", Description: "分支操作：列出、创建、删除分支", Handler: resources.Serialize(gt.locks, gt.baseDir, gt.GitBranch)},
		{Name: "git_checkout", Description: "切换分支或检出提交", Handler: resources.Serialize(gt.locks, gt.baseDir, gt.GitCheckout)},
		{Name: "git_reset", Description: "重置仓库到指定状态（支持soft/mixed/hard模式）", Handler: resources.Serialize(gt.locks, gt.baseDir, gt.GitReset)},
		{Name: "git_tag", Description: "标签操作：列出、创建、删除标签", Handler: resources.Serialize(gt.locks, gt.baseDir, gt.GitTag)},
		{Name: "git_diff", Description: "查看工作区或暂存区的差异", Handler: resources.Read(gt.locks, gt.baseDir, gt.GitDiff)},
		{Name: "git_clean", Description: "清理未跟踪的文件", Handler: resources.Serialize(gt.locks, gt.baseDir, gt.GitClean)},
		{Name: "git_remove", Description: "从仓库移除文件", Handler: resources.Serialize(gt.locks, gt.baseDir, gt.GitRemove)},
		{Name: "git_config", Description: "Git配置操作：获取、设置、列出配置", Handler: resources.Serialize(gt.locks, gt.baseDir, gt.GitConfig)},
		{Name: "git_remote", Description: "远程仓库操作：列出、添加、移除远程仓库", Handler: resources.Serialize(gt.locks, gt.baseDir, gt.GitRemote)},
	})
}
