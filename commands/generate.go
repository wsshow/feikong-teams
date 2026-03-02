package commands

import (
	"context"
	"fkteams/common"
	"fkteams/config"
	"fmt"

	ucli "github.com/urfave/cli/v3"
)

// generateCommand 创建 generate 子命令
func generateCommand() *ucli.Command {
	return &ucli.Command{
		Name:  "generate",
		Usage: "生成配置文件",
		Commands: []*ucli.Command{
			{
				Name:  "env",
				Usage: "生成示例 .env 文件",
				Action: func(ctx context.Context, cmd *ucli.Command) error {
					if err := common.GenerateExampleEnv(".env.example"); err != nil {
						return err
					}
					fmt.Println("成功生成示例.env文件: .env.example")
					return nil
				},
			},
			{
				Name:  "config",
				Usage: "生成示例配置文件",
				Action: func(ctx context.Context, cmd *ucli.Command) error {
					if err := config.GenerateExample(); err != nil {
						return err
					}
					fmt.Println("成功生成示例配置文件: config/config.toml")
					return nil
				},
			},
		},
	}
}
