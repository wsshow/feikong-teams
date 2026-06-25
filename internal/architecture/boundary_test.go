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

func TestAgentcoreFacadeIsRemoved(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	if _, err := os.Stat(filepath.Join(root, "agentcore")); err == nil {
		t.Fatal("agentcore facade directory exists; use internal/domain/* and internal/ports/runtime directly")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestRootLifecyclePackageIsRemoved(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	if _, err := os.Stat(filepath.Join(root, "lifecycle")); err == nil {
		t.Fatal("root lifecycle package exists; use internal/app/lifecycle and internal/bootstrap/services")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestRootAppStatePackageIsRemoved(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	if _, err := os.Stat(filepath.Join(root, "appstate")); err == nil {
		t.Fatal("root appstate package exists; use internal/app/appstate")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestRootBootstrapPackageIsRemoved(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	if _, err := os.Stat(filepath.Join(root, "bootstrap")); err == nil {
		t.Fatal("root bootstrap package exists; use internal/bootstrap/*")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestRootProvidersPackageIsRemoved(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	if _, err := os.Stat(filepath.Join(root, "providers")); err == nil {
		t.Fatal("root providers package exists; use internal/adapters/model/providers")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestRootEnvPackageIsRemoved(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	if _, err := os.Stat(filepath.Join(root, "fkenv")); err == nil {
		t.Fatal("root fkenv package exists; use internal/runtime/env")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestRootLogPackageIsRemoved(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	if _, err := os.Stat(filepath.Join(root, "log")); err == nil {
		t.Fatal("root log package exists; use internal/runtime/log")
	} else if !os.IsNotExist(err) {
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
	if importPath == "fkteams/lifecycle" {
		t.Errorf("%s imports removed root lifecycle package; use internal/app/lifecycle", rel)
	}
	if importPath == "fkteams/appstate" {
		t.Errorf("%s imports removed root appstate package; use internal/app/appstate", rel)
	}
	if importPath == "fkteams/bootstrap" || strings.HasPrefix(importPath, "fkteams/bootstrap/") {
		t.Errorf("%s imports removed root bootstrap package; use internal/bootstrap/*", rel)
	}
	if importPath == "fkteams/providers" || strings.HasPrefix(importPath, "fkteams/providers/") {
		t.Errorf("%s imports removed root providers package; use internal/adapters/model/providers", rel)
	}
	if importPath == "fkteams/fkenv" {
		t.Errorf("%s imports removed root fkenv package; use internal/runtime/env", rel)
	}
	if importPath == "fkteams/log" {
		t.Errorf("%s imports removed root log package; use internal/runtime/log", rel)
	}
	if strings.HasPrefix(rel, "internal/") && importPath == "fkteams/events" {
		t.Errorf("%s imports root events facade; use internal/runtime/events inside internal packages", rel)
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

func TestEventViewUsesDomainTypes(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	err := filepath.WalkDir(filepath.Join(root, "events", "view"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
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
			if strings.Trim(spec.Path.Value, `"`) == "fkteams/agentcore" {
				t.Errorf("%s imports agentcore; event views must use internal/domain/event and internal/domain/message", rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
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

func TestSchedulerBoundariesUseAppAndAdapters(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	legacyDir := filepath.Join(root, "tools", "scheduler")
	if entries, err := os.ReadDir(legacyDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
				t.Errorf("tools/scheduler/%s exists; scheduler tool adapter belongs under internal/adapters/tools/builtin/scheduler", entry.Name())
			}
		}
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
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
			importPath := strings.Trim(spec.Path.Value, `"`)
			if importPath == "fkteams/tools/scheduler" {
				t.Errorf("%s imports removed tools/scheduler package; use app/schedule or scheduler adapters", rel)
			}
			if strings.HasPrefix(rel, "internal/adapters/tools/builtin/scheduler/") &&
				strings.HasPrefix(importPath, "fkteams/internal/adapters/scheduler/") {
				t.Errorf("%s imports scheduler storage adapter; tool adapter must call app/schedule service", rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	registryPath := filepath.Join(root, "tools", "registry.go")
	content, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, forbidden := range []string{"InitGlobal(", "NewScheduler("} {
		if strings.Contains(text, forbidden) {
			t.Errorf("tools/registry.go initializes scheduler with %s; scheduler lifecycle belongs to service composition", forbidden)
		}
	}
}

func TestTaskStreamLivesInChatUseCase(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	legacyDir := filepath.Join(root, "server", "handler", "taskstream")
	if entries, err := os.ReadDir(legacyDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
				t.Errorf("server/handler/taskstream/%s exists; task stream state belongs under internal/app/chat/taskstream", entry.Name())
			}
		}
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
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
			if strings.Trim(spec.Path.Value, `"`) == "fkteams/server/handler/taskstream" {
				t.Errorf("%s imports removed server/handler/taskstream package; use internal/app/chat/taskstream", rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRootCommonPackageIsNotUsed(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	if _, err := os.Stat(filepath.Join(root, "common")); err == nil {
		t.Fatal("root common directory exists; use explicit internal packages")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, legacy := range []string{
		"common/common.go",
		"common/input_history.go",
		"common/memory.go",
		"common/resource_cleaner.go",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(legacy))); err == nil {
			t.Errorf("%s exists; split root common responsibilities into explicit packages", legacy)
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
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
			importPath := strings.Trim(spec.Path.Value, `"`)
			if importPath == "fkteams/common" || strings.HasPrefix(importPath, "fkteams/common/") {
				t.Errorf("%s imports removed root common package; use explicit internal packages", rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestToolsUseRuntimePortsDirectly(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	err := filepath.WalkDir(filepath.Join(root, "tools"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
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
			if strings.Trim(spec.Path.Value, `"`) == "fkteams/agentcore" {
				t.Errorf("%s imports agentcore; tools must use internal/ports/runtime and domain types directly", rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAgentsUseRuntimePortsDirectly(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	err := filepath.WalkDir(filepath.Join(root, "agents"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
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
			if strings.Trim(spec.Path.Value, `"`) == "fkteams/agentcore" {
				t.Errorf("%s imports agentcore; agents must use internal/ports/runtime directly", rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestModelProvidersUseRuntimePortsDirectly(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	for _, dir := range []string{
		"internal/adapters/model/providers",
		"internal/adapters/runtime/eino/providers",
	} {
		err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(dir)), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
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
				if strings.Trim(spec.Path.Value, `"`) == "fkteams/agentcore" {
					t.Errorf("%s imports agentcore; model providers must use internal/ports/runtime directly", rel)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestMemoryAndTestModelUseDomainAndRuntimePorts(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	for _, dir := range []string{
		"memory",
		"internal/testmodel",
	} {
		err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(dir)), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
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
				if strings.Trim(spec.Path.Value, `"`) == "fkteams/agentcore" {
					t.Errorf("%s imports agentcore; memory and test models must use domain types and runtime ports directly", rel)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestChannelsUseDomainAndRuntimePorts(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	err := filepath.WalkDir(filepath.Join(root, "channels"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
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
			if strings.Trim(spec.Path.Value, `"`) == "fkteams/agentcore" {
				t.Errorf("%s imports agentcore; channels must use internal/ports/runtime and domain types directly", rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCLIEntrypointsUseDomainAndRuntimePorts(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	for _, dir := range []string{
		"cli",
		"commands",
	} {
		err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(dir)), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
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
				if strings.Trim(spec.Path.Value, `"`) == "fkteams/agentcore" {
					t.Errorf("%s imports agentcore; CLI entrypoints must use internal/ports/runtime and domain types directly", rel)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestServerHandlersUseDomainAndRuntimePorts(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	err := filepath.WalkDir(filepath.Join(root, "server", "handler"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
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
			if strings.Trim(spec.Path.Value, `"`) == "fkteams/agentcore" {
				t.Errorf("%s imports agentcore; server handlers must use internal/ports/runtime and domain types directly", rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestEinoAdapterUsesDomainAndRuntimePorts(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	err := filepath.WalkDir(filepath.Join(root, "internal", "adapters", "runtime", "eino"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
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
			if strings.Trim(spec.Path.Value, `"`) == "fkteams/agentcore" {
				t.Errorf("%s imports agentcore; Eino adapter must use internal/ports/runtime and domain types directly", rel)
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
