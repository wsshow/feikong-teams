package agentsmd

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"fkteams/agentcore"
	"fkteams/common"
	einoruntime "fkteams/internal/adapters/runtime/eino"
	"fkteams/internal/adapters/runtime/eino/middlewares/fkfs"

	einoagentsmd "github.com/cloudwego/eino/adk/middlewares/agentsmd"
)

const allAgentsMDMaxBytes = 256 * 1024

var defaultAgentsMDFiles = []string{"AGENTS.md", "Agents.md"}

func New(ctx context.Context) (agentcore.AgentMiddleware, error) {
	backend, err := fkfs.NewLocalBackend(common.WorkspaceDir())
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
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	log.Printf("[agentsmd] load warning for %s: %v", filePath, err)
}
