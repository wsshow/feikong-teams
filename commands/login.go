package commands

import (
	"context"
	"fmt"

	"fkteams/config"
	"fkteams/providers"
	"fkteams/providers/copilot"
	"fkteams/tui"

	ucli "github.com/urfave/cli/v3"
)

// providerEntry 供应商信息
type providerEntry struct {
	name       string // 子命令名
	usage      string // 帮助说明
	defaultURL string // 默认 BaseURL（空表示无需或由 SDK 决定）
	needKey    bool   // 是否需要 API Key
}

// knownProviders 已知供应商列表（不含 copilot 和 custom）
var knownProviders = []providerEntry{
	{"openai", "登录 OpenAI", "https://api.openai.com/v1", true},
	{"deepseek", "登录 DeepSeek", "https://api.deepseek.com", true},
	{"claude", "登录 Anthropic Claude", "https://api.anthropic.com", true},
	{"gemini", "登录 Google Gemini", "", true},
	{"qwen", "登录阿里通义千问", "https://dashscope.aliyuncs.com/compatible-mode/v1", true},
	{"ollama", "登录 Ollama 本地模型", "http://localhost:11434/v1", false},
	{"ark", "登录火山引擎方舟", "https://ark.cn-beijing.volces.com/api/v3", true},
	{"openrouter", "登录 OpenRouter", "https://openrouter.ai/api/v1", true},
}

// apiKeyFlags 通用 API Key 相关的 flags
func apiKeyFlags(defaultURL string, needKey bool) []ucli.Flag {
	flags := []ucli.Flag{
		&ucli.StringFlag{
			Name:  "base-url",
			Usage: fmt.Sprintf("自定义 API 地址（默认: %s）", defaultOrNone(defaultURL)),
		},
		&ucli.StringFlag{
			Name:  "model",
			Usage: "默认模型名称",
		},
		&ucli.StringFlag{
			Name:  "name",
			Usage: "配置名称（默认与供应商同名，设为 default 会覆盖默认模型）",
		},
	}
	if needKey {
		flags = append([]ucli.Flag{
			&ucli.StringFlag{
				Name:  "api-key",
				Usage: "API 密钥（未提供则交互式输入）",
			},
		}, flags...)
	}
	return flags
}

func defaultOrNone(s string) string {
	if s == "" {
		return "无"
	}
	return s
}

// loginCommand 创建 login 子命令（统一登录入口）
func loginCommand() *ucli.Command {
	commands := []*ucli.Command{
		{
			Name:  "copilot",
			Usage: "登录 GitHub Copilot（OAuth 设备码流程）",
			Flags: []ucli.Flag{
				&ucli.BoolFlag{
					Name:  "import",
					Usage: "从 VS Code 已保存的 Copilot token 导入（免登录）",
				},
			},
			Action: copilotLoginAction,
		},
	}

	// 注册所有已知供应商
	for _, p := range knownProviders {
		p := p // capture
		commands = append(commands, &ucli.Command{
			Name:  p.name,
			Usage: p.usage,
			Flags: apiKeyFlags(p.defaultURL, p.needKey),
			Action: func(ctx context.Context, cmd *ucli.Command) error {
				return providerLoginAction(cmd, p.name, p.defaultURL, p.needKey)
			},
		})
	}

	// 自定义供应商
	commands = append(commands, &ucli.Command{
		Name:  "custom",
		Usage: "登录自定义 OpenAI 兼容供应商",
		Flags: []ucli.Flag{
			&ucli.StringFlag{
				Name:  "api-key",
				Usage: "API 密钥（未提供则交互式输入）",
			},
			&ucli.StringFlag{
				Name:     "base-url",
				Usage:    "API 地址（必填）",
				Required: true,
			},
			&ucli.StringFlag{
				Name:  "model",
				Usage: "默认模型名称",
			},
			&ucli.StringFlag{
				Name:  "name",
				Usage: "配置名称（必填，用于引用此供应商）",
				Value: "custom",
			},
			&ucli.StringFlag{
				Name:  "provider",
				Usage: "供应商类型（默认 openai，用于模型检测）",
				Value: "openai",
			},
		},
		Action: customLoginAction,
	})

	return &ucli.Command{
		Name:     "login",
		Usage:    "登录模型服务提供者",
		Commands: commands,
		Action:   interactiveLoginAction,
	}
}

