package git

import (
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// GetTools 返回所有Git相关工具
func (gt *GitTools) GetTools() ([]tool.BaseTool, error) {
	gitInitTool, err := utils.InferTool(
		"git_init",
		"初始化一个新的Git仓库",
		gt.GitInit,
	)
	if err != nil {
		return nil, err
	}

	gitStatusTool, err := utils.InferTool(
		"git_status",
		"获取Git仓库的状态",
		gt.GitStatus,
	)
	if err != nil {
		return nil, err
	}

	gitAddTool, err := utils.InferTool(
		"git_add",
		"添加文件到暂存区",
		gt.GitAdd,
	)
	if err != nil {
		return nil, err
	}

	gitCommitTool, err := utils.InferTool(
		"git_commit",
		"创建一个新的提交",
		gt.GitCommit,
	)
	if err != nil {
		return nil, err
	}

	gitLogTool, err := utils.InferTool(
		"git_log",
		"查看提交历史",
		gt.GitLog,
	)
	if err != nil {
		return nil, err
	}

	gitBranchTool, err := utils.InferTool(
		"git_branch",
		"分支操作：列出、创建、删除分支",
		gt.GitBranch,
	)
	if err != nil {
		return nil, err
	}

	gitCheckoutTool, err := utils.InferTool(
		"git_checkout",
		"切换分支或检出提交",
		gt.GitCheckout,
	)
	if err != nil {
		return nil, err
	}

	gitResetTool, err := utils.InferTool(
		"git_reset",
		"重置仓库到指定状态（支持soft/mixed/hard模式）",
		gt.GitReset,
	)
	if err != nil {
		return nil, err
	}

	gitTagTool, err := utils.InferTool(
		"git_tag",
		"标签操作：列出、创建、删除标签",
		gt.GitTag,
	)
	if err != nil {
		return nil, err
	}

	gitDiffTool, err := utils.InferTool(
		"git_diff",
		"查看工作区或暂存区的差异",
		gt.GitDiff,
	)
	if err != nil {
		return nil, err
	}

	gitCleanTool, err := utils.InferTool(
		"git_clean",
		"清理未跟踪的文件",
		gt.GitClean,
	)
	if err != nil {
		return nil, err
	}

	gitRemoveTool, err := utils.InferTool(
		"git_remove",
		"从仓库移除文件",
		gt.GitRemove,
	)
	if err != nil {
		return nil, err
	}

	gitConfigTool, err := utils.InferTool(
		"git_config",
		"Git配置操作：获取、设置、列出配置",
		gt.GitConfig,
	)
	if err != nil {
		return nil, err
	}

	gitRemoteTool, err := utils.InferTool(
		"git_remote",
		"远程仓库操作：列出、添加、移除远程仓库",
		gt.GitRemote,
	)
	if err != nil {
		return nil, err
	}

	return []tool.BaseTool{
		gitInitTool,
		gitStatusTool,
		gitAddTool,
		gitCommitTool,
		gitLogTool,
		gitBranchTool,
		gitCheckoutTool,
		gitResetTool,
		gitTagTool,
		gitDiffTool,
		gitCleanTool,
		gitRemoveTool,
		gitConfigTool,
		gitRemoteTool,
	}, nil
}
