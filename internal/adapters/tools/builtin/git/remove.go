package git

import (
	"context"
	"fmt"

	"strings"

	"github.com/go-git/go-git/v5"
)

// GitRemoveRequest 从仓库移除文件请求
type GitRemoveRequest struct {
	Path  string   `json:"path,omitempty" jsonschema:"description=仓库路径(相对于工作目录)"`
	Files []string `json:"files" jsonschema:"description=要移除的文件列表,required"`
}

// GitRemoveResponse 从仓库移除文件响应
type GitRemoveResponse struct {
	RemovedFiles []string `json:"removed_files,omitempty" jsonschema:"description=移除的文件列表"`
	Message      string   `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string   `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// GitRemove 从仓库移除文件
func (gt *GitTools) GitRemove(ctx context.Context, req *GitRemoveRequest) (*GitRemoveResponse, error) {
	path, err := gt.validatePath(req.Path)
	if err != nil {
		return &GitRemoveResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if len(req.Files) == 0 {
		return &GitRemoveResponse{
			ErrorMessage: "请指定要移除的文件",
		}, nil
	}

	if err := requireGitApproval(ctx, path, "remove", strings.Join(req.Files, ", ")); err != nil {
		if msg, ok := gitApprovalError(err); ok {
			return &GitRemoveResponse{ErrorMessage: msg}, nil
		}
		return nil, err
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		return &GitRemoveResponse{
			ErrorMessage: fmt.Sprintf("无法打开仓库: %v", err),
		}, nil
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return &GitRemoveResponse{
			ErrorMessage: fmt.Sprintf("无法获取工作树: %v", err),
		}, nil
	}

	var removedFiles []string
	for _, file := range req.Files {
		_, err := worktree.Remove(file)
		if err != nil {
			return &GitRemoveResponse{
				ErrorMessage: fmt.Sprintf("移除文件 %s 失败: %v", file, err),
			}, nil
		}
		removedFiles = append(removedFiles, file)
	}

	return &GitRemoveResponse{
		RemovedFiles: removedFiles,
		Message:      fmt.Sprintf("成功从暂存区移除 %d 个文件", len(removedFiles)),
	}, nil
}
