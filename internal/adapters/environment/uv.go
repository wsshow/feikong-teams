package environment

import (
	"fkteams/internal/runtime/env"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pterm/pterm"
)

// uvInitializer 检测并安装/升级 uv（Python 包管理器）
type uvInitializer struct{}

func (u *uvInitializer) Name() string { return "uv" }

func (u *uvInitializer) Run() error {
	uvPath, err := lookPath("uv")
	if err != nil {
		pterm.Warning.Println("未检测到 uv，正在安装...")
		if err := u.install(); err != nil {
			return err
		}
	} else {
		// 获取当前版本并升级
		out, err := combinedOutput(uvPath, "--version")
		if err != nil {
			return fmt.Errorf("get current uv version: %w", err)
		}
		currentVersion := strings.TrimSpace(string(out))
		pterm.Info.Printfln("当前版本: %s，正在检查更新...", currentVersion)

		if err := u.upgrade(uvPath); err != nil {
			return fmt.Errorf("upgrade failed: %w", err)
		}

		out, err = combinedOutput(uvPath, "--version")
		if err != nil {
			return fmt.Errorf("get updated uv version: %w", err)
		}
		newVersion := strings.TrimSpace(string(out))
		if newVersion != currentVersion {
			pterm.Info.Printfln("已升级: %s → %s", currentVersion, newVersion)
		} else {
			pterm.Info.Printfln("已是最新版本: %s", newVersion)
		}
	}

	// 如果配置了代理，同步设置 uv 镜像源
	return nil
}

// install 执行 uv 安装，支持 FEIKONG_PROXY_URL 代理
func (u *uvInitializer) install() error {
	var name string
	var args []string
	if runtime.GOOS == "windows" {
		name = "powershell"
		args = []string{"-ExecutionPolicy", "ByPass", "-c", "irm https://astral.sh/uv/install.ps1 | iex"}
	} else {
		name = "sh"
		args = []string{"-c", "curl -LsSf https://astral.sh/uv/install.sh | sh"}
	}
	if err := runCommand(appendProxyEnv(os.Environ()), name, args...); err != nil {
		return fmt.Errorf("install command failed: %w", err)
	}
	return nil
}

// upgrade 升级 uv 到最新版本
func (u *uvInitializer) upgrade(uvPath string) error {
	return runCommand(nil, uvPath, "self", "update")
}

// ConfigureMirror 当 FEIKONG_PROXY_URL 不为空或 mirror 为 true 时，配置 uv 国内镜像源
func (u *uvInitializer) ConfigureMirror(mirror bool) {
	proxyURL := env.Get(env.ProxyURL)
	if proxyURL == "" && !mirror {
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

	existing, err := loadMirrorConfig(configPath)
	if err != nil {
		pterm.Error.Printfln("读取 uv 配置失败: %v", err)
		return
	}
	updated, changed, err := mergeUVMirrorConfig(existing)
	if err != nil {
		pterm.Error.Printfln("合并 uv 配置失败: %v", err)
		return
	}
	if !changed {
		pterm.Info.Println("uv 镜像源已配置，跳过")
		return
	}

	if err := saveMirrorConfig(configPath, updated); err != nil {
		pterm.Error.Printfln("写入 uv 配置失败: %v", err)
		return
	}
	pterm.Success.Printfln("已配置 uv 镜像源: %s", configPath)
}
