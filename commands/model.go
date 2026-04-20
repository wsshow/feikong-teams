package commands

import (
	"context"
	"fmt"

	"fkteams/config"
	"fkteams/providers"
	"fkteams/tui"

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
			{
				Name:    "sw",
				Aliases: []string{"switch"},
				Usage:   "切换默认模型",
				Flags: []ucli.Flag{
					&ucli.StringFlag{
						Name:    "name",
						Aliases: []string{"n"},
						Usage:   "模型配置名称（未指定则交互式选择）",
					},
					&ucli.StringFlag{
						Name:    "model",
						Aliases: []string{"m"},
						Usage:   "切换到该供应商下的指定模型（可选）",
					},
				},
				Action: func(ctx context.Context, cmd *ucli.Command) error {
					if err := config.Init(); err != nil {
						return err
					}
					return switchModel(ctx, cmd.String("name"), cmd.String("model"))
				},
			},
			{
				Name:    "rm",
				Aliases: []string{"remove"},
				Usage:   "移除指定的模型配置",
				Flags: []ucli.Flag{
					&ucli.StringFlag{
						Name:    "name",
						Aliases: []string{"n"},
						Usage:   "要移除的模型配置名称（未指定则交互式选择）",
					},
				},
				Action: func(ctx context.Context, cmd *ucli.Command) error {
					if err := config.Init(); err != nil {
						return err
					}
					name := cmd.String("name")
					if name == "" {
						var err error
						name, err = selectModelToRemove()
						if err != nil {
							return err
						}
					}
					return removeModel(name)
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

// removeModel 移除指定名称的模型配置
func removeModel(name string) error {
	cfg := config.Get()
	var newModels []config.ModelConfig
	var removed bool
	for _, m := range cfg.Models {
		if m.Name == name {
			removed = true
			continue
		}
		newModels = append(newModels, m)
	}

	if !removed {
		return fmt.Errorf("未找到模型配置「%s」", name)
	}

	cfg.Models = newModels
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}

	fmt.Printf("✓ 已移除模型配置「%s」\n", name)
	return nil
}

// selectModelToRemove 交互式选择要移除的模型配置
func selectModelToRemove() (string, error) {
	cfg := config.Get()
	if len(cfg.Models) == 0 {
		return "", fmt.Errorf("暂无配置的模型")
	}

	items := make([]tui.SelectItem, 0, len(cfg.Models))
	for _, m := range cfg.Models {
		label := m.Name
		if m.Provider != "" {
			label += " (" + m.Provider
			if m.Model != "" {
				label += "/" + m.Model
			}
			label += ")"
		}
		items = append(items, tui.SelectItem{Label: label, Value: m.Name})
	}

	return tui.SelectFromList("请选择要移除的模型配置", items)
}

// switchModel 切换默认模型
func switchModel(ctx context.Context, name, model string) error {
	cfg := config.Get()

	// 快捷路径：直接更新 default 模型
	if name == "" || name == "default" {
		defaultModel := cfg.ResolveModel("default")
		if defaultModel == nil {
			return fmt.Errorf("尚未配置默认模型，请先使用 fkteams login 登录或 fkteams model sw 选择供应商")
		}
		if model != "" {
			oldModel := defaultModel.Model
			defaultModel.Model = model
			if err := config.Save(cfg); err != nil {
				return fmt.Errorf("保存配置失败: %w", err)
			}
			fmt.Printf("✓ 已切换模型：%s → %s（%s）\n", oldModel, model, defaultModel.Provider)
			return nil
		}
		if name == "default" {
			return switchCurrentModel(ctx, cfg)
		}
	}

	// 筛选可切换的模型（排除 default）
	var candidates []config.ModelConfig
	for _, m := range cfg.Models {
		if m.Name != "default" {
			candidates = append(candidates, m)
		}
	}

	// 交互式选择
	if name == "" {
		var items []tui.SelectItem

		defaultModel := cfg.ResolveModel("default")
		if defaultModel != nil && defaultModel.Provider != "" {
			current := defaultModel.Provider
			if defaultModel.Model != "" {
				current += "/" + defaultModel.Model
			}
			items = append(items, tui.SelectItem{
				Label: fmt.Sprintf("切换当前供应商模型（%s）", current),
				Value: "__switch_model__",
			})
		}

		if len(candidates) == 0 && len(items) == 0 {
			return fmt.Errorf("没有可切换的模型配置，请先使用 fkteams login 登录")
		}

		for _, m := range candidates {
			label := m.Name + " (" + m.Provider
			if m.Model != "" {
				label += "/" + m.Model
			}
			label += ")"
			items = append(items, tui.SelectItem{Label: label, Value: m.Name})
		}

		selected, err := tui.SelectFromList("请选择操作", items)
		if err != nil {
			return err
		}
		if selected == "__switch_model__" {
			return switchCurrentModel(ctx, cfg)
		}
		name = selected
	}

	// 按 name 查找，回退按 provider 查找（含 default）
	source := findModelConfig(cfg, candidates, name)
	if source == nil {
		return fmt.Errorf("未找到模型配置「%s」", name)
	}

	targetModel := source.Model
	if model != "" {
		targetModel = model
	} else if targetModel == "" {
		selected, err := promptModelSelection(ctx, source.Provider, source.APIKey, source.BaseURL)
		if err != nil {
			return err
		}
		if selected != "" {
			targetModel = selected
		}
	}

	// 更新或创建 default
	defaultModel := cfg.ResolveModel("default")
	if defaultModel != nil {
		defaultModel.Provider = source.Provider
		defaultModel.APIKey = source.APIKey
		defaultModel.BaseURL = source.BaseURL
		defaultModel.Model = targetModel
	} else {
		cfg.Models = append(cfg.Models, config.ModelConfig{
			Name:     "default",
			Provider: source.Provider,
			APIKey:   source.APIKey,
			BaseURL:  source.BaseURL,
			Model:    targetModel,
		})
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}

	displayModel := targetModel
	if displayModel == "" {
		displayModel = "(未指定)"
	}
	fmt.Printf("✓ 已切换默认模型为「%s」（%s / %s）\n", name, source.Provider, displayModel)
	return nil
}

// findModelConfig 按 name 查找候选模型，回退按 provider 在所有模型中查找
func findModelConfig(cfg *config.Config, candidates []config.ModelConfig, name string) *config.ModelConfig {
	for i := range candidates {
		if candidates[i].Name == name {
			return &candidates[i]
		}
	}
	for i := range cfg.Models {
		if cfg.Models[i].Provider == name {
			return &cfg.Models[i]
		}
	}
	return nil
}

// switchCurrentModel 在当前默认供应商内切换模型
func switchCurrentModel(ctx context.Context, cfg *config.Config) error {
	defaultModel := cfg.ResolveModel("default")

	selected, err := promptModelSelection(ctx, defaultModel.Provider, defaultModel.APIKey, defaultModel.BaseURL)
	if err != nil {
		return err
	}
	if selected == "" {
		return fmt.Errorf("未选择模型")
	}

	oldModel := defaultModel.Model
	defaultModel.Model = selected
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}
	fmt.Printf("✓ 已切换模型：%s → %s（%s）\n", oldModel, selected, defaultModel.Provider)
	return nil
}
