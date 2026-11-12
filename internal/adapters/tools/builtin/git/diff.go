package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fkteams/internal/runtime/executil"
	"fkteams/internal/runtime/pathguard"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	fdiff "github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/plumbing/object"
	linediff "github.com/go-git/go-git/v5/utils/diff"
)

// GitDiffRequest 控制工作区或暂存区差异读取。
type GitDiffRequest struct {
	Path   string `json:"path,omitempty" jsonschema:"description=仓库路径（相对于工作区）"`
	Cached bool   `json:"cached,omitempty" jsonschema:"description=查看暂存区与HEAD的差异；默认查看工作区与暂存区的差异"`
}

// FileDiff 返回文件的真实增删行数和有界 unified diff。
type FileDiff struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Additions int    `json:"additions,omitempty"`
	Deletions int    `json:"deletions,omitempty"`
	Patch     string `json:"patch,omitempty"`
	Binary    bool   `json:"binary,omitempty"`
	Warning   string `json:"warning,omitempty"`
}

// GitDiffResponse 返回有序差异，并明确标记结果预算截断。
type GitDiffResponse struct {
	Diffs        []FileDiff `json:"diffs,omitempty"`
	Truncated    bool       `json:"truncated,omitempty"`
	Message      string     `json:"message,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
}

// GitDiff 直接比较 HEAD、index 与工作区内容，不修改 index 或仓库对象。
func (gt *GitTools) GitDiff(ctx context.Context, req *GitDiffRequest) (*GitDiffResponse, error) {
	path, err := gt.validatePath(req.Path)
	if err != nil {
		return &GitDiffResponse{ErrorMessage: err.Error()}, nil
	}
	repo, err := gt.openRepository(path)
	if err != nil {
		return &GitDiffResponse{ErrorMessage: err.Error()}, nil
	}
	wt, err := repo.Worktree()
	if err != nil {
		return &GitDiffResponse{ErrorMessage: err.Error()}, nil
	}
	status, err := wt.Status()
	if err != nil {
		return &GitDiffResponse{ErrorMessage: err.Error()}, nil
	}
	index, err := repo.Storer.Index()
	if err != nil {
		return &GitDiffResponse{ErrorMessage: err.Error()}, nil
	}
	var head *object.Tree
	if req.Cached {
		ref, err := repo.Head()
		if err == nil {
			commit, e := repo.CommitObject(ref.Hash())
			if e != nil {
				return &GitDiffResponse{ErrorMessage: e.Error()}, nil
			}
			head, err = commit.Tree()
		}
		if err != nil && !errors.Is(err, plumbing.ErrReferenceNotFound) {
			return &GitDiffResponse{ErrorMessage: err.Error()}, nil
		}
	}
	names := make([]string, 0, len(status))
	for name := range status {
		names = append(names, name)
	}
	sort.Strings(names)
	response := &GitDiffResponse{}
	remaining := 64 << 10
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		state := status[name].Worktree
		if req.Cached {
			state = status[name].Staging
		}
		if state == git.Unmodified || state == git.Untracked || state == 0 {
			continue
		}
		if len(response.Diffs) >= 100 || remaining <= 0 {
			response.Truncated = true
			break
		}
		item := FileDiff{Path: name, Status: statusCodeToString(state)}
		var indexHash plumbing.Hash
		indexMode := filemode.Regular
		for _, entry := range index.Entries {
			if entry.Name == name {
				indexHash = entry.Hash
				indexMode = entry.Mode
				break
			}
		}
		oldHash, newHash := indexHash, plumbing.ZeroHash
		oldMode, newMode := indexMode, indexMode
		var before, after string
		if req.Cached {
			oldHash = plumbing.ZeroHash
			if head != nil {
				file, e := head.File(name)
				if e == nil {
					oldHash = file.Hash
					oldMode = file.Mode
				} else if !errors.Is(e, object.ErrFileNotFound) {
					return &GitDiffResponse{ErrorMessage: e.Error()}, nil
				}
			}
			newHash = indexHash
			before, err = readDiffBlob(repo, oldHash)
			if err == nil {
				after, err = readDiffBlob(repo, newHash)
			}
		} else {
			before, err = readDiffBlob(repo, indexHash)
			if err == nil && state != git.Deleted {
				after, newMode, err = gt.readDiffWorktree(path, name)
			}
		}
		if err != nil {
			item.Warning = err.Error()
			response.Diffs = append(response.Diffs, item)
			continue
		}
		from, to := (*patchFile)(nil), (*patchFile)(nil)
		if oldHash != plumbing.ZeroHash {
			from = &patchFile{name, oldHash, oldMode}
		}
		if (req.Cached && newHash != plumbing.ZeroHash) || (!req.Cached && state != git.Deleted) {
			to = &patchFile{name, newHash, newMode}
		}
		filePatch := &textPatch{from: from, to: to, binary: strings.ContainsRune(before, 0) || strings.ContainsRune(after, 0)}
		item.Binary = filePatch.binary
		if !item.Binary {
			for _, change := range linediff.DoWithTimeout(before, after, 200*time.Millisecond) {
				operation := fdiff.Equal
				if change.Type < 0 {
					operation = fdiff.Delete
					item.Deletions += diffLineCount(change.Text)
				} else if change.Type > 0 {
					operation = fdiff.Add
					item.Additions += diffLineCount(change.Text)
				}
				filePatch.chunks = append(filePatch.chunks, patchChunk{change.Text, operation})
			}
		}
		writer := &patchWriter{limit: min(16<<10, remaining)}
		if err := fdiff.NewUnifiedEncoder(writer, 3).Encode(patchSet{filePatch}); err != nil {
			return &GitDiffResponse{ErrorMessage: err.Error()}, nil
		}
		item.Patch = executil.String(writer.Bytes(), writer.truncated)
		if writer.truncated {
			item.Warning = "patch truncated by output budget"
			response.Truncated = true
		}
		remaining -= len(item.Patch)
		response.Diffs = append(response.Diffs, item)
	}
	response.Message = fmt.Sprintf("返回 %d 个文件的差异", len(response.Diffs))
	return response, nil
}

func readDiffBlob(repo *git.Repository, hash plumbing.Hash) (string, error) {
	if hash == plumbing.ZeroHash {
		return "", nil
	}
	blob, err := repo.BlobObject(hash)
	if err != nil {
		return "", err
	}
	if blob.Size > 1<<20 {
		return "", fmt.Errorf("file exceeds 1 MB diff input limit")
	}
	reader, err := blob.Reader()
	if err != nil {
		return "", err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, (1<<20)+1))
	if len(data) > 1<<20 {
		return "", fmt.Errorf("file exceeds diff input limit")
	}
	return string(data), err
}

func (gt *GitTools) readDiffWorktree(repoPath, name string) (string, filemode.FileMode, error) {
	joined := filepath.Join(repoPath, filepath.FromSlash(name))
	// Git 符号链接的内容是链接目标文本，不能跟随读取目标文件。
	info, err := os.Lstat(joined)
	if err != nil {
		return "", 0, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(joined)
		return target, filemode.Symlink, err
	}
	resolved, err := pathguard.ResolveWorkspace(gt.baseDir, joined)
	if err != nil {
		return "", 0, err
	}
	root, err := os.OpenRoot(resolved.BaseAbs)
	if err != nil {
		return "", 0, err
	}
	defer root.Close()
	file, err := root.Open(resolved.RelPath)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return "", 0, fmt.Errorf("file exceeds regular-file diff limits")
	}
	data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if len(data) > 1<<20 {
		return "", 0, fmt.Errorf("file exceeds diff input limit")
	}
	mode := filemode.Regular
	if info.Mode()&0111 != 0 {
		mode = filemode.Executable
	}
	return string(data), mode, err
}

func diffLineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + boolInt(!strings.HasSuffix(s, "\n"))
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

type patchFile struct {
	name string
	hash plumbing.Hash
	mode filemode.FileMode
}

func (f *patchFile) Path() string            { return f.name }
func (f *patchFile) Hash() plumbing.Hash     { return f.hash }
func (f *patchFile) Mode() filemode.FileMode { return f.mode }

type patchChunk struct {
	text      string
	operation fdiff.Operation
}

func (c patchChunk) Content() string       { return c.text }
func (c patchChunk) Type() fdiff.Operation { return c.operation }

type textPatch struct {
	from, to *patchFile
	chunks   []fdiff.Chunk
	binary   bool
}

func (p *textPatch) Files() (fdiff.File, fdiff.File) {
	var from, to fdiff.File
	if p.from != nil {
		from = p.from
	}
	if p.to != nil {
		to = p.to
	}
	return from, to
}
func (p *textPatch) Chunks() []fdiff.Chunk { return p.chunks }
func (p *textPatch) IsBinary() bool        { return p.binary }

type patchSet struct{ file fdiff.FilePatch }

func (p patchSet) FilePatches() []fdiff.FilePatch { return []fdiff.FilePatch{p.file} }
func (p patchSet) Message() string                { return "" }

type patchWriter struct {
	bytes.Buffer
	limit     int
	truncated bool
}

func (w *patchWriter) Write(p []byte) (int, error) {
	n := len(p)
	remaining := w.limit - w.Len()
	if len(p) > remaining {
		p = p[:max(0, remaining)]
		w.truncated = true
	}
	_, err := w.Buffer.Write(p)
	return n, err
}
