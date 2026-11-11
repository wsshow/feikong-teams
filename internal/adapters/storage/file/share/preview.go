package share

import (
	"encoding/json"
	"fkteams/internal/app/appdata"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PreviewLinkStore 保存单个 HTTP runtime 的预览分享链接。
type PreviewLinkStore struct{ *entryStore[PreviewEntry] }

const shareFileName = "share.json"

// shareFileEntry JSON 持久化条目
type shareFileEntry struct {
	FilePaths     []string `json:"file_paths"`
	ResourcePaths []string `json:"resource_paths"`
	PasswordHash  string   `json:"password_hash,omitempty"`
	ExpiresAt     int64    `json:"expires_at"` // Unix 时间戳，0 表示永不过期
	CreatedAt     int64    `json:"created_at"`
}

// NewPreviewLinkStore 创建预览分享存储，并从持久化文件加载现有链接。
func NewPreviewLinkStore(filePath string) *PreviewLinkStore {
	if filePath == "" {
		filePath = shareLinksFilePath()
	}
	store := &PreviewLinkStore{newEntryStore(filePath, clonePreviewEntry, validatePreviewLinkEntry, encodePreviewEntryMap)}
	store.loadErr = store.Load()
	return store
}

func shareLinksFilePath() string {
	return filepath.Join(appdata.ShareDir(), shareFileName)
}

// Load 从持久化文件加载未过期的预览分享链接。
func (s *PreviewLinkStore) Load() error {
	if s == nil {
		return nil
	}
	entries, err := readShareEntries(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("load preview links: %w", err)
	}
	loaded := make(map[string]*PreviewEntry, len(entries))
	now := time.Now()
	for id, e := range entries {
		if e == nil {
			return fmt.Errorf("invalid preview link entry: %s", id)
		}
		var expiresAt time.Time
		if e.ExpiresAt > 0 {
			expiresAt = time.Unix(e.ExpiresAt, 0)
			if now.After(expiresAt) {
				continue // 跳过已过期
			}
		}
		entry := NewPreviewEntry(
			e.FilePaths,
			e.ResourcePaths,
			e.PasswordHash,
			expiresAt,
			time.Unix(e.CreatedAt, 0),
		)
		if err := validatePreviewLinkEntry(id, entry); err != nil {
			return err
		}
		loaded[id] = entry
	}
	s.mu.Lock()
	s.m = loaded
	s.mu.Unlock()
	return nil
}

func readShareEntries(filePath string) (map[string]*shareFileEntry, error) {
	data, err := readPersistedShareStore(filePath)
	if err != nil {
		return nil, err
	}
	var entries map[string]*shareFileEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	if len(entries) > maxPersistedShareEntries {
		return nil, errShareStoreFull
	}
	return entries, nil
}

func encodePreviewEntryMap(values map[string]*PreviewEntry) ([]byte, error) {
	entries := make(map[string]*shareFileEntry, len(values))
	for id, e := range values {
		entries[id] = &shareFileEntry{
			FilePaths:     e.FilePaths,
			ResourcePaths: e.ResourcePaths,
			PasswordHash:  e.PasswordHash,
			ExpiresAt:     ExpiresAtUnix(e.ExpiresAt),
			CreatedAt:     e.CreatedAt.Unix(),
		}
	}
	return json.MarshalIndent(entries, "", "  ")
}

// PreviewEntry 存储条目
type PreviewEntry struct {
	FilePaths     []string // 用户显式分享的工作区相对路径列表
	ResourcePaths []string // 创建链接时授权的普通文件清单
	PasswordHash  string   // bcrypt 哈希 (空字符串表示无密码)
	ExpiresAt     time.Time
	CreatedAt     time.Time
	resourceSet   map[string]struct{}
}

func NewPreviewEntry(filePaths, resourcePaths []string, passwordHash string, expiresAt, createdAt time.Time) *PreviewEntry {
	var copiedResources []string
	if resourcePaths != nil {
		copiedResources = append([]string{}, resourcePaths...)
	}
	entry := &PreviewEntry{
		FilePaths:     append([]string(nil), filePaths...),
		ResourcePaths: copiedResources,
		PasswordHash:  passwordHash,
		ExpiresAt:     expiresAt,
		CreatedAt:     createdAt,
	}
	entry.indexResources()
	return entry
}

func validatePreviewLinkEntry(id string, entry *PreviewEntry) error {
	if id == "" || entry == nil {
		return fmt.Errorf("invalid preview link entry")
	}
	if len(entry.FilePaths) == 0 || len(entry.FilePaths) > MaxPreviewResourceFiles {
		return fmt.Errorf("invalid preview link file paths")
	}
	if len(entry.ResourcePaths) > MaxPreviewResourceFiles {
		return fmt.Errorf("invalid preview link resource paths")
	}
	for _, path := range entry.FilePaths {
		if path == "" {
			return fmt.Errorf("invalid preview link file path")
		}
	}
	return nil
}

func (e *PreviewEntry) indexResources() {
	if e == nil {
		return
	}
	paths := e.ResourcePaths
	if paths == nil {
		paths = e.FilePaths
	}
	e.resourceSet = make(map[string]struct{}, len(paths))
	for _, resourcePath := range paths {
		e.resourceSet[filepath.Clean(resourcePath)] = struct{}{}
	}
}

func (e *PreviewEntry) AllowsResource(resourcePath string) bool {
	if e == nil {
		return false
	}
	cleanPath := filepath.Clean(resourcePath)
	if !e.ResourceWithinSharedRoots(cleanPath) {
		return false
	}
	if e.resourceSet != nil {
		_, ok := e.resourceSet[cleanPath]
		return ok
	}
	paths := e.ResourcePaths
	if paths == nil {
		paths = e.FilePaths
	}
	for _, allowedPath := range paths {
		if filepath.Clean(allowedPath) == cleanPath {
			return true
		}
	}
	return false
}

func (e *PreviewEntry) ResourceWithinSharedRoots(resourcePath string) bool {
	for _, rootPath := range e.FilePaths {
		cleanRoot := filepath.Clean(rootPath)
		if rootPath == "" || cleanRoot == "." {
			return true
		}
		if resourcePath == cleanRoot || strings.HasPrefix(resourcePath, cleanRoot+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

const MaxPreviewResourceFiles = 10000

func clonePreviewEntry(entry *PreviewEntry) *PreviewEntry {
	return NewPreviewEntry(entry.FilePaths, entry.ResourcePaths, entry.PasswordHash, entry.ExpiresAt, entry.CreatedAt)
}
