package commands

import (
	"context"
	"fkteams/internal/app/config"
	"fmt"

	"fkteams/internal/app/tools"

	"github.com/pterm/pterm"
	ucli "github.com/urfave/cli/v3"
)

// toolCommand 创建 tool 子命令
func toolCommand() *ucli.Command {
	return &ucli.Command{
		Name:  "tool",
		Usage: "工具管理",
		Commands: []*ucli.Command{
			{
				Name:  "list",
				Usage: "列出所有可用的工具",
				Action: func(ctx context.Context, cmd *ucli.Command) error {
					if err := config.Init(); err != nil {
						return err
					}
					return listTools(ctx)
				},
			},
		},
	}
}

func listTools(ctx context.Context) error {
	// 内置工具
	pterm.DefaultSection.Println("内置工具")

	for _, groupName := range tools.BuiltinToolNames(ctx) {
		ts, err := tools.GetToolsByName(ctx, groupName)
		if err != nil {
			pterm.FgGray.Printfln("  [%s] (不可用: %v)", groupName, err)
			continue
		}
		fmt.Println()
		pterm.Bold.Printfln("  [%s]", groupName)
		for _, t := range ts {
			info, _ := t.Info(context.Background())
			pterm.FgGray.Printfln("    %-30s %s", info.Name, info.Desc)
		}
	}

	// MCP 工具
	mcpTools, err := tools.GetAllMCPToolGroups(ctx)
	if err == nil && len(mcpTools) > 0 {
		fmt.Println()
		pterm.DefaultSection.Println("MCP 工具")
		for name, group := range mcpTools {
			fmt.Println()
			desc := group.Desc
			if desc == "" {
				desc = name
			}
			pterm.Bold.Printfln("  [mcp-%s] %s", name, desc)
			for _, t := range group.Tools {
				info, _ := t.Info(context.Background())
				pterm.FgGray.Printfln("    %-30s %s", info.Name, info.Desc)
			}
		}
	}

	return nil
}
