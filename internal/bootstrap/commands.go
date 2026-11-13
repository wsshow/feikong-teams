package bootstrap

import (
	"context"
	commands "fkteams/internal/adapters/transport/cli/commands"
	httpserver "fkteams/internal/adapters/transport/http"
	"fkteams/internal/app/lifecycle"
	appschedule "fkteams/internal/app/schedule"
	"fkteams/internal/bootstrap/services"
	"fkteams/internal/bootstrap/skills"
	ucli "github.com/urfave/cli/v3"
)

// Command 装配 CLI 及其应用入口。
func (d *ExecutionDependencies) Command() *ucli.Command {
	return commands.Root(commands.Options{PrepareStorage: func(ctx context.Context) (context.Context, error) {
		return d.prepareStorage(ctx, httpserver.ModeAPI, nil)
	}, CloseStorage: d.closeStorage, RunServer: d.RunServer, ConfigureServices: d.configureServices, SkillProviders: skills.NewDefaultProviderRegistry()})
}

func (d *ExecutionDependencies) configureServices(app *lifecycle.Application) func() *appschedule.Service {
	app.OnCleanup(func(context.Context) error {
		if d.ToolRegistry != nil {
			d.ToolRegistry.ClearMCPToolCache()
		}
		return d.closeStorage()
	})
	if d.Queues != nil {
		app.RegisterService(d.Queues)
	}
	if d.Processes != nil {
		app.RegisterService(d.Processes)
	}
	cfg := app.Config()
	memoryService := services.NewMemoryService(cfg.WorkspaceDir, app.State(), d.MemoryStore)
	memoryService.Enabled = cfg.MemoryEnabled
	app.RegisterService(memoryService)
	if cfg.SchedulerEnabled {
		scheduler := services.NewSchedulerService(cfg.SchedulerDir, d.ExecutionGate)
		scheduler.Documents = d.Documents
		app.RegisterService(scheduler)
		return scheduler.AppService
	}
	return nil
}
