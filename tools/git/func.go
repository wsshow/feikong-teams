package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// GitTools Git工具实例
type GitTools struct {
	// baseDir 是允许操作的基础目录
	baseDir string
}

// NewGitTools 创建一个新的Git工具实例
// baseDir 是允许操作的基础目录
func NewGitTools(baseDir string) (*GitTools, error) {
	// 转换为绝对路径
	absPath, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("无法获取绝对路径: %w", err)
	}

	// 检查目录是否存在，如果不存在则创建
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

	// 清理路径
	cleanPath := filepath.Clean(userPath)

	// 如果是相对路径，则相对于 baseDir
	if !filepath.IsAbs(cleanPath) {
		cleanPath = filepath.Join(gt.baseDir, cleanPath)
	}

	// 转换为绝对路径以检查路径穿越
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return "", fmt.Errorf("无法解析路径: %w", err)
	}

	// 检查路径是否在允许的目录内
	if !strings.HasPrefix(absPath, gt.baseDir) {
		return "", fmt.Errorf("访问被拒绝: 路径 %s 不在允许的目录 %s 内", absPath, gt.baseDir)
	}

	return absPath, nil
}

// ========== Git Init ==========

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

	// 确保目录存在
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

// ========== Git Status ==========

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

// ========== Git Add ==========

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

// ========== Git Commit ==========

// GitCommitRequest 提交请求
type GitCommitRequest struct {
	Path    string `json:"path,omitempty" jsonschema:"description=仓库路径(相对于工作目录)"`
	Message string `json:"message" jsonschema:"description=提交信息,required"`
	Author  string `json:"author,omitempty" jsonschema:"description=作者名称"`
	Email   string `json:"email,omitempty" jsonschema:"description=作者邮箱"`
	All     bool   `json:"all,omitempty" jsonschema:"description=是否自动暂存所有修改的文件"`
}