// logoutCommand 创建 logout 子命令
func logoutCommand() *ucli.Command {
	commands := []*ucli.Command{
		{
			Name:  "copilot",
			Usage: "退出 GitHub Copilot 登录",
			Action: func(ctx context.Context, cmd *ucli.Command) error {
				if err := copilot.GetTokenManager().SetToken(&copilot.Token{}); err != nil {
					return err
				}
				fmt.Println("✓ 已退出 GitHub Copilot")
				return nil
			},
		},
	}

	// 为所有已知供应商注册 logout
	for _, p := range knownProviders {
		p := p
		commands = append(commands, &ucli.Command{
			Name:  p.name,
			Usage: fmt.Sprintf("移除 %s 登录配置", p.name),
			Flags: []ucli.Flag{
				&ucli.StringFlag{
					Name:  "name",
					Usage: "配置名称（默认与供应商同名）",
				},
			},
			Action: func(ctx context.Context, cmd *ucli.Command) error {
				return providerLogoutAction(cmd, p.name)
			},
		})
	}

	// 自定义供应商 logout
	commands = append(commands, &ucli.Command{
		Name:  "custom",
		Usage: "移除自定义供应商登录配置",
		Flags: []ucli.Flag{
			&ucli.StringFlag{
				Name:     "name",
				Usage:    "配置名称",
				Required: true,
			},
		},
		Action: func(ctx context.Context, cmd *ucli.Command) error {
			return providerLogoutAction(cmd, "custom")
		},
	})

	return &ucli.Command{
		Name:     "logout",
		Usage:    "退出模型服务登录",
		Commands: commands,
		Action:   interactiveLogoutAction,
	}
}

func copilotLoginAction(ctx context.Context, cmd *ucli.Command) error {
	tm := copilot.GetTokenManager()

	// 尝试从 VS Code 导入
	if cmd.Bool("import") {
		githubToken, ok := copilot.ImportFromVSCode()
		if !ok {
			return fmt.Errorf("未找到 VS Code 已保存的 Copilot token")
		}

		fmt.Println("从 VS Code 导入 GitHub token...")
		dc, err := loginWithGitHubToken(ctx, tm, githubToken)
		if err != nil {
			return err
		}
		_ = dc
		return nil
	}

	// 设备码流程
	fmt.Println("正在请求设备授权码...")
	dc, err := copilot.RequestDeviceCode(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("\n请在浏览器中打开: %s\n", dc.VerificationURI)
	fmt.Printf("输入授权码: %s\n\n", dc.UserCode)
	fmt.Println("等待授权...")

	token, err := copilot.PollForToken(ctx, dc)
	if err != nil {
		return err
	}

	if err := tm.SetToken(token); err != nil {
		return fmt.Errorf("保存 token 失败: %w", err)
	}

	fmt.Println("✓ GitHub Copilot 登录成功!")
	return nil
}

func loginWithGitHubToken(ctx context.Context, tm *copilot.TokenManager, githubToken string) (*copilot.Token, error) {
	// 直接用 github token 创建一个临时 token 以触发 exchange
	tempToken := &copilot.Token{GitHubToken: githubToken, ExpiresAt: 0}
	if err := tm.SetToken(tempToken); err != nil {
		return nil, err
	}
	// GetToken 会检测到过期并自动刷新（触发 exchange）
	if _, err := tm.GetToken(ctx); err != nil {
		return nil, fmt.Errorf("Copilot token 交换失败: %w", err)
	}
	fmt.Println("✓ GitHub Copilot 登录成功（从 VS Code 导入）!")
	return tempToken, nil
}

// providerLoginAction 处理已知供应商的登录（保存 API Key 到配置文件）
func providerLoginAction(cmd *ucli.Command, provider, defaultURL string, needKey bool) error {
	if err := config.Init(); err != nil {
		return err
	}

	name := cmd.String("name")
	if name == "" {
		name = provider
	}

	apiKey := cmd.String("api-key")
	if needKey && apiKey == "" {
		var err error
		apiKey, err = promptAPIKey(provider)
		if err != nil {
			return err
		}
		if apiKey == "" {
			return fmt.Errorf("API Key 不能为空")
		}
	}

	baseURL := cmd.String("base-url")
	if baseURL == "" {
		baseURL = defaultURL
	}

	model := cmd.String("model")
	if model == "" {
		var err error
		model, err = promptModelSelection(context.Background(), provider, apiKey, baseURL)
		if err != nil {
			return err
		}
	}

	return saveProviderConfig(name, provider, apiKey, baseURL, model)
}

// customLoginAction 处理自定义供应商的登录
func customLoginAction(ctx context.Context, cmd *ucli.Command) error {
	if err := config.Init(); err != nil {
		return err
	}

	name := cmd.String("name")
	provider := cmd.String("provider")
	baseURL := cmd.String("base-url")
	model := cmd.String("model")

	apiKey := cmd.String("api-key")
	if apiKey == "" {
		var err error
		apiKey, err = promptAPIKey("custom")
		if err != nil {
			return err
		}
	}

	if model == "" {
		var err error
		model, err = promptModelSelection(ctx, provider, apiKey, baseURL)
		if err != nil {
			return err
		}
	}

	return saveProviderConfig(name, provider, apiKey, baseURL, model)
}

// saveProviderConfig 保存供应商配置到 config.toml
func saveProviderConfig(name, provider, apiKey, baseURL, model string) error {
	cfg := config.Get()

	// 查找是否已存在同名配置
	var found bool
	for i := range cfg.Models {
		if cfg.Models[i].Name == name {
			cfg.Models[i].Provider = provider
			cfg.Models[i].APIKey = apiKey
			if baseURL != "" {
				cfg.Models[i].BaseURL = baseURL
			}
			if model != "" {
				cfg.Models[i].Model = model
			}
			found = true
			break
		}
	}

	if !found {
		cfg.Models = append(cfg.Models, config.ModelConfig{
			Name:     name,
			Provider: provider,
			APIKey:   apiKey,
			BaseURL:  baseURL,
			Model:    model,
		})
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}

	action := "新增"
	if found {
		action = "更新"
	}
	fmt.Printf("✓ 已%s供应商配置「%s」（%s）\n", action, name, provider)
	if baseURL != "" {
		fmt.Printf("  API 地址: %s\n", baseURL)
	}
	return nil
}

// providerLogoutAction 移除供应商配置
func providerLogoutAction(cmd *ucli.Command, defaultName string) error {
	if err := config.Init(); err != nil {
		return err
	}

	name := cmd.String("name")
	if name == "" {
		name = defaultName
	}

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
		return fmt.Errorf("未找到配置「%s」", name)
	}

	cfg.Models = newModels
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}

	fmt.Printf("✓ 已移除供应商配置「%s」\n", name)
	return nil
}

