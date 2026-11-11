package bootstrap

import (
	"context"
	commands "fkteams/internal/adapters/transport/cli/commands"
	"fkteams/internal/app/lifecycle"
	appschedule "fkteams/internal/app/schedule"
	"fkteams/internal/bootstrap/services"
	"fkteams/internal/bootstrap/skills"
	ucli "github.com/urfave/cli/v3"
)

// Command 装配 CLI 及其应用入口。
func (d *ExecutionDependencies) Command() *ucli.Command {
	return commands.Root(commands.Options{RunServer: d.RunServer, ConfigureServices: d.configureServices, SkillProviders: skills.NewDefaultProviderRegistry()})
}

func (d *ExecutionDependencies) configureServices(app *lifecycle.Application) func() *appschedule.Service {
	app.OnCleanup(func(context.Context) error {
		if d.ToolRegistry != nil {
			d.ToolRegistry.ClearMCPToolCache()
		}
		return nil
	})
	cfg := app.Config()
	if cfg.MemoryEnabled {
		app.RegisterService(services.NewMemoryService(cfg.WorkspaceDir, app.State()))
	}
	if cfg.SchedulerEnabled {
		scheduler := services.NewSchedulerService(cfg.SchedulerDir, d.ExecutionGate)
		app.RegisterService(scheduler)
		return scheduler.AppService
	}
	return nil
}
