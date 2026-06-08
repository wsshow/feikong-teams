package git

import (
	"context"
	"fmt"

	"github.com/go-git/go-git/v5"
)

// GitDiffRequest 查看差异请求
type GitDiffRequest struct {
	Path   string `json:"path,omitempty" jsonschema:"description=仓库路径(相对于工作目录)"`
	Cached bool   `json:"cached,omitempty" jsonschema:"description=是否查看暂存区与HEAD的差异(默认查看工作区与暂存区的差异)"`
}

// FileDiff 文件差异信息
type FileDiff struct {
	Path      string `json:"path" jsonschema:"description=文件路径"`
	Status    string `json:"status" jsonschema:"description=状态"`
	Additions int    `json:"additions,omitempty" jsonschema:"description=新增行数"`
	Deletions int    `json:"deletions,omitempty" jsonschema:"description=删除行数"`
}

// GitDiffResponse 查看差异响应
type GitDiffResponse struct {
	Diffs        []FileDiff `json:"diffs,omitempty" jsonschema:"description=差异列表"`
	Message      string     `json:"message,omitempty" jsonschema:"description=结果消息"`
	ErrorMessage string     `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// GitDiff 查看差异
func (gt *GitTools) GitDiff(ctx context.Context, req *GitDiffRequest) (*GitDiffResponse, error) {
	path, err := gt.validatePath(req.Path)
	if err != nil {
		return &GitDiffResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		return &GitDiffResponse{
			ErrorMessage: fmt.Sprintf("无法打开仓库: %v", err),
		}, nil
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return &GitDiffResponse{
			ErrorMessage: fmt.Sprintf("无法获取工作树: %v", err),
		}, nil
	}

	status, err := worktree.Status()
	if err != nil {
		return &GitDiffResponse{
			ErrorMessage: fmt.Sprintf("无法获取状态: %v", err),
		}, nil
	}

	var diffs []FileDiff
	for filePath, fileStatus := range status {
		var statusStr string
		if req.Cached {

			statusStr = statusCodeToString(fileStatus.Staging)
		} else {

			statusStr = statusCodeToString(fileStatus.Worktree)
		}

		if statusStr != "未修改" && statusStr != "" {
			diffs = append(diffs, FileDiff{
				Path:   filePath,
				Status: statusStr,
			})
		}
	}

	diffType := "工作区"
	if req.Cached {
		diffType = "暂存区"
	}

	return &GitDiffResponse{
		Diffs:   diffs,
		Message: fmt.Sprintf("%s有 %d 个文件有变更", diffType, len(diffs)),
	}, nil
}
