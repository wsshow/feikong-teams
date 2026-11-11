package javascript

import (
	"encoding/json"
	"errors"
	storageport "fkteams/internal/ports/storage"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"

	"fkteams/internal/runtime/atomicfile"
)

const (
	maxStateFileBytes = 1 << 20
	maxStateKeys      = 256
	maxStateKeyBytes  = 128
)

// FileStateStore 将每个脚本的状态原子持久化到独立 JSON 文件。
type FileStateStore struct {
	Documents storageport.Documents
	root      string
	mu        sync.RWMutex
}

// NewFileStateStore 创建脚本状态存储。
func NewFileStateStore(root string, stores ...storageport.Documents) (*FileStateStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("javascript state directory is empty")
	}
	s := &FileStateStore{root: filepath.Clean(root)}
	if len(stores) > 0 {
		s.Documents = stores[0]
	}
	return s, nil
}

func (s *FileStateStore) Get(namespace, key string) (any, bool, error) {
	if err := validateStateKey(key); err != nil {
		return nil, false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, err := s.read(namespace)
	if err != nil {
		return nil, false, err
	}
	value, exists := state[key]
	return value, exists, nil
}

func (s *FileStateStore) Set(namespace, key string, value any) error {
	if err := validateStateKey(key); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.read(namespace)
	if err != nil {
		return err
	}
	if _, exists := state[key]; !exists && len(state) >= maxStateKeys {
		return fmt.Errorf("javascript state exceeds %d keys", maxStateKeys)
	}
	state[key] = value
	return s.write(namespace, state)
}

func (s *FileStateStore) Delete(namespace, key string) (bool, error) {
	if err := validateStateKey(key); err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.read(namespace)
	if err != nil {
		return false, err
	}
	if _, exists := state[key]; !exists {
		return false, nil
	}
	delete(state, key)
	return true, s.write(namespace, state)
}

func (s *FileStateStore) Keys(namespace string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, err := s.read(namespace)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(state))
	for key := range state {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

func (s *FileStateStore) read(namespace string) (map[string]any, error) {
	path, err := s.path(namespace)
	if err != nil {
		return nil, err
	}
	data, err := s.readBytes(path)
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string]any), nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) > maxStateFileBytes {
		return nil, fmt.Errorf("javascript state exceeds %d bytes", maxStateFileBytes)
	}
	state := make(map[string]any)
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("decode javascript state: %w", err)
	}
	return state, nil
}

func (s *FileStateStore) write(namespace string, state map[string]any) error {
	path, err := s.path(namespace)
	if err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode javascript state: %w", err)
	}
	if len(data) > maxStateFileBytes {
		return fmt.Errorf("javascript state exceeds %d bytes", maxStateFileBytes)
	}
	if s.Documents != nil {
		return s.Documents.Write(path, data)
	}
	if err := atomicfile.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write javascript state: %w", err)
	}
	return nil
}

func (s *FileStateStore) path(namespace string) (string, error) {
	if !validNamespace(namespace) {
		return "", fmt.Errorf("javascript state namespace is invalid")
	}
	return filepath.Join(s.root, namespace+".json"), nil
}

func validateStateKey(key string) error {
	if key == "" || len(key) > maxStateKeyBytes {
		return fmt.Errorf("javascript state key must contain 1 to %d bytes", maxStateKeyBytes)
	}
	for _, char := range key {
		if unicode.IsControl(char) {
			return fmt.Errorf("javascript state key contains control characters")
		}
	}
	return nil
}

func validNamespace(namespace string) bool {
	if namespace == "" || len(namespace) > 64 {
		return false
	}
	for index, char := range namespace {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' {
			continue
		}
		if index > 0 && (char >= '0' && char <= '9' || char == '_' || char == '-') {
			continue
		}
		return false
	}
	return true
}

func (s *FileStateStore) readBytes(path string) ([]byte, error) {
	if s.Documents != nil {
		return s.Documents.Read(path)
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, fmt.Errorf("open javascript state: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxStateFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read javascript state: %w", err)
	}
	return data, nil
}
