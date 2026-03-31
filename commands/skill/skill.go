package skill

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	commonPkg "fkteams/common"

	"github.com/goccy/go-yaml"
	"github.com/pterm/pterm"
	ucli "github.com/urfave/cli/v3"
)

// Command 创建 skill 子命令
func Command(loadEnv func() error) *ucli.Command {
	return &ucli.Command{
		Name:  "skill",
		Usage: "技能管理",
		Commands: []*ucli.Command{
			{
				Name:    "list",
				Aliases: []string{"ls"},
				Usage:   "列出本地已安装的技能",
				Action: func(ctx context.Context, cmd *ucli.Command) error {
					if err := loadEnv(); err != nil {
						return nil
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

type localSkillInfo struct {
	Dir         string `yaml:"-"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func listSkills() error {
	skillsDir := filepath.Join(commonPkg.WorkspaceDir(), "skills")

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			pterm.Info.Println("暂无可用的技能")
			pterm.FgGray.Printfln("技能目录: %s", skillsDir)
			return nil
		}
		return fmt.Errorf("读取技能目录失败: %w", err)
	}

	var skills []localSkillInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillFile := filepath.Join(skillsDir, entry.Name(), "SKILL.md")
		info := parseSkillFile(skillFile)
		if info.Name == "" {
			info.Name = entry.Name()
		}
		info.Dir = entry.Name()
		skills = append(skills, info)
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
		pterm.Bold.Printf("  %s", s.Dir)
		if s.Name != "" && s.Name != s.Dir {
			pterm.FgGray.Printf("  %s", s.Name)
		}
		fmt.Println()
		pterm.FgGray.Printfln("    %s", desc)
	}

	return nil
}

// parseSkillFile 解析 SKILL.md 的 YAML frontmatter 提取 name 和 description
func parseSkillFile(path string) localSkillInfo {
	data, err := os.ReadFile(path)
	if err != nil {
		return localSkillInfo{}
	}

	content := string(data)
	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return localSkillInfo{}
	}

	var info localSkillInfo
	if err := yaml.Unmarshal([]byte(parts[1]), &info); err != nil {
		return localSkillInfo{}
	}
	info.Description = strings.Join(strings.Fields(info.Description), " ")
	return info
}
