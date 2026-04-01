package skill

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	commonPkg "fkteams/common"

	"github.com/pterm/pterm"
	ucli "github.com/urfave/cli/v3"
)

func installCommand() *ucli.Command {
	return &ucli.Command{
		Name:      "install",
		Usage:     "从技能市场安装技能",
		ArgsUsage: "<技能slug>",
		Flags: []ucli.Flag{
			&ucli.StringFlag{
				Name:  "version",
				Usage: "技能版本（留空为最新版本）",
			},
			&ucli.StringFlag{
				Name:  "provider",
				Usage: "指定后端，可选: " + strings.Join(ProviderNames(), ", "),
			},
		},
		Action: func(ctx context.Context, cmd *ucli.Command) error {
			slug := cmd.Args().First()
			if slug == "" {
				return fmt.Errorf("请提供技能 slug，例如: fkteams skill install video-frames")
			}
			version := cmd.String("version")

			var provider Provider
			if name := cmd.String("provider"); name != "" {
				provider = GetProviderByName(name)
				if provider == nil {
					return fmt.Errorf("未找到后端: %s", name)
				}
			} else {
				provider = GetDefaultProvider()
			}
			if provider == nil {
				return fmt.Errorf("无可用的技能后端")
			}

			return installSkill(ctx, slug, version, provider)
		},
	}
}

func installSkill(ctx context.Context, slug, version string, provider Provider) error {

	skillsDir := filepath.Join(commonPkg.AppDir(), "skills")
	targetDir := filepath.Join(skillsDir, slug)

	// 检查是否已安装
	if _, err := os.Stat(targetDir); err == nil {
		pterm.Warning.Printfln("技能 %s 已存在，将覆盖安装", slug)
		if err := os.RemoveAll(targetDir); err != nil {
			return fmt.Errorf("删除旧版本失败: %w", err)
		}
	}

	// 创建技能目录
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("创建技能目录失败: %w", err)
	}

	// 下载
	versionLabel := version
	if versionLabel == "" {
		versionLabel = "latest"
	}
	spinner, _ := pterm.DefaultSpinner.Start(fmt.Sprintf("正在从 %s 下载 %s@%s...", provider.Name(), slug, versionLabel))

	body, err := provider.Download(ctx, slug, version)
	if err != nil {
		spinner.Fail(fmt.Sprintf("下载失败: %v", err))
		os.Remove(targetDir)
		return err
	}
	defer body.Close()

	// 写入 zip 到技能目录
	zipPath := filepath.Join(targetDir, slug+".zip")
	out, err := os.Create(zipPath)
	if err != nil {
		spinner.Fail(fmt.Sprintf("创建文件失败: %v", err))
		return err
	}
	if _, err := io.Copy(out, body); err != nil {
		out.Close()
		os.Remove(zipPath)
		spinner.Fail(fmt.Sprintf("写入文件失败: %v", err))
		return err
	}
	out.Close()

	spinner.UpdateText("正在解压...")

	// 解压到技能目录
	if err := unzip(zipPath, targetDir); err != nil {
		spinner.Fail(fmt.Sprintf("解压失败: %v", err))
		os.Remove(zipPath)
		return err
	}

	// 删除 zip
	os.Remove(zipPath)

	spinner.Success(fmt.Sprintf("技能 %s@%s 安装成功", slug, versionLabel))
	pterm.FgGray.Printfln("安装路径: %s", targetDir)
	return nil
}

func unzip(src, destDir string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("打开 zip 失败: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		// 防止路径穿越攻击
		targetPath := filepath.Join(destDir, f.Name)
		if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("非法路径: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(targetPath, f.Mode())
			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}

		outFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
