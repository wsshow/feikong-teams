package agentsmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	einoruntime "fkteams/internal/adapters/runtime/eino"
	"fkteams/internal/adapters/runtime/eino/middlewares/fkfs"
	"fkteams/internal/app/appdata"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/log"

	einoagentsmd "github.com/cloudwego/eino/adk/middlewares/agentsmd"
)

const allAgentsMDMaxBytes = 256 * 1024

var defaultAgentsMDFiles = []string{"AGENTS.md", "Agents.md"}

func New(ctx context.Context) (runtimeport.AgentMiddleware, error) {
	backend, err := fkfs.NewLocalBackend(appdata.WorkspaceDir())
	if err != nil {
		return nil, fmt.Errorf("create agents.md backend: %w", err)
	}
	middleware, err := einoagentsmd.New(ctx, &einoagentsmd.Config{
		Backend:             backend,
		AgentsMDFiles:       defaultAgentsMDFiles,
		AllAgentsMDMaxBytes: allAgentsMDMaxBytes,
		OnLoadWarning:       onLoadWarning,
	})
	if err != nil {
		return nil, err
	}
	return einoruntime.WrapAgentMiddleware("agentsmd", middleware), nil
}

func onLoadWarning(filePath string, err error) {
	if isMissingAgentsMD(err) {
		return
	}
	log.Warnf("[agentsmd] load warning for %s: %v", filePath, err)
}

func isMissingAgentsMD(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrNotExist) || os.IsNotExist(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "file not found") || strings.Contains(msg, "no such file")
}
