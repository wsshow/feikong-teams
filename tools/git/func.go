package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fkteams/tools/approval"
)

// GitTools Git工具实例
type GitTools struct {
	// baseDir 是允许操作的基础目录
	baseDir string
}

// NewGitTools 创建一个新的Git工具实例
// baseDir 是允许操作的基础目录
func NewGitTools(baseDir string) (*GitTools, error) {

	absPath, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("无法获取绝对路径: %w", err)
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		if err := os.MkdirAll(absPath, 0755); err != nil {
			return nil, fmt.Errorf("无法创建目录 %s: %w", absPath, err)
		}
	}

	return &GitTools{
		baseDir: absPath,
	}, nil
}

// validatePath 验证并规范化路径，确保路径在允许的目录范围内
func (gt *GitTools) validatePath(userPath string) (string, error) {
	if userPath == "" {
		return gt.baseDir, nil
	}

	cleanPath := filepath.Clean(userPath)

	if !filepath.IsAbs(cleanPath) {
		cleanPath = filepath.Join(gt.baseDir, cleanPath)
	}

	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return "", fmt.Errorf("无法解析路径: %w", err)
	}

	if !strings.HasPrefix(absPath, gt.baseDir) {
		return "", fmt.Errorf("访问被拒绝: 路径 %s 不在允许的目录 %s 内", absPath, gt.baseDir)
	}

	return absPath, nil
}

func requireGitApproval(ctx context.Context, repoPath, action, detail string) error {
	details := []approval.OperationDetail{{Name: "Operation", Value: action}}
	if detail != "" {
		details = append(details, approval.OperationDetail{Name: "Detail", Value: detail})
	}
	return approval.RequireOperation(ctx, approval.Operation{
		StoreName: approval.StoreGit,
		Key:       filepath.Join(repoPath, action),
		Title:     "Git operation requires approval",
		Target:    repoPath,
		Details:   details,
	})
}

func gitApprovalError(err error) (string, bool) {
	return approval.RejectedMessage(err, "git operation rejected by user")
}
