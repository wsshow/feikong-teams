package git

import (
	"context"
	"fmt"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
)

// GitRemoteRequest 远程仓库操作请求
type GitRemoteRequest struct {
	Path   string `json:"path,omitempty" jsonschema:"description=仓库路径(相对于工作目录)"`
	Action string `json:"action,omitempty" jsonschema:"description=操作类型: list(列出远程)/add(添加远程)/remove(移除远程),默认为list"`
	Name   string `json:"name,omitempty" jsonschema:"description=远程仓库名称(add/remove时需要)"`
	URL    string `json:"url,omitempty" jsonschema:"description=远程仓库URL(add时需要)"`
}

// RemoteInfo 远程仓库信息
type RemoteInfo struct {
	Name string   `json:"name" jsonschema:"description=远程仓库名称"`
	URLs []string `json:"urls" jsonschema:"description=远程仓库URL列表"`
}

// GitRemoteResponse 远程仓库操作响应
type GitRemoteResponse struct {
	Remotes      []RemoteInfo `json:"remotes,omitempty" jsonschema:"description=远程仓库列表"`
	Message      string       `json:"message,omitempty" jsonschema:"description=结果消息"`
	ErrorMessage string       `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// GitRemote 远程仓库操作
func (gt *GitTools) GitRemote(ctx context.Context, req *GitRemoteRequest) (*GitRemoteResponse, error) {
	path, err := gt.validatePath(req.Path)
	if err != nil {
		return &GitRemoteResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		return &GitRemoteResponse{
			ErrorMessage: fmt.Sprintf("无法打开仓库: %v", err),
		}, nil
	}

	action := req.Action
	if action == "" {
		action = "list"
	}

	switch action {
	case "list":
		remotes, err := repo.Remotes()
		if err != nil {
			return &GitRemoteResponse{
				ErrorMessage: fmt.Sprintf("无法获取远程仓库列表: %v", err),
			}, nil
		}
		var remoteInfos []RemoteInfo
		for _, r := range remotes {
			cfg := r.Config()
			remoteInfos = append(remoteInfos, RemoteInfo{
				Name: cfg.Name,
				URLs: cfg.URLs,
			})
		}
		return &GitRemoteResponse{
			Remotes: remoteInfos,
			Message: fmt.Sprintf("共 %d 个远程仓库", len(remoteInfos)),
		}, nil

	case "add":
		if req.Name == "" || req.URL == "" {
			return &GitRemoteResponse{
				ErrorMessage: "请指定远程仓库名称和URL",
			}, nil
		}
		if err := requireGitApproval(ctx, path, "remote:add", req.Name); err != nil {
			if msg, ok := gitApprovalError(err); ok {
				return &GitRemoteResponse{ErrorMessage: msg}, nil
			}
			return nil, err
		}
		_, err := repo.CreateRemote(&config.RemoteConfig{
			Name: req.Name,
			URLs: []string{req.URL},
		})
		if err != nil {
			return &GitRemoteResponse{
				ErrorMessage: fmt.Sprintf("添加远程仓库失败: %v", err),
			}, nil
		}
		return &GitRemoteResponse{
			Message: fmt.Sprintf("成功添加远程仓库 %s -> %s", req.Name, req.URL),
		}, nil

	case "remove":
		if req.Name == "" {
			return &GitRemoteResponse{
				ErrorMessage: "请指定远程仓库名称",
			}, nil
		}
		if err := requireGitApproval(ctx, path, "remote:remove", req.Name); err != nil {
			if msg, ok := gitApprovalError(err); ok {
				return &GitRemoteResponse{ErrorMessage: msg}, nil
			}
			return nil, err
		}
		err := repo.DeleteRemote(req.Name)
		if err != nil {
			return &GitRemoteResponse{
				ErrorMessage: fmt.Sprintf("移除远程仓库失败: %v", err),
			}, nil
		}
		return &GitRemoteResponse{
			Message: fmt.Sprintf("成功移除远程仓库 %s", req.Name),
		}, nil

	default:
		return &GitRemoteResponse{
			ErrorMessage: fmt.Sprintf("未知的操作类型: %s", action),
		}, nil
	}
}
