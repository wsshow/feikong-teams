package git

import (
	"context"
	"fmt"

	"github.com/go-git/go-git/v5"
)

// GitStatusRequest 获取仓库状态请求
type GitStatusRequest struct {
	Path string `json:"path,omitempty" jsonschema:"description=仓库路径(相对于工作目录)"`
}

// FileStatusInfo 文件状态信息
type FileStatusInfo struct {
	Path     string `json:"path" jsonschema:"description=文件路径"`
	Staging  string `json:"staging" jsonschema:"description=暂存区状态"`
	Worktree string `json:"worktree" jsonschema:"description=工作区状态"`
}

// GitStatusResponse 获取仓库状态响应
type GitStatusResponse struct {
	IsClean      bool             `json:"is_clean" jsonschema:"description=仓库是否干净"`
	Files        []FileStatusInfo `json:"files,omitempty" jsonschema:"description=文件状态列表"`
	Message      string           `json:"message,omitempty" jsonschema:"description=状态消息"`
	ErrorMessage string           `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// statusCodeToString 将状态码转换为可读字符串
func statusCodeToString(code git.StatusCode) string {
	switch code {
	case git.Unmodified:
		return "未修改"
	case git.Untracked:
		return "未跟踪"
	case git.Modified:
		return "已修改"
	case git.Added:
		return "已添加"
	case git.Deleted:
		return "已删除"
	case git.Renamed:
		return "已重命名"
	case git.Copied:
		return "已复制"
	case git.UpdatedButUnmerged:
		return "未合并"
	default:
		return string(code)
	}
}

// GitStatus 获取仓库状态
func (gt *GitTools) GitStatus(ctx context.Context, req *GitStatusRequest) (*GitStatusResponse, error) {
	path, err := gt.validatePath(req.Path)
	if err != nil {
		return &GitStatusResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		return &GitStatusResponse{
			ErrorMessage: fmt.Sprintf("无法打开仓库: %v", err),
		}, nil
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return &GitStatusResponse{
			ErrorMessage: fmt.Sprintf("无法获取工作树: %v", err),
		}, nil
	}

	status, err := worktree.Status()
	if err != nil {
		return &GitStatusResponse{
			ErrorMessage: fmt.Sprintf("无法获取状态: %v", err),
		}, nil
	}

	var files []FileStatusInfo
	for filePath, fileStatus := range status {
		files = append(files, FileStatusInfo{
			Path:     filePath,
			Staging:  statusCodeToString(fileStatus.Staging),
			Worktree: statusCodeToString(fileStatus.Worktree),
		})
	}

	isClean := status.IsClean()
	message := "工作区干净"
	if !isClean {
		message = fmt.Sprintf("有 %d 个文件有变更", len(files))
	}

	return &GitStatusResponse{
		IsClean: isClean,
		Files:   files,
		Message: message,
	}, nil
}
