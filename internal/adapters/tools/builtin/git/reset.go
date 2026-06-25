package git

import (
	"context"
	"fmt"

	"strings"

	"github.com/go-git/go-git/v5"

	"github.com/go-git/go-git/v5/plumbing"
)

// GitResetRequest 重置请求
type GitResetRequest struct {
	Path   string   `json:"path,omitempty" jsonschema:"description=仓库路径(相对于工作目录)"`
	Commit string   `json:"commit,omitempty" jsonschema:"description=要重置到的提交哈希(默认为HEAD)"`
	Mode   string   `json:"mode,omitempty" jsonschema:"description=重置模式: soft(只移动HEAD)/mixed(重置暂存区)/hard(重置暂存区和工作区),默认为mixed"`
	Files  []string `json:"files,omitempty" jsonschema:"description=要重置的文件列表(仅在重置特定文件时使用)"`
}

// GitResetResponse 重置响应
type GitResetResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// GitReset 重置仓库到指定状态
func (gt *GitTools) GitReset(ctx context.Context, req *GitResetRequest) (*GitResetResponse, error) {
	path, err := gt.validatePath(req.Path)
	if err != nil {
		return &GitResetResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		return &GitResetResponse{
			ErrorMessage: fmt.Sprintf("无法打开仓库: %v", err),
		}, nil
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return &GitResetResponse{
			ErrorMessage: fmt.Sprintf("无法获取工作树: %v", err),
		}, nil
	}

	// 获取目标提交
	var commitHash plumbing.Hash
	if req.Commit != "" {
		resolved, err := repo.ResolveRevision(plumbing.Revision(req.Commit))
		if err != nil {
			return &GitResetResponse{
				ErrorMessage: fmt.Sprintf("无法解析提交 %s: %v", req.Commit, err),
			}, nil
		}
		commitHash = *resolved
	} else {
		head, err := repo.Head()
		if err != nil {
			return &GitResetResponse{
				ErrorMessage: fmt.Sprintf("无法获取 HEAD: %v", err),
			}, nil
		}
		commitHash = head.Hash()
	}

	// 确定重置模式
	var mode git.ResetMode
	switch strings.ToLower(req.Mode) {
	case "soft":
		mode = git.SoftReset
	case "hard":
		mode = git.HardReset
	case "mixed", "":
		mode = git.MixedReset
	default:
		return &GitResetResponse{
			ErrorMessage: fmt.Sprintf("未知的重置模式: %s", req.Mode),
		}, nil
	}

	if err := requireGitApproval(ctx, path, "reset", strings.ToLower(req.Mode)); err != nil {
		if msg, ok := gitApprovalError(err); ok {
			return &GitResetResponse{ErrorMessage: msg}, nil
		}
		return nil, err
	}

	opts := &git.ResetOptions{
		Commit: commitHash,
		Mode:   mode,
		Files:  req.Files,
	}

	err = worktree.Reset(opts)
	if err != nil {
		return &GitResetResponse{
			ErrorMessage: fmt.Sprintf("重置失败: %v", err),
		}, nil
	}

	modeDesc := "mixed"
	if req.Mode != "" {
		modeDesc = strings.ToLower(req.Mode)
	}

	return &GitResetResponse{
		Message: fmt.Sprintf("成功重置到 %s (%s模式)", commitHash.String()[:7], modeDesc),
	}, nil
}
