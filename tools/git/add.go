package git

import (
	"context"
	"fmt"

	"strings"

	"github.com/go-git/go-git/v5"
)

// GitAddRequest 添加文件到暂存区请求
type GitAddRequest struct {
	Path  string   `json:"path,omitempty" jsonschema:"description=仓库路径(相对于工作目录)"`
	Files []string `json:"files" jsonschema:"description=要添加的文件列表(相对于仓库根目录),使用 [\".\"] 添加所有文件,required"`
}

// GitAddResponse 添加文件到暂存区响应
type GitAddResponse struct {
	Message      string   `json:"message" jsonschema:"description=操作结果消息"`
	AddedFiles   []string `json:"added_files,omitempty" jsonschema:"description=已添加的文件列表"`
	ErrorMessage string   `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// GitAdd 添加文件到暂存区
func (gt *GitTools) GitAdd(ctx context.Context, req *GitAddRequest) (*GitAddResponse, error) {
	path, err := gt.validatePath(req.Path)
	if err != nil {
		return &GitAddResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if len(req.Files) == 0 {
		return &GitAddResponse{
			ErrorMessage: "请指定要添加的文件",
		}, nil
	}

	if err := requireGitApproval(ctx, path, "add", strings.Join(req.Files, ", ")); err != nil {
		if msg, ok := gitApprovalError(err); ok {
			return &GitAddResponse{ErrorMessage: msg}, nil
		}
		return nil, err
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		return &GitAddResponse{
			ErrorMessage: fmt.Sprintf("无法打开仓库: %v", err),
		}, nil
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return &GitAddResponse{
			ErrorMessage: fmt.Sprintf("无法获取工作树: %v", err),
		}, nil
	}

	var addedFiles []string
	for _, file := range req.Files {
		_, err := worktree.Add(file)
		if err != nil {
			return &GitAddResponse{
				ErrorMessage: fmt.Sprintf("添加文件 %s 失败: %v", file, err),
			}, nil
		}
		addedFiles = append(addedFiles, file)
	}

	return &GitAddResponse{
		Message:    fmt.Sprintf("成功添加 %d 个文件到暂存区", len(addedFiles)),
		AddedFiles: addedFiles,
	}, nil
}
