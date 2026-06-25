package git

import (
	"context"
	"fmt"

	"github.com/go-git/go-git/v5"
)

// GitCleanRequest 清理未跟踪文件请求
type GitCleanRequest struct {
	Path        string `json:"path,omitempty" jsonschema:"description=仓库路径(相对于工作目录)"`
	DryRun      bool   `json:"dry_run,omitempty" jsonschema:"description=是否只显示将要删除的文件而不实际删除"`
	Directories bool   `json:"directories,omitempty" jsonschema:"description=是否也删除未跟踪的目录"`
}

// GitCleanResponse 清理未跟踪文件响应
type GitCleanResponse struct {
	CleanedFiles []string `json:"cleaned_files,omitempty" jsonschema:"description=清理的文件列表"`
	Message      string   `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string   `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// GitClean 清理未跟踪的文件
func (gt *GitTools) GitClean(ctx context.Context, req *GitCleanRequest) (*GitCleanResponse, error) {
	path, err := gt.validatePath(req.Path)
	if err != nil {
		return &GitCleanResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		return &GitCleanResponse{
			ErrorMessage: fmt.Sprintf("无法打开仓库: %v", err),
		}, nil
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return &GitCleanResponse{
			ErrorMessage: fmt.Sprintf("无法获取工作树: %v", err),
		}, nil
	}

	status, err := worktree.Status()
	if err != nil {
		return &GitCleanResponse{
			ErrorMessage: fmt.Sprintf("无法获取状态: %v", err),
		}, nil
	}

	var untrackedFiles []string
	for filePath, fileStatus := range status {
		if fileStatus.Worktree == git.Untracked {
			untrackedFiles = append(untrackedFiles, filePath)
		}
	}

	if req.DryRun {
		return &GitCleanResponse{
			CleanedFiles: untrackedFiles,
			Message:      fmt.Sprintf("将要删除 %d 个未跟踪文件（干运行模式）", len(untrackedFiles)),
		}, nil
	}

	if err := requireGitApproval(ctx, path, "clean", fmt.Sprintf("directories=%t", req.Directories)); err != nil {
		if msg, ok := gitApprovalError(err); ok {
			return &GitCleanResponse{ErrorMessage: msg}, nil
		}
		return nil, err
	}

	err = worktree.Clean(&git.CleanOptions{
		Dir: req.Directories,
	})
	if err != nil {
		return &GitCleanResponse{
			ErrorMessage: fmt.Sprintf("清理失败: %v", err),
		}, nil
	}

	return &GitCleanResponse{
		CleanedFiles: untrackedFiles,
		Message:      fmt.Sprintf("成功清理 %d 个未跟踪文件", len(untrackedFiles)),
	}, nil
}
