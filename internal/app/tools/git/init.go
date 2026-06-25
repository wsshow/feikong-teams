package git

import (
	"context"
	"fmt"
	"os"

	"github.com/go-git/go-git/v5"
)

// GitInitRequest 初始化仓库请求
type GitInitRequest struct {
	Path string `json:"path,omitempty" jsonschema:"description=要初始化的目录路径(相对于工作目录)"`
	Bare bool   `json:"bare,omitempty" jsonschema:"description=是否创建纯仓库(无工作区)"`
}

// GitInitResponse 初始化仓库响应
type GitInitResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	Path         string `json:"path,omitempty" jsonschema:"description=仓库路径"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// GitInit 初始化一个新的Git仓库
func (gt *GitTools) GitInit(ctx context.Context, req *GitInitRequest) (*GitInitResponse, error) {
	path, err := gt.validatePath(req.Path)
	if err != nil {
		return &GitInitResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if err := requireGitApproval(ctx, path, "init", fmt.Sprintf("bare=%t", req.Bare)); err != nil {
		if msg, ok := gitApprovalError(err); ok {
			return &GitInitResponse{ErrorMessage: msg}, nil
		}
		return nil, err
	}

	if err := os.MkdirAll(path, 0755); err != nil {
		return &GitInitResponse{
			ErrorMessage: fmt.Sprintf("无法创建目录: %v", err),
		}, nil
	}

	_, err = git.PlainInit(path, req.Bare)
	if err != nil {
		return &GitInitResponse{
			ErrorMessage: fmt.Sprintf("初始化仓库失败: %v", err),
		}, nil
	}

	repoType := "标准"
	if req.Bare {
		repoType = "纯"
	}

	return &GitInitResponse{
		Message: fmt.Sprintf("成功在 %s 初始化%s Git仓库", path, repoType),
		Path:    path,
	}, nil
}
