package skill

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fkteams/internal/app/appdata"
	appskill "fkteams/internal/app/skill"

	"github.com/pterm/pterm"
	ucli "github.com/urfave/cli/v3"
)

func installCommand() *ucli.Command {
	return &ucli.Command{
		Name:      "install",
		Usage:     "从技能市场安装技能",
		ArgsUsage: "<技能slug>",
		Flags: []ucli.Flag{
			&ucli.StringFlag{
				Name:  "version",
				Usage: "技能版本（留空为最新版本）",
			},
			&ucli.StringFlag{
				Name:  "provider",
				Usage: "指定后端，可选: " + strings.Join(appskill.ProviderNames(), ", "),
			},
		},
		Action: func(ctx context.Context, cmd *ucli.Command) error {
			slug := cmd.Args().First()
			if slug == "" {
				return fmt.Errorf("请提供技能 slug，例如: fkteams skill install video-frames")
			}
			version := cmd.String("version")

			var provider appskill.Provider
			if name := cmd.String("provider"); name != "" {
				provider = appskill.GetProviderByName(name)
				if provider == nil {
					return fmt.Errorf("未找到后端: %s", name)
				}
			} else {
				provider = appskill.GetDefaultProvider()
			}
			if provider == nil {
				return fmt.Errorf("无可用的技能后端")
			}

			return installSkill(ctx, slug, version, provider)
		},
	}
}

func installSkill(ctx context.Context, slug, version string, provider appskill.Provider) error {
	skillsDir := appdata.SkillsDir()
	targetDir := filepath.Join(skillsDir, slug)
	if _, err := os.Stat(targetDir); err == nil {
		pterm.Warning.Printfln("技能 %s 已存在，将覆盖安装", slug)
	}
	versionLabel := version
	if versionLabel == "" {
		versionLabel = "latest"
	}
	spinner, _ := pterm.DefaultSpinner.Start(fmt.Sprintf("正在从 %s 下载 %s@%s...", provider.Name(), slug, versionLabel))
	if err := appskill.InstallSkillFromProvider(ctx, slug, version, provider); err != nil {
		spinner.Fail(fmt.Sprintf("下载失败: %v", err))
		return err
	}
	spinner.Success(fmt.Sprintf("技能 %s@%s 安装成功", slug, versionLabel))
	pterm.FgGray.Printfln("安装路径: %s", targetDir)
	return nil
}