// interactiveLogoutAction 无子命令时的交互式退出入口
func interactiveLogoutAction(ctx context.Context, cmd *ucli.Command) error {
	if err := config.Init(); err != nil {
		return err
	}

	cfg := config.Get()

	// 构建已配置的供应商列表
	var items []tui.SelectItem

	// 检查 copilot 是否已登录（仅检查本地 token，不触发网络请求）
	tm := copilot.GetTokenManager()
	hasCopilot := tm.HasToken()
	if hasCopilot {
		items = append(items, tui.SelectItem{Label: "copilot - GitHub Copilot", Value: "copilot"})
	}

	// 列出已配置的模型（跳过 copilot 供应商，避免重复）
	for _, m := range cfg.Models {
		if hasCopilot && m.Provider == "copilot" {
			continue
		}
		label := m.Name
		if m.Provider != "" {
			label += " (" + m.Provider + ")"
		}
		if m.BaseURL != "" {
			label += " - " + m.BaseURL
		}
		items = append(items, tui.SelectItem{Label: label, Value: m.Name})
	}

	if len(items) == 0 {
		fmt.Println("当前没有已登录的供应商")
		return nil
	}

	selected, err := tui.SelectFromList("请选择要退出的供应商", items, 12)
	if err != nil {
		return err
	}

	if selected == "copilot" {
		if err := tm.SetToken(&copilot.Token{}); err != nil {
			return err
		}
		fmt.Println("✓ 已退出 GitHub Copilot")
		return nil
	}

	// 移除模型配置
	var newModels []config.ModelConfig
	for _, m := range cfg.Models {
		if m.Name != selected {
			newModels = append(newModels, m)
		}
	}
	cfg.Models = newModels
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}
	fmt.Printf("✓ 已移除供应商配置「%s」\n", selected)
	return nil
}

// interactiveLoginAction 无子命令时的交互式登录入口
func interactiveLoginAction(ctx context.Context, cmd *ucli.Command) error {
	// 构建供应商选择列表
	items := []tui.SelectItem{
		{Label: "copilot - GitHub Copilot（OAuth 设备码）", Value: "copilot"},
	}
	for _, p := range knownProviders {
		label := p.name
		if p.defaultURL != "" {
			label += " - " + p.defaultURL
		}
		items = append(items, tui.SelectItem{Label: label, Value: p.name})
	}
	items = append(items, tui.SelectItem{Label: "custom - 自定义 OpenAI 兼容供应商", Value: "custom"})

	providerName, err := tui.SelectFromList("请选择供应商", items, 12)
	if err != nil {
		return err
	}

	switch providerName {
	case "copilot":
		return copilotLoginAction(ctx, cmd)
	case "custom":
		return interactiveCustomLogin(ctx)
	default:
		return interactiveProviderLogin(ctx, providerName)
	}
}