// GitCommitResponse 提交响应
type GitCommitResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	CommitHash   string `json:"commit_hash,omitempty" jsonschema:"description=提交哈希"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// GitCommit 创建一个新的提交
func (gt *GitTools) GitCommit(ctx context.Context, req *GitCommitRequest) (*GitCommitResponse, error) {
	path, err := gt.validatePath(req.Path)
	if err != nil {
		return &GitCommitResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.Message == "" {
		return &GitCommitResponse{
			ErrorMessage: "提交信息不能为空",
		}, nil
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		return &GitCommitResponse{
			ErrorMessage: fmt.Sprintf("无法打开仓库: %v", err),
		}, nil
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return &GitCommitResponse{
			ErrorMessage: fmt.Sprintf("无法获取工作树: %v", err),
		}, nil
	}

	// 设置作者信息
	author := req.Author
	email := req.Email
	if author == "" {
		author = "fkteams"
	}
	if email == "" {
		email = "fkteams@example.com"
	}

	commitHash, err := worktree.Commit(req.Message, &git.CommitOptions{
		All: req.All,
		Author: &object.Signature{
			Name:  author,
			Email: email,
			When:  time.Now(),
		},
	})
	if err != nil {
		return &GitCommitResponse{
			ErrorMessage: fmt.Sprintf("提交失败: %v", err),
		}, nil
	}

	return &GitCommitResponse{
		Message:    fmt.Sprintf("成功创建提交: %s", commitHash.String()[:7]),
		CommitHash: commitHash.String(),
	}, nil
}

// ========== Git Log ==========

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
	// 忽略 "limit reached" 错误
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

// ========== Git Branch ==========

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
		return gt.createBranch(repo, req.Name, req.StartPoint)
	case "delete":
		return gt.deleteBranch(repo, req.Name)
	default:
		return &GitBranchResponse{
			ErrorMessage: fmt.Sprintf("未知的操作类型: %s", action),
		}, nil
	}
}

func (gt *GitTools) listBranches(repo *git.Repository) (*GitBranchResponse, error) {
	// 获取当前分支
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
		// 尝试解析为提交哈希
		resolvedHash, err := repo.ResolveRevision(plumbing.Revision(startPoint))
		if err != nil {
			return &GitBranchResponse{
				ErrorMessage: fmt.Sprintf("无法解析起始点 %s: %v", startPoint, err),
			}, nil
		}
		hash = *resolvedHash
	} else {
		// 使用 HEAD
		head, err := repo.Head()
		if err != nil {
			return &GitBranchResponse{
				ErrorMessage: fmt.Sprintf("无法获取 HEAD: %v", err),
			}, nil
		}
		hash = head.Hash()
	}

	// 创建分支引用
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

	// 检查是否是当前分支
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

// ========== Git Checkout ==========

// GitCheckoutRequest 切换分支/提交请求
type GitCheckoutRequest struct {
	Path   string `json:"path,omitempty" jsonschema:"description=仓库路径(相对于工作目录)"`
	Branch string `json:"branch,omitempty" jsonschema:"description=要切换的分支名称"`
	Hash   string `json:"hash,omitempty" jsonschema:"description=要切换的提交哈希"`
	Create bool   `json:"create,omitempty" jsonschema:"description=如果分支不存在是否创建"`
	Force  bool   `json:"force,omitempty" jsonschema:"description=是否强制切换(丢弃本地修改)"`
}

// GitCheckoutResponse 切换分支/提交响应
type GitCheckoutResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// GitCheckout 切换分支或提交
func (gt *GitTools) GitCheckout(ctx context.Context, req *GitCheckoutRequest) (*GitCheckoutResponse, error) {
	path, err := gt.validatePath(req.Path)
	if err != nil {
		return &GitCheckoutResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		return &GitCheckoutResponse{
			ErrorMessage: fmt.Sprintf("无法打开仓库: %v", err),
		}, nil
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return &GitCheckoutResponse{
			ErrorMessage: fmt.Sprintf("无法获取工作树: %v", err),
		}, nil
	}

	opts := &git.CheckoutOptions{
		Force:  req.Force,
		Create: req.Create,
	}

	if req.Branch != "" {
		opts.Branch = plumbing.NewBranchReferenceName(req.Branch)
	}

	if req.Hash != "" {
		hash := plumbing.NewHash(req.Hash)
		opts.Hash = hash
	}

	err = worktree.Checkout(opts)
	if err != nil {
		return &GitCheckoutResponse{
			ErrorMessage: fmt.Sprintf("切换失败: %v", err),
		}, nil
	}

	var message string
	if req.Branch != "" {
		message = fmt.Sprintf("成功切换到分支 %s", req.Branch)
		if req.Create {
			message = fmt.Sprintf("成功创建并切换到分支 %s", req.Branch)
		}
	} else if req.Hash != "" {
		message = fmt.Sprintf("成功切换到提交 %s", req.Hash[:7])
	}

	return &GitCheckoutResponse{
		Message: message,
	}, nil
}

// ========== Git Reset ==========

// GitResetRequest 重置请求
type GitResetRequest struct {
	Path   string   `json:"path,omitempty" jsonschema:"description=仓库路径(相对于工作目录)"`
	Commit string   `json:"commit,omitempty" jsonschema:"description=要重置到的提交哈希(默认为HEAD)"`
	Mode   string   `json:"mode,omitempty" jsonschema:"description=重置模式: soft(只移动HEAD)/mixed(重置暂存区)/hard(重置暂存区和工作区),默认为mixed"`
	Files  []string `json:"files,omitempty" jsonschema:"description=要重置的文件列表(仅在重置特定文件时使用)"`
}

// GitResetResponse 重置响应
type GitResetResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// GitReset 重置仓库到指定状态
func (gt *GitTools) GitReset(ctx context.Context, req *GitResetRequest) (*GitResetResponse, error) {
	path, err := gt.validatePath(req.Path)
	if err != nil {
		return &GitResetResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		return &GitResetResponse{
			ErrorMessage: fmt.Sprintf("无法打开仓库: %v", err),
		}, nil
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return &GitResetResponse{
			ErrorMessage: fmt.Sprintf("无法获取工作树: %v", err),
		}, nil
	}

	// 获取目标提交
	var commitHash plumbing.Hash
	if req.Commit != "" {
		resolved, err := repo.ResolveRevision(plumbing.Revision(req.Commit))
		if err != nil {
			return &GitResetResponse{
				ErrorMessage: fmt.Sprintf("无法解析提交 %s: %v", req.Commit, err),
			}, nil
		}
		commitHash = *resolved
	} else {
		head, err := repo.Head()
		if err != nil {
			return &GitResetResponse{
				ErrorMessage: fmt.Sprintf("无法获取 HEAD: %v", err),
			}, nil
		}
		commitHash = head.Hash()
	}

	// 确定重置模式
	var mode git.ResetMode
	switch strings.ToLower(req.Mode) {
	case "soft":
		mode = git.SoftReset
	case "hard":
		mode = git.HardReset
	case "mixed", "":
		mode = git.MixedReset
	default:
		return &GitResetResponse{
			ErrorMessage: fmt.Sprintf("未知的重置模式: %s", req.Mode),
		}, nil
	}

	opts := &git.ResetOptions{
		Commit: commitHash,
		Mode:   mode,
		Files:  req.Files,
	}

	err = worktree.Reset(opts)
	if err != nil {
		return &GitResetResponse{
			ErrorMessage: fmt.Sprintf("重置失败: %v", err),
		}, nil
	}

	modeDesc := "mixed"
	if req.Mode != "" {
		modeDesc = strings.ToLower(req.Mode)
	}

	return &GitResetResponse{
		Message: fmt.Sprintf("成功重置到 %s (%s模式)", commitHash.String()[:7], modeDesc),
	}, nil
}

// ========== Git Tag ==========

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
		return gt.createTag(repo, req.Name, req.Message, req.Commit)
	case "delete":
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

		// 尝试获取附注标签信息
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
		// 创建附注标签
		_, err = repo.CreateTag(name, hash, &git.CreateTagOptions{
			Message: message,
			Tagger: &object.Signature{
				Name:  "fkteams",
				Email: "fkteams@example.com",
				When:  time.Now(),
			},
		})
	} else {
		// 创建轻量标签
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

// ========== Git Diff ==========

// GitDiffRequest 查看差异请求
type GitDiffRequest struct {
	Path   string `json:"path,omitempty" jsonschema:"description=仓库路径(相对于工作目录)"`
	Cached bool   `json:"cached,omitempty" jsonschema:"description=是否查看暂存区与HEAD的差异(默认查看工作区与暂存区的差异)"`
}

// FileDiff 文件差异信息
type FileDiff struct {
	Path      string `json:"path" jsonschema:"description=文件路径"`
	Status    string `json:"status" jsonschema:"description=状态"`
	Additions int    `json:"additions,omitempty" jsonschema:"description=新增行数"`
	Deletions int    `json:"deletions,omitempty" jsonschema:"description=删除行数"`
}

// GitDiffResponse 查看差异响应
type GitDiffResponse struct {
	Diffs        []FileDiff `json:"diffs,omitempty" jsonschema:"description=差异列表"`
	Message      string     `json:"message,omitempty" jsonschema:"description=结果消息"`
	ErrorMessage string     `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// GitDiff 查看差异
