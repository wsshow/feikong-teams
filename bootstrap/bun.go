package bootstrap

import (
	"fkteams/fkenv"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pterm/pterm"
)

// bunInitializer 检测并安装/升级 bun（JavaScript 运行时和包管理器）
type bunInitializer struct{}

func (b *bunInitializer) Name() string { return "bun" }

func (b *bunInitializer) Run() error {
	bunPath, err := lookPath("bun")
	if err != nil {
		pterm.Warning.Println("未检测到 bun，正在安装...")
		if err := b.install(); err != nil {
			return err
		}
	} else {
		// 获取当前版本并升级
		out, _ := combinedOutput(bunPath, "--version")
		currentVersion := strings.TrimSpace(string(out))
		pterm.Info.Printfln("当前版本: %s，正在检查更新...", currentVersion)

		if err := b.upgrade(); err != nil {
			return fmt.Errorf("upgrade failed: %w", err)
		}

		out, _ = combinedOutput(bunPath, "--version")
		newVersion := strings.TrimSpace(string(out))
		if newVersion != currentVersion {
			pterm.Info.Printfln("已升级: %s → %s", currentVersion, newVersion)
		} else {
			pterm.Info.Printfln("已是最新版本: %s", newVersion)
		}
	}

	// 如果配置了代理，同步设置 bun 镜像源
	return nil
}

// install 执行 bun 安装，支持代理
func (b *bunInitializer) install() error {
	var name string
	var args []string
	if runtime.GOOS == "windows" {
		name = "powershell"
		args = []string{"-ExecutionPolicy", "ByPass", "-c", "irm bun.sh/install.ps1 | iex"}
	} else {
		name = "sh"
		args = []string{"-c", "curl -fsSL https://bun.sh/install | bash"}
	}
	if err := runCommand(appendProxyEnv(os.Environ()), name, args...); err != nil {
		return fmt.Errorf("install command failed: %w", err)
	}
	return nil
}

// upgrade 升级 bun 到最新版本
func (b *bunInitializer) upgrade() error {
	bunPath, err := lookPath("bun")
	if err != nil {
		return fmt.Errorf("bun not found: %w", err)
	}
	return runCommand(nil, bunPath, "upgrade")
}

// ConfigureMirror 当 FEIKONG_PROXY_URL 不为空或 mirror 为 true 时，配置 bun 国内镜像源
func (b *bunInitializer) ConfigureMirror(mirror bool) {
	proxyURL := fkenv.Get(fkenv.ProxyURL)
	if proxyURL == "" && !mirror {
		return
	}

	// 确定 bunfig.toml 全局配置路径
	var configPath string
	if runtime.GOOS == "windows" {
		configPath = filepath.Join(os.Getenv("USERPROFILE"), ".bunfig.toml")
	} else {
		home, _ := os.UserHomeDir()
		configPath = filepath.Join(home, ".bunfig.toml")
	}

	// 镜像源配置内容（使用 npmmirror）
	mirrorConfig := `# 由 fkteams init 自动生成
[install]
registry = "https://registry.npmmirror.com"
`

	// 检查配置文件是否已存在且包含镜像配置
	if data, err := os.ReadFile(configPath); err == nil {
		if strings.Contains(string(data), "registry.npmmirror.com") {
			pterm.Info.Println("bun 镜像源已配置，跳过")
			return
		}
	}

	// 写入配置
	if err := os.WriteFile(configPath, []byte(mirrorConfig), 0644); err != nil {
		pterm.Error.Printfln("写入 bun 配置失败: %v", err)
		return
	}
	pterm.Success.Printfln("已配置 bun 镜像源: %s", configPath)
}
