package git

import (
	"context"
	"fmt"

	"github.com/go-git/go-git/v5"

	"github.com/go-git/go-git/v5/plumbing"
)

// GitCheckoutRequest 切换分支/提交请求
type GitCheckoutRequest struct {
	Path   string `json:"path,omitempty" jsonschema:"description=仓库路径(相对于工作目录)"`
	Branch string `json:"branch,omitempty" jsonschema:"description=要切换的分支名称"`
	Hash   string `json:"hash,omitempty" jsonschema:"description=要切换的提交哈希"`
	Create bool   `json:"create,omitempty" jsonschema:"description=如果分支不存在是否创建"`
	Force  bool   `json:"force,omitempty" jsonschema:"description=是否强制切换(丢弃本地修改)"`
}

// GitCheckoutResponse 切换分支/提交响应
type GitCheckoutResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// GitCheckout 切换分支或提交
func (gt *GitTools) GitCheckout(ctx context.Context, req *GitCheckoutRequest) (*GitCheckoutResponse, error) {
	path, err := gt.validatePath(req.Path)
	if err != nil {
		return &GitCheckoutResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		return &GitCheckoutResponse{
			ErrorMessage: fmt.Sprintf("无法打开仓库: %v", err),
		}, nil
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return &GitCheckoutResponse{
			ErrorMessage: fmt.Sprintf("无法获取工作树: %v", err),
		}, nil
	}

	detail := req.Branch
	if detail == "" {
		detail = req.Hash
	}
	if err := requireGitApproval(ctx, path, "checkout", detail); err != nil {
		if msg, ok := gitApprovalError(err); ok {
			return &GitCheckoutResponse{ErrorMessage: msg}, nil
		}
		return nil, err
	}

	opts := &git.CheckoutOptions{
		Force:  req.Force,
		Create: req.Create,
	}

	if req.Branch != "" {
		opts.Branch = plumbing.NewBranchReferenceName(req.Branch)
	}

	if req.Hash != "" {
		hash := plumbing.NewHash(req.Hash)
		opts.Hash = hash
	}

	err = worktree.Checkout(opts)
	if err != nil {
		return &GitCheckoutResponse{
			ErrorMessage: fmt.Sprintf("切换失败: %v", err),
		}, nil
	}

	var message string
	if req.Branch != "" {
		message = fmt.Sprintf("成功切换到分支 %s", req.Branch)
		if req.Create {
			message = fmt.Sprintf("成功创建并切换到分支 %s", req.Branch)
		}
	} else if req.Hash != "" {
		message = fmt.Sprintf("成功切换到提交 %s", req.Hash[:7])
	}

	return &GitCheckoutResponse{
		Message: message,
	}, nil
}