func (gt *GitTools) GitDiff(ctx context.Context, req *GitDiffRequest) (*GitDiffResponse, error) {
	path, err := gt.validatePath(req.Path)
	if err != nil {
		return &GitDiffResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		return &GitDiffResponse{
			ErrorMessage: fmt.Sprintf("无法打开仓库: %v", err),
		}, nil
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return &GitDiffResponse{
			ErrorMessage: fmt.Sprintf("无法获取工作树: %v", err),
		}, nil
	}

	status, err := worktree.Status()
	if err != nil {
		return &GitDiffResponse{
			ErrorMessage: fmt.Sprintf("无法获取状态: %v", err),
		}, nil
	}

	var diffs []FileDiff
	for filePath, fileStatus := range status {
		var statusStr string
		if req.Cached {
			// 查看暂存区的变化
			statusStr = statusCodeToString(fileStatus.Staging)
		} else {
			// 查看工作区的变化
			statusStr = statusCodeToString(fileStatus.Worktree)
		}

		if statusStr != "未修改" && statusStr != "" {
			diffs = append(diffs, FileDiff{
				Path:   filePath,
				Status: statusStr,
			})
		}
	}

	diffType := "工作区"
	if req.Cached {
		diffType = "暂存区"
	}

	return &GitDiffResponse{
		Diffs:   diffs,
		Message: fmt.Sprintf("%s有 %d 个文件有变更", diffType, len(diffs)),
	}, nil
}

// ========== Git Clean ==========

// GitCleanRequest 清理未跟踪文件请求
type GitCleanRequest struct {
	Path        string `json:"path,omitempty" jsonschema:"description=仓库路径(相对于工作目录)"`
	DryRun      bool   `json:"dry_run,omitempty" jsonschema:"description=是否只显示将要删除的文件而不实际删除"`
	Directories bool   `json:"directories,omitempty" jsonschema:"description=是否也删除未跟踪的目录"`
}

