package commands

import (
	"context"
	"fmt"

	"fkteams/providers/copilot"

	ucli "github.com/urfave/cli/v3"
)

// loginCommand 创建 login 子命令（统一登录入口）
func loginCommand() *ucli.Command {
	return &ucli.Command{
		Name:  "login",
		Usage: "登录模型服务提供者",
		Commands: []*ucli.Command{
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
		},
	}
}

// logoutCommand 创建 logout 子命令
func logoutCommand() *ucli.Command {
	return &ucli.Command{
		Name:  "logout",
		Usage: "退出模型服务登录",
		Commands: []*ucli.Command{
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
		},
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
