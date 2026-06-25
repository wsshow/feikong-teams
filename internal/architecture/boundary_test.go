package architecture

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInternalLayerBoundaries(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
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
			assertBoundary(t, rel, importPath)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func assertBoundary(t *testing.T, rel, importPath string) {
	switch {
	case strings.HasPrefix(rel, "internal/domain/"):
		forbidden := []string{
			"fkteams/internal/app",
			"fkteams/internal/adapters",
			"fkteams/agentcore/eino",
			"github.com/cloudwego/eino",
			"github.com/gin-gonic/gin",
		}
		assertNotImported(t, rel, importPath, forbidden)
	case strings.HasPrefix(rel, "internal/ports/"):
		forbidden := []string{
			"fkteams/internal/app",
			"fkteams/internal/adapters",
			"fkteams/agentcore/eino",
			"github.com/cloudwego/eino",
			"github.com/gin-gonic/gin",
		}
		assertNotImported(t, rel, importPath, forbidden)
	case strings.HasPrefix(rel, "internal/app/"):
		forbidden := []string{
			"fkteams/internal/adapters",
			"fkteams/agentcore/eino",
			"github.com/cloudwego/eino",
			"github.com/gin-gonic/gin",
		}
		assertNotImported(t, rel, importPath, forbidden)
	}
}

func TestTurnSessionCreationStaysInsideChatUseCase(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	allowed := map[string]bool{
		"internal/app/chat/service.go": true,
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
			if filepath.ToSlash(path) == filepath.ToSlash(filepath.Join(root, "internal", "runtime", "turn")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if allowed[rel] {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(content)
		if strings.Contains(text, "engine.NewSession") || strings.Contains(text, "turn.NewSession") {
			t.Errorf("%s creates turn session directly; use internal/app/chat.Service", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestChatInputBuilderStaysInsideChatUseCase(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
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
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
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
			if strings.Trim(spec.Path.Value, `"`) == "fkteams/events/chat" {
				t.Errorf("%s imports events/chat; use internal/app/chat for input building", rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunnerFactoryStaysInsideAgentUseCase(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
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
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
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
			if strings.Trim(spec.Path.Value, `"`) == "fkteams/runner" {
				t.Errorf("%s imports removed runner package; use internal/app/agent", rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func assertNotImported(t *testing.T, rel, importPath string, forbidden []string) {
	t.Helper()
	for _, prefix := range forbidden {
		if strings.HasPrefix(importPath, prefix) {
			t.Errorf("%s imports forbidden dependency %s", rel, importPath)
		}
	}
}
