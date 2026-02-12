package bootstrap

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pterm/pterm"
)

// bunInitializer 检测并安装/升级 bun（JavaScript 运行时和包管理器）
type bunInitializer struct{}

func (b *bunInitializer) Name() string { return "bun" }

func (b *bunInitializer) Run() error {
	bunPath, err := exec.LookPath("bun")
	if err != nil {
		pterm.Warning.Println("未检测到 bun，正在安装...")
		if err := b.install(); err != nil {
			return err
		}
	} else {
		// 获取当前版本并升级
		out, _ := exec.Command(bunPath, "--version").CombinedOutput()
		currentVersion := strings.TrimSpace(string(out))
		pterm.Info.Printfln("当前版本: %s，正在检查更新...", currentVersion)

		if err := b.upgrade(); err != nil {
			return fmt.Errorf("upgrade failed: %w", err)
		}

		out, _ = exec.Command(bunPath, "--version").CombinedOutput()
		newVersion := strings.TrimSpace(string(out))
		if newVersion != currentVersion {
			pterm.Info.Printfln("已升级: %s → %s", currentVersion, newVersion)
		} else {
			pterm.Info.Printfln("已是最新版本: %s", newVersion)
		}
	}

	// 如果配置了代理，同步设置 bun 镜像源
	b.configureMirror()
	return nil
}

// install 执行 bun 安装，支持代理
func (b *bunInitializer) install() error {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell", "-ExecutionPolicy", "ByPass", "-c",
			"irm bun.sh/install.ps1 | iex")
	} else {
		cmd = exec.Command("sh", "-c", "curl -fsSL https://bun.sh/install | bash")
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = appendProxyEnv(os.Environ())
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("install command failed: %w", err)
	}
	return nil
}

// upgrade 升级 bun 到最新版本
func (b *bunInitializer) upgrade() error {
	bunPath, err := exec.LookPath("bun")
	if err != nil {
		return fmt.Errorf("bun not found: %w", err)
	}
	cmd := exec.Command(bunPath, "upgrade")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// configureMirror 当 FEIKONG_PROXY_URL 不为空时，配置 bun 国内镜像源
func (b *bunInitializer) configureMirror() {
	proxyURL := os.Getenv("FEIKONG_PROXY_URL")
	if proxyURL == "" {
		return
	}

	// 确定 bunfig.toml 全局配置路径
	var configDir string
	if runtime.GOOS == "windows" {
		configDir = filepath.Join(os.Getenv("USERPROFILE"), ".bunfig")
	} else {
		home, _ := os.UserHomeDir()
		configDir = home
	}

	configPath := filepath.Join(configDir, "bunfig.toml")

	// 镜像源配置内容（使用 npmmirror）
	mirrorConfig := `# 由 fkteams --init 自动生成
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
