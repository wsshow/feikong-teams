package git

import (
	"context"
	"fmt"

	"strings"
	"time"

	"github.com/go-git/go-git/v5"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// GitTagRequest 标签操作请求
type GitTagRequest struct {
	Path    string `json:"path,omitempty" jsonschema:"description=仓库路径(相对于工作目录)"`
	Action  string `json:"action,omitempty" jsonschema:"description=操作类型: list(列出标签)/create(创建标签)/delete(删除标签),默认为list"`
	Name    string `json:"name,omitempty" jsonschema:"description=标签名称(创建或删除时需要)"`
	Message string `json:"message,omitempty" jsonschema:"description=标签消息(创建附注标签时使用)"`
	Commit  string `json:"commit,omitempty" jsonschema:"description=要打标签的提交(默认为HEAD)"`
}

// TagInfo 标签信息
type TagInfo struct {
	Name    string `json:"name" jsonschema:"description=标签名称"`
	Hash    string `json:"hash" jsonschema:"description=标签指向的提交哈希"`
	Message string `json:"message,omitempty" jsonschema:"description=标签消息(附注标签)"`
	Tagger  string `json:"tagger,omitempty" jsonschema:"description=标签创建者(附注标签)"`
}

// GitTagResponse 标签操作响应
type GitTagResponse struct {
	Tags         []TagInfo `json:"tags,omitempty" jsonschema:"description=标签列表"`
	Message      string    `json:"message,omitempty" jsonschema:"description=结果消息"`
	ErrorMessage string    `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// GitTag 标签操作
func (gt *GitTools) GitTag(ctx context.Context, req *GitTagRequest) (*GitTagResponse, error) {
	path, err := gt.validatePath(req.Path)
	if err != nil {
		return &GitTagResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		return &GitTagResponse{
			ErrorMessage: fmt.Sprintf("无法打开仓库: %v", err),
		}, nil
	}

	action := req.Action
	if action == "" {
		action = "list"
	}

	switch action {
	case "list":
		return gt.listTags(repo)
	case "create":
		if err := requireGitApproval(ctx, path, "tag:create", req.Name); err != nil {
			if msg, ok := gitApprovalError(err); ok {
				return &GitTagResponse{ErrorMessage: msg}, nil
			}
			return nil, err
		}
		return gt.createTag(repo, req.Name, req.Message, req.Commit)
	case "delete":
		if err := requireGitApproval(ctx, path, "tag:delete", req.Name); err != nil {
			if msg, ok := gitApprovalError(err); ok {
				return &GitTagResponse{ErrorMessage: msg}, nil
			}
			return nil, err
		}
		return gt.deleteTag(repo, req.Name)
	default:
		return &GitTagResponse{
			ErrorMessage: fmt.Sprintf("未知的操作类型: %s", action),
		}, nil
	}
}

func (gt *GitTools) listTags(repo *git.Repository) (*GitTagResponse, error) {
	tagIter, err := repo.Tags()
	if err != nil {
		return &GitTagResponse{
			ErrorMessage: fmt.Sprintf("无法获取标签列表: %v", err),
		}, nil
	}

	var tags []TagInfo
	err = tagIter.ForEach(func(ref *plumbing.Reference) error {
		tagInfo := TagInfo{
			Name: ref.Name().Short(),
			Hash: ref.Hash().String()[:7],
		}

		tagObj, err := repo.TagObject(ref.Hash())
		if err == nil {
			tagInfo.Message = strings.TrimSpace(tagObj.Message)
			tagInfo.Tagger = tagObj.Tagger.Name
		}

		tags = append(tags, tagInfo)
		return nil
	})
	if err != nil {
		return &GitTagResponse{
			ErrorMessage: fmt.Sprintf("遍历标签失败: %v", err),
		}, nil
	}

	return &GitTagResponse{
		Tags:    tags,
		Message: fmt.Sprintf("共 %d 个标签", len(tags)),
	}, nil
}

func (gt *GitTools) createTag(repo *git.Repository, name string, message string, commit string) (*GitTagResponse, error) {
	if name == "" {
		return &GitTagResponse{
			ErrorMessage: "标签名称不能为空",
		}, nil
	}

	// 获取目标提交
	var hash plumbing.Hash
	if commit != "" {
		resolved, err := repo.ResolveRevision(plumbing.Revision(commit))
		if err != nil {
			return &GitTagResponse{
				ErrorMessage: fmt.Sprintf("无法解析提交 %s: %v", commit, err),
			}, nil
		}
		hash = *resolved
	} else {
		head, err := repo.Head()
		if err != nil {
			return &GitTagResponse{
				ErrorMessage: fmt.Sprintf("无法获取 HEAD: %v", err),
			}, nil
		}
		hash = head.Hash()
	}

	// 创建标签
	var err error
	if message != "" {

		_, err = repo.CreateTag(name, hash, &git.CreateTagOptions{
			Message: message,
			Tagger: &object.Signature{
				Name:  "fkteams",
				Email: "fkteams@example.com",
				When:  time.Now(),
			},
		})
	} else {

		_, err = repo.CreateTag(name, hash, nil)
	}

	if err != nil {
		return &GitTagResponse{
			ErrorMessage: fmt.Sprintf("创建标签失败: %v", err),
		}, nil
	}

	tagType := "轻量"
	if message != "" {
		tagType = "附注"
	}

	return &GitTagResponse{
		Message: fmt.Sprintf("成功创建%s标签 %s", tagType, name),
	}, nil
}

func (gt *GitTools) deleteTag(repo *git.Repository, name string) (*GitTagResponse, error) {
	if name == "" {
		return &GitTagResponse{
			ErrorMessage: "标签名称不能为空",
		}, nil
	}

	err := repo.DeleteTag(name)
	if err != nil {
		return &GitTagResponse{
			ErrorMessage: fmt.Sprintf("删除标签失败: %v", err),
		}, nil
	}

	return &GitTagResponse{
		Message: fmt.Sprintf("成功删除标签 %s", name),
	}, nil
}
