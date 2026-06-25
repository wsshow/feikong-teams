package skill

import (
	"context"
	"fmt"

	"fkteams/internal/app/appdata"
	appskill "fkteams/internal/app/skill"

	"github.com/pterm/pterm"
	ucli "github.com/urfave/cli/v3"
)

// Command 创建 skill 子命令
func Command(initConfig func() error) *ucli.Command {
	return &ucli.Command{
		Name:  "skill",
		Usage: "技能管理",
		Commands: []*ucli.Command{
			{
				Name:    "list",
				Aliases: []string{"ls"},
				Usage:   "列出本地已安装的技能",
				Action: func(ctx context.Context, cmd *ucli.Command) error {
					if err := initConfig(); err != nil {
						return err
					}
					return listSkills()
				},
			},
			searchCommand(),
			installCommand(),
			removeCommand(),
		},
	}
}

func listSkills() error {
	skillsDir := appdata.SkillsDir()
	skills, err := appskill.ListLocalSkills()
	if err != nil {
		return fmt.Errorf("读取技能目录失败: %w", err)
	}

	if len(skills) == 0 {
		pterm.Info.Println("暂无可用的技能")
		pterm.FgGray.Printfln("技能目录: %s", skillsDir)
		return nil
	}

	pterm.DefaultSection.Println("可用的技能列表")
	for _, s := range skills {
		desc := s.Description
		if desc == "" {
			desc = "(无描述)"
		}
		pterm.Bold.Printf("  %s", s.Slug)
		if s.Name != "" && s.Name != s.Slug {
			pterm.FgGray.Printf("  %s", s.Name)
		}
		fmt.Println()
		pterm.FgGray.Printfln("    %s", desc)
	}

	return nil
}
