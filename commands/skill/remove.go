package skill

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	commonPkg "fkteams/common"

	"github.com/pterm/pterm"
	ucli "github.com/urfave/cli/v3"
)

func removeCommand() *ucli.Command {
	return &ucli.Command{
		Name:      "remove",
		Aliases:   []string{"rm"},
		Usage:     "移除已安装的技能",
		ArgsUsage: "<技能slug>",
		Action: func(ctx context.Context, cmd *ucli.Command) error {
			slug := cmd.Args().First()
			if slug == "" {
				return fmt.Errorf("请提供技能 slug，例如: fkteams skill remove video-frames")
			}
			targetDir := filepath.Join(commonPkg.AppDir(), "skills", slug)
			if _, err := os.Stat(targetDir); os.IsNotExist(err) {
				return fmt.Errorf("技能 %s 未安装", slug)
			}
			if err := os.RemoveAll(targetDir); err != nil {
				return fmt.Errorf("移除技能失败: %w", err)
			}
			pterm.Success.Printfln("技能 %s 已移除", slug)
			return nil
		},
	}
}
