package skill

import (
	"context"
	"fmt"
	"strings"

	"github.com/pterm/pterm"
	ucli "github.com/urfave/cli/v3"
)

func searchCommand() *ucli.Command {
	return &ucli.Command{
		Name:      "search",
		Usage:     "搜索技能市场",
		ArgsUsage: "<关键词>",
		Flags: []ucli.Flag{
			&ucli.IntFlag{
				Name:  "page",
				Value: 1,
				Usage: "页码",
			},
			&ucli.IntFlag{
				Name:  "size",
				Value: 10,
				Usage: "每页数量",
			},
			&ucli.StringSliceFlag{
				Name:  "provider",
				Usage: "指定后端（可多次指定），可选: " + strings.Join(ProviderNames(), ", "),
			},
		},
		Action: func(ctx context.Context, cmd *ucli.Command) error {
			keyword := cmd.Args().First()
			if keyword == "" {
				return fmt.Errorf("请提供搜索关键词，例如: fkteams skill search ffmpeg")
			}
			page := int(cmd.Int("page"))
			size := int(cmd.Int("size"))
			providers, err := GetProvidersByNames(cmd.StringSlice("provider"))
			if err != nil {
				return err
			}
			return searchSkills(ctx, keyword, page, size, providers)
		},
	}
}

func searchSkills(ctx context.Context, keyword string, page, size int, providers []Provider) error {
	for _, p := range providers {
		resp, err := p.Search(ctx, keyword, page, size, "", "")
		if err != nil {
			pterm.Warning.Printfln("[%s] 搜索失败: %v", p.Name(), err)
			continue
		}

		if resp.Total == 0 {
			pterm.Info.Printfln("[%s] 未找到与 \"%s\" 相关的技能", p.Name(), keyword)
			continue
		}

		pterm.DefaultSection.Printfln("搜索结果 (%d/%d)  来源: %s", len(resp.Skills), resp.Total, p.Name())
		fmt.Println()

		for i, s := range resp.Skills {
			idx := (page-1)*size + i + 1

			// 第一行：序号 + 名称 + slug + 版本
			pterm.FgWhite.Printf("  %2d. ", idx)
			pterm.Bold.Print(s.Name)
			if s.Slug != "" && s.Slug != s.Name {
				pterm.FgCyan.Printf(" (%s)", s.Slug)
			}
			if s.Version != "" {
				pterm.FgGray.Printf(" v%s", s.Version)
			}
			fmt.Println()

			// 第二行：描述（优先中文，截断 100 字符）
			desc := s.DescZh
			if desc == "" {
				desc = s.Description
			}
			if desc != "" {
				desc = strings.Join(strings.Fields(desc), " ")
				if runes := []rune(desc); len(runes) > 100 {
					desc = string(runes[:100]) + "..."
				}
				pterm.FgGray.Printfln("      %s", desc)
			}

			// 第三行：元信息
			var meta []string
			if s.Owner != "" {
				meta = append(meta, "@"+s.Owner)
			}
			meta = append(meta, "下载 "+formatCount(s.Downloads))
			if s.Stars > 0 {
				meta = append(meta, "★ "+formatCount(s.Stars))
			}
			pterm.FgDarkGray.Printfln("      %s", strings.Join(meta, "  "))

			fmt.Println()
		}

		if resp.Total > page*size {
			pterm.FgGray.Printfln("  使用 --page %d 查看更多结果", page+1)
		}
	}
	return nil
}

func formatCount(n int) string {
	switch {
	case n >= 1000000:
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	case n >= 1000:
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	default:
		return fmt.Sprintf("%d", n)
	}
}
