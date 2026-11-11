package environment

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"fkteams/internal/runtime/atomicfile"

	"github.com/pelletier/go-toml/v2"
)

const (
	maxMirrorConfigBytes int64 = 1 << 20
	bunMirrorRegistry          = "https://registry.npmmirror.com"
	uvPythonMirror             = "https://gh-proxy.com/https://github.com/astral-sh/python-build-standalone/releases/download"
	uvPackageMirror            = "https://mirrors.aliyun.com/pypi/simple/"
)

func loadMirrorConfig(path string) ([]byte, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open mirror config: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxMirrorConfigBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read mirror config: %w", readErr)
	}
	if int64(len(data)) > maxMirrorConfigBytes {
		return nil, fmt.Errorf("mirror config exceeds %d bytes", maxMirrorConfigBytes)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close mirror config: %w", closeErr)
	}
	return data, nil
}

func mergeBunMirrorConfig(data []byte) ([]byte, bool, error) {
	config, err := decodeMirrorConfig(data)
	if err != nil {
		return nil, false, err
	}
	install, err := configTable(config, "install")
	if err != nil {
		return nil, false, err
	}
	if registry, ok := install["registry"].(string); ok && registry == bunMirrorRegistry {
		return data, false, nil
	}
	install["registry"] = bunMirrorRegistry
	config["install"] = install
	encoded, err := toml.Marshal(config)
	if err != nil {
		return nil, false, fmt.Errorf("encode bun mirror config: %w", err)
	}
	return encoded, true, validateMirrorConfigSize(encoded)
}

func mergeUVMirrorConfig(data []byte) ([]byte, bool, error) {
	config, err := decodeMirrorConfig(data)
	if err != nil {
		return nil, false, err
	}
	changed := false
	if mirror, ok := config["python-install-mirror"].(string); !ok || mirror != uvPythonMirror {
		config["python-install-mirror"] = uvPythonMirror
		changed = true
	}

	indexes, err := configTables(config, "index")
	if err != nil {
		return nil, false, err
	}
	found := false
	for _, index := range indexes {
		url, _ := index["url"].(string)
		isTarget := strings.TrimRight(url, "/") == strings.TrimRight(uvPackageMirror, "/")
		if isTarget {
			found = true
			if value, ok := index["default"].(bool); !ok || !value {
				index["default"] = true
				changed = true
			}
		} else if value, ok := index["default"].(bool); ok && value {
			index["default"] = false
			changed = true
		}
	}
	if !found {
		indexes = append(indexes, map[string]any{"url": uvPackageMirror, "default": true})
		changed = true
	}
	if !changed {
		return data, false, nil
	}
	config["index"] = indexes
	encoded, err := toml.Marshal(config)
	if err != nil {
		return nil, false, fmt.Errorf("encode uv mirror config: %w", err)
	}
	return encoded, true, validateMirrorConfigSize(encoded)
}

func decodeMirrorConfig(data []byte) (map[string]any, error) {
	config := make(map[string]any)
	if len(strings.TrimSpace(string(data))) == 0 {
		return config, nil
	}
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("decode mirror config: %w", err)
	}
	return config, nil
}

func configTable(config map[string]any, key string) (map[string]any, error) {
	value, exists := config[key]
	if !exists {
		return make(map[string]any), nil
	}
	table, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("mirror config key %q is not a table", key)
	}
	return table, nil
}

func configTables(config map[string]any, key string) ([]map[string]any, error) {
	value, exists := config[key]
	if !exists {
		return nil, nil
	}
	switch tables := value.(type) {
	case []map[string]any:
		return tables, nil
	case []any:
		result := make([]map[string]any, 0, len(tables))
		for _, value := range tables {
			table, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("mirror config key %q contains a non-table value", key)
			}
			result = append(result, table)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("mirror config key %q is not an array of tables", key)
	}
}

func validateMirrorConfigSize(data []byte) error {
	if int64(len(data)) > maxMirrorConfigBytes {
		return fmt.Errorf("mirror config exceeds %d bytes", maxMirrorConfigBytes)
	}
	return nil
}

func saveMirrorConfig(path string, data []byte) error {
	writePath := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		writePath = resolved
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("resolve mirror config: %w", err)
	}
	permission := os.FileMode(0644)
	if info, err := os.Stat(writePath); err == nil {
		permission = info.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect mirror config: %w", err)
	}
	if err := atomicfile.WriteFile(writePath, data, permission); err != nil {
		return fmt.Errorf("save mirror config: %w", err)
	}
	return nil
}
