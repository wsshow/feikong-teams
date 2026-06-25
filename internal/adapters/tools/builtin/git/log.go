package git

import (
	"context"
	"fmt"

	"strings"

	"github.com/go-git/go-git/v5"

	"github.com/go-git/go-git/v5/plumbing/object"
)

// GitLogRequest 查看提交历史请求
type GitLogRequest struct {
	Path  string `json:"path,omitempty" jsonschema:"description=仓库路径(相对于工作目录)"`
	Limit int    `json:"limit,omitempty" jsonschema:"description=限制返回的提交数量(默认10)"`
}

// CommitInfo 提交信息
type CommitInfo struct {
	Hash       string `json:"hash" jsonschema:"description=提交哈希"`
	ShortHash  string `json:"short_hash" jsonschema:"description=短哈希"`
	Author     string `json:"author" jsonschema:"description=作者"`
	Email      string `json:"email" jsonschema:"description=作者邮箱"`
	Message    string `json:"message" jsonschema:"description=提交信息"`
	CommitTime string `json:"commit_time" jsonschema:"description=提交时间"`
}

// GitLogResponse 查看提交历史响应
type GitLogResponse struct {
	Commits      []CommitInfo `json:"commits,omitempty" jsonschema:"description=提交列表"`
	Message      string       `json:"message,omitempty" jsonschema:"description=结果消息"`
	ErrorMessage string       `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// GitLog 查看提交历史
func (gt *GitTools) GitLog(ctx context.Context, req *GitLogRequest) (*GitLogResponse, error) {
	path, err := gt.validatePath(req.Path)
	if err != nil {
		return &GitLogResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		return &GitLogResponse{
			ErrorMessage: fmt.Sprintf("无法打开仓库: %v", err),
		}, nil
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	commitIter, err := repo.Log(&git.LogOptions{})
	if err != nil {
		return &GitLogResponse{
			ErrorMessage: fmt.Sprintf("无法获取提交历史: %v", err),
		}, nil
	}

	var commits []CommitInfo
	count := 0
	err = commitIter.ForEach(func(c *object.Commit) error {
		if count >= limit {
			return fmt.Errorf("limit reached")
		}
		commits = append(commits, CommitInfo{
			Hash:       c.Hash.String(),
			ShortHash:  c.Hash.String()[:7],
			Author:     c.Author.Name,
			Email:      c.Author.Email,
			Message:    strings.TrimSpace(c.Message),
			CommitTime: c.Author.When.Format("2006-01-02 15:04:05"),
		})
		count++
		return nil
	})

	if err != nil && err.Error() != "limit reached" {
		return &GitLogResponse{
			ErrorMessage: fmt.Sprintf("遍历提交历史失败: %v", err),
		}, nil
	}

	return &GitLogResponse{
		Commits: commits,
		Message: fmt.Sprintf("获取了 %d 条提交记录", len(commits)),
	}, nil
}