// GitCleanResponse 清理未跟踪文件响应
type GitCleanResponse struct {
	CleanedFiles []string `json:"cleaned_files,omitempty" jsonschema:"description=清理的文件列表"`
	Message      string   `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string   `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// GitClean 清理未跟踪的文件
func (gt *GitTools) GitClean(ctx context.Context, req *GitCleanRequest) (*GitCleanResponse, error) {
	path, err := gt.validatePath(req.Path)
	if err != nil {
		return &GitCleanResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		return &GitCleanResponse{
			ErrorMessage: fmt.Sprintf("无法打开仓库: %v", err),
		}, nil
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return &GitCleanResponse{
			ErrorMessage: fmt.Sprintf("无法获取工作树: %v", err),
		}, nil
	}

	status, err := worktree.Status()
	if err != nil {
		return &GitCleanResponse{
			ErrorMessage: fmt.Sprintf("无法获取状态: %v", err),
		}, nil
	}

	var untrackedFiles []string
	for filePath, fileStatus := range status {
		if fileStatus.Worktree == git.Untracked {
			untrackedFiles = append(untrackedFiles, filePath)
		}
	}

	if req.DryRun {
		return &GitCleanResponse{
			CleanedFiles: untrackedFiles,
			Message:      fmt.Sprintf("将要删除 %d 个未跟踪文件（干运行模式）", len(untrackedFiles)),
		}, nil
	}

	// 实际删除文件
	err = worktree.Clean(&git.CleanOptions{
		Dir: req.Directories,
	})
	if err != nil {
		return &GitCleanResponse{
			ErrorMessage: fmt.Sprintf("清理失败: %v", err),
		}, nil
	}

	return &GitCleanResponse{
		CleanedFiles: untrackedFiles,
		Message:      fmt.Sprintf("成功清理 %d 个未跟踪文件", len(untrackedFiles)),
	}, nil
}

// ========== Git Remove ==========

// GitRemoveRequest 从仓库移除文件请求
type GitRemoveRequest struct {
	Path  string   `json:"path,omitempty" jsonschema:"description=仓库路径(相对于工作目录)"`
	Files []string `json:"files" jsonschema:"description=要移除的文件列表,required"`
}

// GitRemoveResponse 从仓库移除文件响应
type GitRemoveResponse struct {
	RemovedFiles []string `json:"removed_files,omitempty" jsonschema:"description=移除的文件列表"`
	Message      string   `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string   `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// GitRemove 从仓库移除文件
func (gt *GitTools) GitRemove(ctx context.Context, req *GitRemoveRequest) (*GitRemoveResponse, error) {
	path, err := gt.validatePath(req.Path)
	if err != nil {
		return &GitRemoveResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if len(req.Files) == 0 {
		return &GitRemoveResponse{
			ErrorMessage: "请指定要移除的文件",
		}, nil
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		return &GitRemoveResponse{
			ErrorMessage: fmt.Sprintf("无法打开仓库: %v", err),
		}, nil
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return &GitRemoveResponse{
			ErrorMessage: fmt.Sprintf("无法获取工作树: %v", err),
		}, nil
	}

	var removedFiles []string
	for _, file := range req.Files {
		_, err := worktree.Remove(file)
		if err != nil {
			return &GitRemoveResponse{
				ErrorMessage: fmt.Sprintf("移除文件 %s 失败: %v", file, err),
			}, nil
		}
		removedFiles = append(removedFiles, file)
	}

	return &GitRemoveResponse{
		RemovedFiles: removedFiles,
		Message:      fmt.Sprintf("成功从暂存区移除 %d 个文件", len(removedFiles)),
	}, nil
}

// ========== Git Config ==========

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
		// 用户配置
		if cfg.User.Name != "" {
			configs = append(configs, ConfigItem{Key: "user.name", Value: cfg.User.Name})
		}
		if cfg.User.Email != "" {
			configs = append(configs, ConfigItem{Key: "user.email", Value: cfg.User.Email})
		}
		// 远程仓库配置
		for name, remote := range cfg.Remotes {
			if len(remote.URLs) > 0 {
				configs = append(configs, ConfigItem{Key: fmt.Sprintf("remote.%s.url", name), Value: remote.URLs[0]})
			}
		}
		// 分支配置
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
		// 简单的配置获取实现
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

// ========== Git Remote ==========

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
