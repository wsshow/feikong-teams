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

// uvInitializer 检测并安装/升级 uv（Python 包管理器）
type uvInitializer struct{}

func (u *uvInitializer) Name() string { return "uv" }

func (u *uvInitializer) Run() error {
	uvPath, err := exec.LookPath("uv")
	if err != nil {
		pterm.Warning.Println("未检测到 uv，正在安装...")
		if err := u.install(); err != nil {
			return err
		}
	} else {
		// 获取当前版本并升级
		out, _ := exec.Command(uvPath, "--version").CombinedOutput()
		currentVersion := strings.TrimSpace(string(out))
		pterm.Info.Printfln("当前版本: %s，正在检查更新...", currentVersion)

		if err := u.upgrade(uvPath); err != nil {
			return fmt.Errorf("upgrade failed: %w", err)
		}

		out, _ = exec.Command(uvPath, "--version").CombinedOutput()
		newVersion := strings.TrimSpace(string(out))
		if newVersion != currentVersion {
			pterm.Info.Printfln("已升级: %s → %s", currentVersion, newVersion)
		} else {
			pterm.Info.Printfln("已是最新版本: %s", newVersion)
		}
	}

	// 如果配置了代理，同步设置 uv 镜像源
	u.configureMirror()
	return nil
}

// install 执行 uv 安装，支持 FEIKONG_PROXY_URL 代理
func (u *uvInitializer) install() error {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell", "-ExecutionPolicy", "ByPass", "-c",
			"irm https://astral.sh/uv/install.ps1 | iex")
	} else {
		cmd = exec.Command("sh", "-c", "curl -LsSf https://astral.sh/uv/install.sh | sh")
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = appendProxyEnv(os.Environ())
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("install command failed: %w", err)
	}
	return nil
}

// upgrade 升级 uv 到最新版本
func (u *uvInitializer) upgrade(uvPath string) error {
	cmd := exec.Command(uvPath, "self", "update")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// configureMirror 当 FEIKONG_PROXY_URL 不为空时，配置 uv 国内镜像源
func (u *uvInitializer) configureMirror() {
	proxyURL := os.Getenv("FEIKONG_PROXY_URL")
	if proxyURL == "" {
		return
	}

	// 确定 uv 配置文件路径
	var configDir string
	if runtime.GOOS == "windows" {
		configDir = filepath.Join(os.Getenv("APPDATA"), "uv")
	} else {
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".config", "uv")
	}

	configPath := filepath.Join(configDir, "uv.toml")

	// 镜像源配置内容
	mirrorConfig := `# 由 fkteams --init 自动生成
python-install-mirror = "https://gh-proxy.com/https://github.com/astral-sh/python-build-standalone/releases/download"

[[index]]
url = "https://mirrors.aliyun.com/pypi/simple/"
default = true
`

	// 检查配置文件是否已存在且包含镜像配置
	if data, err := os.ReadFile(configPath); err == nil {
		if strings.Contains(string(data), "mirrors.aliyun.com") {
			pterm.Info.Println("uv 镜像源已配置，跳过")
			return
		}
	}

	// 创建目录并写入配置
	if err := os.MkdirAll(configDir, 0755); err != nil {
		pterm.Error.Printfln("创建 uv 配置目录失败: %v", err)
		return
	}
	if err := os.WriteFile(configPath, []byte(mirrorConfig), 0644); err != nil {
		pterm.Error.Printfln("写入 uv 配置失败: %v", err)
		return
	}
	pterm.Success.Printfln("已配置 uv 镜像源: %s", configPath)
}

// appendProxyEnv 如果设置了 FEIKONG_PROXY_URL，注入 HTTP_PROXY/HTTPS_PROXY 环境变量
func appendProxyEnv(env []string) []string {
	proxyURL := os.Getenv("FEIKONG_PROXY_URL")
	if proxyURL == "" {
		return env
	}
	pterm.Info.Printfln("使用代理: %s", proxyURL)
	env = append(env,
		"HTTP_PROXY="+proxyURL,
		"HTTPS_PROXY="+proxyURL,
		"http_proxy="+proxyURL,
		"https_proxy="+proxyURL,
	)
	return env
}
