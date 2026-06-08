package git

import (
	"context"
	"fmt"

	"github.com/go-git/go-git/v5"
)

// GitConfigRequest 配置操作请求
type GitConfigRequest struct {
	Path   string `json:"path,omitempty" jsonschema:"description=仓库路径(相对于工作目录)"`
	Action string `json:"action,omitempty" jsonschema:"description=操作类型: get(获取配置)/set(设置配置)/list(列出所有配置),默认为list"`
	Key    string `json:"key,omitempty" jsonschema:"description=配置键名(get/set时需要)"`
	Value  string `json:"value,omitempty" jsonschema:"description=配置值(set时需要)"`
}

// ConfigItem 配置项
type ConfigItem struct {
	Key   string `json:"key" jsonschema:"description=配置键"`
	Value string `json:"value" jsonschema:"description=配置值"`
}

// GitConfigResponse 配置操作响应
type GitConfigResponse struct {
	Configs      []ConfigItem `json:"configs,omitempty" jsonschema:"description=配置列表"`
	Value        string       `json:"value,omitempty" jsonschema:"description=配置值(get操作时)"`
	Message      string       `json:"message,omitempty" jsonschema:"description=结果消息"`
	ErrorMessage string       `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// GitConfig 配置操作
func (gt *GitTools) GitConfig(ctx context.Context, req *GitConfigRequest) (*GitConfigResponse, error) {
	path, err := gt.validatePath(req.Path)
	if err != nil {
		return &GitConfigResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		return &GitConfigResponse{
			ErrorMessage: fmt.Sprintf("无法打开仓库: %v", err),
		}, nil
	}

	cfg, err := repo.Config()
	if err != nil {
		return &GitConfigResponse{
			ErrorMessage: fmt.Sprintf("无法获取配置: %v", err),
		}, nil
	}

	action := req.Action
	if action == "" {
		action = "list"
	}

	switch action {
	case "list":
		var configs []ConfigItem

		if cfg.User.Name != "" {
			configs = append(configs, ConfigItem{Key: "user.name", Value: cfg.User.Name})
		}
		if cfg.User.Email != "" {
			configs = append(configs, ConfigItem{Key: "user.email", Value: cfg.User.Email})
		}

		for name, remote := range cfg.Remotes {
			if len(remote.URLs) > 0 {
				configs = append(configs, ConfigItem{Key: fmt.Sprintf("remote.%s.url", name), Value: remote.URLs[0]})
			}
		}

		for name, branch := range cfg.Branches {
			if branch.Remote != "" {
				configs = append(configs, ConfigItem{Key: fmt.Sprintf("branch.%s.remote", name), Value: branch.Remote})
			}
			if branch.Merge != "" {
				configs = append(configs, ConfigItem{Key: fmt.Sprintf("branch.%s.merge", name), Value: string(branch.Merge)})
			}
		}
		return &GitConfigResponse{
			Configs: configs,
			Message: fmt.Sprintf("共 %d 项配置", len(configs)),
		}, nil

	case "get":
		if req.Key == "" {
			return &GitConfigResponse{
				ErrorMessage: "请指定配置键名",
			}, nil
		}

		switch req.Key {
		case "user.name":
			return &GitConfigResponse{Value: cfg.User.Name, Message: "获取成功"}, nil
		case "user.email":
			return &GitConfigResponse{Value: cfg.User.Email, Message: "获取成功"}, nil
		default:
			return &GitConfigResponse{
				ErrorMessage: fmt.Sprintf("不支持获取配置项: %s", req.Key),
			}, nil
		}

	case "set":
		if req.Key == "" || req.Value == "" {
			return &GitConfigResponse{
				ErrorMessage: "请指定配置键名和值",
			}, nil
		}
		if err := requireGitApproval(ctx, path, "config:set", req.Key); err != nil {
			if msg, ok := gitApprovalError(err); ok {
				return &GitConfigResponse{ErrorMessage: msg}, nil
			}
			return nil, err
		}
		switch req.Key {
		case "user.name":
			cfg.User.Name = req.Value
		case "user.email":
			cfg.User.Email = req.Value
		default:
			return &GitConfigResponse{
				ErrorMessage: fmt.Sprintf("不支持设置配置项: %s", req.Key),
			}, nil
		}
		err = repo.SetConfig(cfg)
		if err != nil {
			return &GitConfigResponse{
				ErrorMessage: fmt.Sprintf("设置配置失败: %v", err),
			}, nil
		}
		return &GitConfigResponse{
			Message: fmt.Sprintf("成功设置 %s = %s", req.Key, req.Value),
		}, nil

	default:
		return &GitConfigResponse{
			ErrorMessage: fmt.Sprintf("未知的操作类型: %s", action),
		}, nil
	}
}
