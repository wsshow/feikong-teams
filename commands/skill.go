package commands

import (
	"fkteams/commands/skill"
	"fkteams/config"

	ucli "github.com/urfave/cli/v3"
)

// skillCommand 创建 skill 子命令
func skillCommand() *ucli.Command {
	return skill.Command(config.Init)
}
