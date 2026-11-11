package commands

import (
	"fkteams/internal/adapters/transport/cli/commands/skill"
	"fkteams/internal/app/config"

	ucli "github.com/urfave/cli/v3"
)

// skillCommand 创建 skill 子命令
func skillCommand(opts Options) *ucli.Command {
	return skill.Command(config.Init, opts.SkillProviders)
}
