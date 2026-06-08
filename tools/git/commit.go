package git

import (
	"context"
	"fmt"

	"time"

	"github.com/go-git/go-git/v5"

	"github.com/go-git/go-git/v5/plumbing/object"
)

// GitCommitRequest 提交请求
type GitCommitRequest struct {
	Path    string `json:"path,omitempty" jsonschema:"description=仓库路径(相对于工作目录)"`
	Message string `json:"message" jsonschema:"description=提交信息,required"`
	Author  string `json:"author,omitempty" jsonschema:"description=作者名称"`
	Email   string `json:"email,omitempty" jsonschema:"description=作者邮箱"`
	All     bool   `json:"all,omitempty" jsonschema:"description=是否自动暂存所有修改的文件"`
}

// GitCommitResponse 提交响应
type GitCommitResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	CommitHash   string `json:"commit_hash,omitempty" jsonschema:"description=提交哈希"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// GitCommit 创建一个新的提交
func (gt *GitTools) GitCommit(ctx context.Context, req *GitCommitRequest) (*GitCommitResponse, error) {
	path, err := gt.validatePath(req.Path)
	if err != nil {
		return &GitCommitResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.Message == "" {
		return &GitCommitResponse{
			ErrorMessage: "提交信息不能为空",
		}, nil
	}

	if err := requireGitApproval(ctx, path, "commit", fmt.Sprintf("all=%t", req.All)); err != nil {
		if msg, ok := gitApprovalError(err); ok {
			return &GitCommitResponse{ErrorMessage: msg}, nil
		}
		return nil, err
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		return &GitCommitResponse{
			ErrorMessage: fmt.Sprintf("无法打开仓库: %v", err),
		}, nil
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return &GitCommitResponse{
			ErrorMessage: fmt.Sprintf("无法获取工作树: %v", err),
		}, nil
	}

	author := req.Author
	email := req.Email
	if author == "" {
		author = "fkteams"
	}
	if email == "" {
		email = "fkteams@example.com"
	}

	commitHash, err := worktree.Commit(req.Message, &git.CommitOptions{
		All: req.All,
		Author: &object.Signature{
			Name:  author,
			Email: email,
			When:  time.Now(),
		},
	})
	if err != nil {
		return &GitCommitResponse{
			ErrorMessage: fmt.Sprintf("提交失败: %v", err),
		}, nil
	}

	return &GitCommitResponse{
		Message:    fmt.Sprintf("成功创建提交: %s", commitHash.String()[:7]),
		CommitHash: commitHash.String(),
	}, nil
}
