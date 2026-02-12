package bootstrap

import (
	"fmt"
	"os"
	"os/exec"
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
		return u.install()
	}

	// 获取当前版本
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
	return nil
}

// install 执行 uv 安装
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
