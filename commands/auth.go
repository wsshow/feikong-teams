package commands

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"fkteams/config"
	"fkteams/tui"

	ucli "github.com/urfave/cli/v3"
)

// authCommand 创建 auth 子命令（管理 web 认证）
func authCommand() *ucli.Command {
	return &ucli.Command{
		Name:  "auth",
		Usage: "管理 Web 服务认证",
		Commands: []*ucli.Command{
			{
				Name:  "enable",
				Usage: "启用 Web 认证",
				Flags: []ucli.Flag{
					&ucli.StringFlag{Name: "username", Aliases: []string{"u"}, Usage: "用户名"},
					&ucli.StringFlag{Name: "password", Aliases: []string{"p"}, Usage: "密码"},
				},
				Action: authEnableAction,
			},
			{
				Name:   "disable",
				Usage:  "禁用 Web 认证",
				Action: authDisableAction,
			},
			{
				Name:   "status",
				Usage:  "查看认证状态",
				Action: authStatusAction,
			},
		},
		Action: interactiveAuthAction,
	}
}

// interactiveAuthAction 无子命令时的交互式认证配置
func interactiveAuthAction(ctx context.Context, cmd *ucli.Command) error {
	if err := config.Init(); err != nil {
		return err
	}

	cfg := config.Get()
	currentStatus := "禁用"
	if cfg.Server.Auth.Enabled {
		currentStatus = fmt.Sprintf("启用（用户: %s）", cfg.Server.Auth.Username)
	}

	items := []tui.SelectItem{
		{Label: "启用认证", Value: "enable"},
		{Label: "禁用认证", Value: "disable"},
	}

	fmt.Printf("当前认证状态: %s\n", currentStatus)
	selected, err := tui.SelectFromList("请选择操作", items, 5)
	if err != nil {
		return err
	}

	switch selected {
	case "enable":
		return configureAuth(cfg)
	case "disable":
		return disableAuth(cfg)
	}
	return nil
}

// authEnableAction 启用认证
func authEnableAction(ctx context.Context, cmd *ucli.Command) error {
	if err := config.Init(); err != nil {
		return err
	}

	cfg := config.Get()
	username := cmd.String("username")
	password := cmd.String("password")

	if username == "" || password == "" {
		return configureAuth(cfg)
	}

	return enableAuth(cfg, username, password)
}

// authDisableAction 禁用认证
func authDisableAction(ctx context.Context, cmd *ucli.Command) error {
	if err := config.Init(); err != nil {
		return err
	}
	return disableAuth(config.Get())
}

// authStatusAction 查看认证状态
func authStatusAction(ctx context.Context, cmd *ucli.Command) error {
	if err := config.Init(); err != nil {
		return err
	}

	auth := config.Get().Server.Auth
	if !auth.Enabled {
		fmt.Println("认证状态: 禁用")
		return nil
	}
	fmt.Printf("认证状态: 启用\n")
	fmt.Printf("  用户名: %s\n", auth.Username)
	fmt.Printf("  密码: %s\n", maskString(auth.Password))
	return nil
}

// configureAuth 交互式配置认证
func configureAuth(cfg *config.Config) error {
	defaultUser := cfg.Server.Auth.Username
	if defaultUser == "" {
		defaultUser = "admin"
	}

	username, err := tui.ReadInput("用户名", defaultUser)
	if err != nil {
		return err
	}
	if username == "" {
		return fmt.Errorf("用户名不能为空")
	}

	password, err := tui.ReadSecret("密码")
	if err != nil {
		return err
	}
	if password == "" {
		return fmt.Errorf("密码不能为空")
	}

	return enableAuth(cfg, username, password)
}

// enableAuth 启用认证并保存配置
func enableAuth(cfg *config.Config, username, password string) error {
	cfg.Server.Auth.Enabled = true
	cfg.Server.Auth.Username = username
	cfg.Server.Auth.Password = password

	// 如果 secret 为空，自动生成
	if cfg.Server.Auth.Secret == "" {
		secret, err := generateSecret()
		if err != nil {
			return fmt.Errorf("生成 secret 失败: %w", err)
		}
		cfg.Server.Auth.Secret = secret
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}

	fmt.Printf("✓ 已启用 Web 认证（用户: %s）\n", username)
	fmt.Println("  重启 web/serve 服务后生效")
	return nil
}

// disableAuth 禁用认证并保存配置
func disableAuth(cfg *config.Config) error {
	cfg.Server.Auth.Enabled = false
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}
	fmt.Println("✓ 已禁用 Web 认证")
	fmt.Println("  重启 web/serve 服务后生效")
	return nil
}

// generateSecret 生成随机 secret
func generateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// maskString 掩码字符串
func maskString(s string) string {
	if len(s) <= 2 {
		return "***"
	}
	return s[:1] + "***" + s[len(s)-1:]
}