// interactiveProviderLogin 交互式完成已知供应商登录
func interactiveProviderLogin(ctx context.Context, providerName string) error {
	if err := config.Init(); err != nil {
		return err
	}

	var entry providerEntry
	for _, p := range knownProviders {
		if p.name == providerName {
			entry = p
			break
		}
	}

	var apiKey string
	if entry.needKey {
		var err error
		apiKey, err = promptAPIKey(entry.name)
		if err != nil {
			return err
		}
		if apiKey == "" {
			return fmt.Errorf("API Key 不能为空")
		}
	}

	baseURL := entry.defaultURL
	customURL, err := tui.ReadInput("API 地址（直接回车使用默认值）", baseURL)
	if err != nil {
		return err
	}
	if customURL != "" {
		baseURL = customURL
	}

	model, err := promptModelSelection(ctx, entry.name, apiKey, baseURL)
	if err != nil {
		return err
	}

	return saveProviderConfig(entry.name, entry.name, apiKey, baseURL, model)
}

// interactiveCustomLogin 交互式完成自定义供应商登录
func interactiveCustomLogin(ctx context.Context) error {
	if err := config.Init(); err != nil {
		return err
	}

	name, err := tui.ReadInput("配置名称", "custom")
	if err != nil {
		return err
	}

	baseURL, err := tui.ReadInput("API 地址（必填）", "")
	if err != nil {
		return err
	}
	if baseURL == "" {
		return fmt.Errorf("API 地址不能为空")
	}

	apiKey, err := promptAPIKey("custom")
	if err != nil {
		return err
	}

	providerType, err := tui.ReadInput("供应商类型", "openai")
	if err != nil {
		return err
	}

	model, err := promptModelSelection(ctx, providerType, apiKey, baseURL)
	if err != nil {
		return err
	}

	return saveProviderConfig(name, providerType, apiKey, baseURL, model)
}

// promptAPIKey 交互式输入 API Key（掩码显示）
func promptAPIKey(provider string) (string, error) {
	return tui.ReadSecret(fmt.Sprintf("%s API Key", provider))
}

// promptModelSelection 尝试从供应商获取模型列表供选择，失败则返回错误
func promptModelSelection(ctx context.Context, provider, apiKey, baseURL string) (string, error) {
	if apiKey == "" && provider != "ollama" {
		// 无 key 且非 ollama，跳过自动获取
		model, err := tui.ReadInput("模型名称（可选，直接回车跳过）", "")
		if err != nil {
			return "", err
		}
		return model, nil
	}

	fmt.Printf("正在获取 %s 可用模型列表...\n", provider)
	models, err := providers.ListModels(ctx, &providers.Config{
		Provider: providers.Type(provider),
		APIKey:   apiKey,
		BaseURL:  baseURL,
	})

	if err != nil {
		return "", fmt.Errorf("获取模型列表失败: %w", err)
	}
	if len(models) == 0 {
		fmt.Println("⚠ 未找到可用模型")
		model, err := tui.ReadInput("模型名称（可选，直接回车跳过）", "")
		if err != nil {
			return "", err
		}
		return model, nil
	}

	fmt.Printf("✓ 找到 %d 个可用模型\n", len(models))

	items := make([]tui.SelectItem, 0, len(models)+1)
	for _, m := range models {
		items = append(items, tui.SelectItem{Label: m.ID, Value: m.ID})
	}
	items = append(items, tui.SelectItem{Label: "(手动输入)", Value: "__manual__"})

	for {
		selected, err := tui.SelectFromList("请选择模型", items, 15)
		if err != nil {
			return "", err
		}

		if selected != "__manual__" {
			return selected, nil
		}

		// 手动输入，Esc 可回退到列表
		model, err := tui.ReadInput("模型名称（Esc 返回列表）", "")
		if err == tui.ErrInterrupted {
			continue
		}
		if err != nil {
			return "", err
		}
		return model, nil
	}
}

// AllProviderNames 返回所有可用的供应商名称（供自动补全等场景使用）
func AllProviderNames() []string {
	names := make([]string, 0, len(knownProviders)+2)
	names = append(names, "copilot")
	for _, p := range knownProviders {
		names = append(names, p.name)
	}
	names = append(names, "custom")
	return names
}

// init 确保 providers 包的类型名与此处保持一致
var _ = providers.OpenAI
