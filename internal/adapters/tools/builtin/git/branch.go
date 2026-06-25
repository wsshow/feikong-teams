package git

import (
	"context"
	"fmt"

	"github.com/go-git/go-git/v5"

	"github.com/go-git/go-git/v5/plumbing"
)

// GitBranchRequest 分支操作请求
type GitBranchRequest struct {
	Path       string `json:"path,omitempty" jsonschema:"description=仓库路径(相对于工作目录)"`
	Name       string `json:"name,omitempty" jsonschema:"description=分支名称(创建或删除分支时需要)"`
	Action     string `json:"action,omitempty" jsonschema:"description=操作类型: list(列出分支)/create(创建分支)/delete(删除分支),默认为list"`
	StartPoint string `json:"start_point,omitempty" jsonschema:"description=创建分支时的起始点(提交哈希或分支名)"`
}

// BranchInfo 分支信息
type BranchInfo struct {
	Name      string `json:"name" jsonschema:"description=分支名称"`
	Hash      string `json:"hash" jsonschema:"description=分支指向的提交哈希"`
	IsCurrent bool   `json:"is_current" jsonschema:"description=是否为当前分支"`
}

// GitBranchResponse 分支操作响应
type GitBranchResponse struct {
	Branches     []BranchInfo `json:"branches,omitempty" jsonschema:"description=分支列表"`
	Message      string       `json:"message,omitempty" jsonschema:"description=结果消息"`
	ErrorMessage string       `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// GitBranch 分支操作
func (gt *GitTools) GitBranch(ctx context.Context, req *GitBranchRequest) (*GitBranchResponse, error) {
	path, err := gt.validatePath(req.Path)
	if err != nil {
		return &GitBranchResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		return &GitBranchResponse{
			ErrorMessage: fmt.Sprintf("无法打开仓库: %v", err),
		}, nil
	}

	action := req.Action
	if action == "" {
		action = "list"
	}

	switch action {
	case "list":
		return gt.listBranches(repo)
	case "create":
		if err := requireGitApproval(ctx, path, "branch:create", req.Name); err != nil {
			if msg, ok := gitApprovalError(err); ok {
				return &GitBranchResponse{ErrorMessage: msg}, nil
			}
			return nil, err
		}
		return gt.createBranch(repo, req.Name, req.StartPoint)
	case "delete":
		if err := requireGitApproval(ctx, path, "branch:delete", req.Name); err != nil {
			if msg, ok := gitApprovalError(err); ok {
				return &GitBranchResponse{ErrorMessage: msg}, nil
			}
			return nil, err
		}
		return gt.deleteBranch(repo, req.Name)
	default:
		return &GitBranchResponse{
			ErrorMessage: fmt.Sprintf("未知的操作类型: %s", action),
		}, nil
	}
}

func (gt *GitTools) listBranches(repo *git.Repository) (*GitBranchResponse, error) {

	head, err := repo.Head()
	var currentBranch string
	if err == nil && head.Name().IsBranch() {
		currentBranch = head.Name().Short()
	}

	branchIter, err := repo.Branches()
	if err != nil {
		return &GitBranchResponse{
			ErrorMessage: fmt.Sprintf("无法获取分支列表: %v", err),
		}, nil
	}

	var branches []BranchInfo
	err = branchIter.ForEach(func(ref *plumbing.Reference) error {
		branches = append(branches, BranchInfo{
			Name:      ref.Name().Short(),
			Hash:      ref.Hash().String()[:7],
			IsCurrent: ref.Name().Short() == currentBranch,
		})
		return nil
	})
	if err != nil {
		return &GitBranchResponse{
			ErrorMessage: fmt.Sprintf("遍历分支失败: %v", err),
		}, nil
	}

	return &GitBranchResponse{
		Branches: branches,
		Message:  fmt.Sprintf("共 %d 个分支", len(branches)),
	}, nil
}

func (gt *GitTools) createBranch(repo *git.Repository, name string, startPoint string) (*GitBranchResponse, error) {
	if name == "" {
		return &GitBranchResponse{
			ErrorMessage: "分支名称不能为空",
		}, nil
	}

	// 获取起始点
	var hash plumbing.Hash
	if startPoint != "" {

		resolvedHash, err := repo.ResolveRevision(plumbing.Revision(startPoint))
		if err != nil {
			return &GitBranchResponse{
				ErrorMessage: fmt.Sprintf("无法解析起始点 %s: %v", startPoint, err),
			}, nil
		}
		hash = *resolvedHash
	} else {

		head, err := repo.Head()
		if err != nil {
			return &GitBranchResponse{
				ErrorMessage: fmt.Sprintf("无法获取 HEAD: %v", err),
			}, nil
		}
		hash = head.Hash()
	}

	refName := plumbing.NewBranchReferenceName(name)
	ref := plumbing.NewHashReference(refName, hash)

	err := repo.Storer.SetReference(ref)
	if err != nil {
		return &GitBranchResponse{
			ErrorMessage: fmt.Sprintf("创建分支失败: %v", err),
		}, nil
	}

	return &GitBranchResponse{
		Message: fmt.Sprintf("成功创建分支 %s", name),
	}, nil
}

func (gt *GitTools) deleteBranch(repo *git.Repository, name string) (*GitBranchResponse, error) {
	if name == "" {
		return &GitBranchResponse{
			ErrorMessage: "分支名称不能为空",
		}, nil
	}

	head, err := repo.Head()
	if err == nil && head.Name().Short() == name {
		return &GitBranchResponse{
			ErrorMessage: "不能删除当前分支",
		}, nil
	}

	refName := plumbing.NewBranchReferenceName(name)
	err = repo.Storer.RemoveReference(refName)
	if err != nil {
		return &GitBranchResponse{
			ErrorMessage: fmt.Sprintf("删除分支失败: %v", err),
		}, nil
	}

	return &GitBranchResponse{
		Message: fmt.Sprintf("成功删除分支 %s", name),
	}, nil
}
