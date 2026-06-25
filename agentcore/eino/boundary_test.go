package eino

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestEinoImportsStayInsideAdapter(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	adapterRoots := []string{
		filepath.ToSlash(filepath.Join("agentcore", "eino")) + "/",
	}
	einoPrefix := "github.com/cloudwego/" + "eino"
	localAdapterPrefix := "fkteams/agentcore/eino"
	localAdapterConsumers := []string{
		filepath.ToSlash(filepath.Join("bootstrap", "runtimes")) + "/",
	}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "release", "node_modules", "web":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			importPath := strings.Trim(spec.Path.Value, `"`)
			if strings.HasPrefix(importPath, einoPrefix) && !isPathUnderAny(rel, adapterRoots) {
				t.Errorf("%s imports %s outside adapter packages", rel, importPath)
			}
			if strings.HasPrefix(importPath, localAdapterPrefix) &&
				!isPathUnderAny(rel, adapterRoots) &&
				!isPathUnderAny(rel, localAdapterConsumers) {
				t.Errorf("%s imports %s outside adapter packages", rel, importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func isPathUnderAny(path string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
