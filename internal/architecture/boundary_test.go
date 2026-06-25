package architecture

import (
	"go/ast"
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
			"fkteams/internal/adapters/runtime/eino",
			"github.com/cloudwego/eino",
			"github.com/gin-gonic/gin",
		}
		assertNotImported(t, rel, importPath, forbidden)
	case strings.HasPrefix(rel, "internal/ports/"):
		forbidden := []string{
			"fkteams/internal/app",
			"fkteams/internal/adapters",
			"fkteams/internal/adapters/runtime/eino",
			"github.com/cloudwego/eino",
			"github.com/gin-gonic/gin",
		}
		assertNotImported(t, rel, importPath, forbidden)
	case strings.HasPrefix(rel, "internal/app/"):
		forbidden := []string{
			"fkteams/agentcore",
			"fkteams/internal/adapters",
			"fkteams/internal/adapters/runtime/eino",
			"github.com/cloudwego/eino",
			"github.com/gin-gonic/gin",
		}
		assertNotImported(t, rel, importPath, forbidden)
	case strings.HasPrefix(rel, "internal/runtime/"):
		forbidden := []string{
			"fkteams/agentcore",
			"fkteams/internal/adapters",
			"fkteams/internal/adapters/runtime/eino",
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

func TestRuntimeRegistryUsesInternalPackage(t *testing.T) {
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
			if strings.Trim(spec.Path.Value, `"`) == "fkteams/agentcore/runtime" {
				t.Errorf("%s imports removed agentcore/runtime package; use internal/runtime/registry", rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestHooksUseInternalPackages(t *testing.T) {
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
			if strings.Trim(spec.Path.Value, `"`) == "fkteams/hooks" {
				t.Errorf("%s imports removed hooks package; use internal/ports/hooks or internal/runtime/hooks", rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRootEventsUseDomainTypes(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	eventsDir := filepath.Join(root, "events")
	entries, err := os.ReadDir(eventsDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		path := filepath.Join(eventsDir, entry.Name())
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		rel := filepath.ToSlash(filepath.Join("events", entry.Name()))
		for _, spec := range file.Imports {
			if strings.Trim(spec.Path.Value, `"`) == "fkteams/agentcore" {
				t.Errorf("%s imports agentcore; root events must use internal/domain/event and internal/domain/message", rel)
			}
		}
	}
}

func TestHistoryLogUsesStorageAdapter(t *testing.T) {
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
			if strings.Trim(spec.Path.Value, `"`) == "fkteams/events/log" {
				t.Errorf("%s imports removed events/log package; use internal/adapters/storage/file/history", rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestToolResolutionUsesRegistry(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	path := filepath.Join(root, "tools", "tools.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "GetToolsByNameWithCleaner" {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			if _, ok := node.(*ast.SwitchStmt); ok {
				t.Errorf("tools/tools.go GetToolsByNameWithCleaner uses switch; register tool groups through ToolGroupRegistry")
				return false
			}
			return true
		})
		return
	}
	t.Fatal("GetToolsByNameWithCleaner not found")
}

func assertNotImported(t *testing.T, rel, importPath string, forbidden []string) {
	t.Helper()
	for _, prefix := range forbidden {
		if strings.HasPrefix(importPath, prefix) {
			t.Errorf("%s imports forbidden dependency %s", rel, importPath)
		}
	}
}
