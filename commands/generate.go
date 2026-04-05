package commands

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fkteams/common"
	"fkteams/config"
	"fmt"

	ucli "github.com/urfave/cli/v3"
)

// generateCommand 创建 generate 子命令
func generateCommand() *ucli.Command {
	return &ucli.Command{
		Name:  "generate",
		Usage: "生成配置文件或密钥",
		Commands: []*ucli.Command{
			{
				Name:  "config",
				Usage: "生成示例配置文件",
				Action: func(ctx context.Context, cmd *ucli.Command) error {
					if err := config.GenerateExample(); err != nil {
						return err
					}
					fmt.Printf("成功生成示例配置文件: %s/config/config.toml\n", common.AppDir())
					return nil
				},
			},
			{
				Name:  "apikey",
				Usage: "生成 OpenAI 兼容 API 密钥",
				Action: func(ctx context.Context, cmd *ucli.Command) error {
					b := make([]byte, 24)
					if _, err := rand.Read(b); err != nil {
						return fmt.Errorf("failed to generate random bytes: %w", err)
					}
					key := "sk-fkteams-" + hex.EncodeToString(b)
					fmt.Println(key)
					fmt.Println("\n请将此密钥添加到 config.toml 的 [openai_api] 配置中:")
					fmt.Println("  api_keys = [\"" + key + "\"]")
					return nil
				},
			},
		},
	}
}
