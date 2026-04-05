package commands

import (
	"context"
	"fmt"

	"fkteams/config"
	"fkteams/providers"

	"github.com/pterm/pterm"
	ucli "github.com/urfave/cli/v3"
)

// modelCommand 创建 model 子命令
func modelCommand() *ucli.Command {
	return &ucli.Command{
		Name:  "model",
		Usage: "模型配置管理",
		Commands: []*ucli.Command{
			{
				Name:    "ls",
				Aliases: []string{"list"},
				Usage:   "列出已配置的模型",
				Action: func(ctx context.Context, cmd *ucli.Command) error {
					if err := config.Init(); err != nil {
						return err
					}
					return listModels()
				},
			},
			{
				Name:    "lr",
				Aliases: []string{"remote"},
				Usage:   "查询服务商的可用模型",
				Flags: []ucli.Flag{
					&ucli.StringFlag{
						Name:    "provider",
						Aliases: []string{"p"},
						Usage:   "服务商 (openai, deepseek, copilot, ...)",
					},
					&ucli.StringFlag{
						Name:  "name",
						Usage: "从已配置的模型读取服务商信息",
						Value: "default",
					},
				},
				Action: func(ctx context.Context, cmd *ucli.Command) error {
					if err := config.Init(); err != nil {
						return err
					}
					return listAvailableModels(ctx, cmd.String("provider"), cmd.String("name"))
				},
			},
		},
	}
}

// listModels 列出所有已配置的模型
func listModels() error {
	cfg := config.Get()
	if len(cfg.Models) == 0 {
		pterm.Warning.Println("暂无配置的模型")
		return nil
	}

	data := make([][]string, 0, len(cfg.Models))
	for _, m := range cfg.Models {
		isDefault := ""
		if m.Name == "default" {
			isDefault = "✓"
		}
		provider := m.Provider
		if provider == "" {
			provider = "-"
		}
		baseURL := m.BaseURL
		if baseURL == "" {
			baseURL = "(默认)"
		}
		data = append(data, []string{m.Name, provider, m.Model, baseURL, isDefault})
	}

	pterm.DefaultTable.WithHasHeader().WithData(append(
		[][]string{{"名称", "服务商", "模型", "接口地址", "默认"}},
		data...,
	)).Render()
	return nil
}

// listAvailableModels 查询服务商可用的模型列表
func listAvailableModels(ctx context.Context, provider, name string) error {
	cfg := config.Get()
	var pcfg providers.Config

	if provider != "" {
		// 通过 --provider 指定
		pcfg.Provider = providers.Type(provider)
		// 尝试从配置中找对应服务商的密钥
		for _, m := range cfg.Models {
			if m.Provider == provider {
				pcfg.APIKey = m.APIKey
				pcfg.BaseURL = m.BaseURL
				break
			}
		}
	} else {
		// 通过 --name 从已配置的模型读取
		mc := cfg.ResolveModel(name)
		if mc == nil {
			return fmt.Errorf("未找到模型配置: %s", name)
		}
		pcfg.Provider = providers.Type(mc.Provider)
		pcfg.APIKey = mc.APIKey
		pcfg.BaseURL = mc.BaseURL
	}

	if pcfg.Provider == "" {
		return fmt.Errorf("请指定服务商（--provider）或模型配置名称（--name）")
	}

	spinner, _ := pterm.DefaultSpinner.Start(fmt.Sprintf("正在查询 %s 可用模型...", pcfg.Provider))
	models, err := providers.ListModels(ctx, &pcfg)
	if err != nil {
		spinner.Fail(err.Error())
		return err
	}
	spinner.Success(fmt.Sprintf("找到 %d 个可用模型", len(models)))

	for _, m := range models {
		fmt.Println("  " + m.ID)
	}
	return nil
}
