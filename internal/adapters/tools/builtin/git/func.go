package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fkteams/internal/runtime/approval"
	"fkteams/internal/runtime/pathguard"
	"fkteams/internal/runtime/resources"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

// GitTools Git工具实例
type GitTools struct {
	locks *resources.KeyedLocks
	// baseDir 是允许操作的基础目录
	baseDir string
}

// NewGitTools 创建一个新的Git工具实例
// baseDir 是允许操作的基础目录
func NewGitTools(baseDir string, shared ...*resources.KeyedLocks) (*GitTools, error) {

	absPath, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("无法获取绝对路径: %w", err)
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		if err := os.MkdirAll(absPath, 0755); err != nil {
			return nil, fmt.Errorf("无法创建目录 %s: %w", absPath, err)
		}
	}

	locks := resources.NewKeyedLocks()
	if len(shared) > 0 && shared[0] != nil {
		locks = shared[0]
	}

	return &GitTools{
		locks:   locks,
		baseDir: absPath,
	}, nil
}

// validatePath 验证并规范化路径，确保路径在允许的目录范围内
func (gt *GitTools) validatePath(userPath string) (string, error) {
	resolved, err := pathguard.ResolveWorkspace(gt.baseDir, userPath)
	if err != nil {
		return "", fmt.Errorf("access denied: %w", err)
	}
	// 仓库元数据也必须留在工作区，拒绝外部 gitdir 和符号链接。
	dotgit := filepath.Join(resolved.AbsPath, ".git")
	if info, statErr := os.Lstat(dotgit); statErr == nil {
		if _, err := pathguard.ResolveWorkspace(gt.baseDir, dotgit); err != nil {
			return "", fmt.Errorf("git metadata is outside workspace: %w", err)
		}
		if !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			data, err := os.ReadFile(dotgit)
			if err != nil {
				return "", err
			}
			if len(data) > 4096 || !strings.HasPrefix(string(data), "gitdir: ") {
				return "", fmt.Errorf("invalid gitdir file")
			}
			target := strings.TrimSpace(strings.TrimPrefix(string(data), "gitdir: "))
			if !filepath.IsAbs(target) {
				target = filepath.Join(resolved.AbsPath, target)
			}
			if _, err := pathguard.ResolveWorkspace(gt.baseDir, target); err != nil {
				return "", fmt.Errorf("git metadata is outside workspace: %w", err)
			}
		}
	}
	return resolved.AbsPath, nil
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

func (gt *GitTools) metadataPath(path string) string {
	metadata := filepath.Join(path, ".git")
	if data, err := os.ReadFile(metadata); err == nil {
		target := strings.TrimSpace(strings.TrimPrefix(string(data), "gitdir: "))
		if !filepath.IsAbs(target) {
			target = filepath.Join(path, target)
		}
		return target
	}
	if _, err := os.Stat(metadata); os.IsNotExist(err) {
		return path
	}
	return metadata
}

func (gt *GitTools) openRepository(path string) (*git.Repository, error) {
	// BoundOS 在每次底层访问时验证路径，不能只校验仓库入口。
	metadata := gt.metadataPath(path)
	if _, err := pathguard.ResolveWorkspace(gt.baseDir, metadata); err != nil {
		return nil, err
	}
	store := filesystem.NewStorage(osfs.New(metadata, osfs.WithBoundOS()), cache.NewObjectLRUDefault())
	return git.Open(store, osfs.New(path, osfs.WithBoundOS()))
}

func (gt *GitTools) initRepository(path string, bare bool) (*git.Repository, error) {
	metadata := path
	if !bare {
		metadata = filepath.Join(path, ".git")
	}
	if _, err := pathguard.ResolveWorkspace(gt.baseDir, metadata); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(metadata, 0755); err != nil {
		return nil, err
	}
	store := filesystem.NewStorage(osfs.New(metadata, osfs.WithBoundOS()), cache.NewObjectLRUDefault())
	if bare {
		return git.Init(store, nil)
	}
	return git.Init(store, osfs.New(path, osfs.WithBoundOS()))
}
