package skill

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"fkteams/internal/app/appdata"
)

func installSkill(ctx context.Context, slug, version string, provider Provider) error {
	skillsDir := appdata.SkillsDir()
	targetDir := filepath.Join(skillsDir, slug)

	if _, err := os.Stat(targetDir); err == nil {
		if err := os.RemoveAll(targetDir); err != nil {
			return fmt.Errorf("删除旧版本失败: %w", err)
		}
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("创建技能目录失败: %w", err)
	}

	body, err := provider.Download(ctx, slug, version)
	if err != nil {
		_ = os.RemoveAll(targetDir)
		return err
	}
	defer body.Close()

	zipPath := filepath.Join(targetDir, slug+".zip")
	out, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, body); err != nil {
		_ = out.Close()
		_ = os.Remove(zipPath)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(zipPath)
		return err
	}

	if err := unzip(zipPath, targetDir); err != nil {
		_ = os.Remove(zipPath)
		return err
	}
	_ = os.Remove(zipPath)
	return nil
}

func unzip(src, destDir string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("打开 zip 失败: %w", err)
	}
	defer r.Close()

	cleanDest := filepath.Clean(destDir)
	for _, f := range r.File {
		targetPath := filepath.Join(destDir, f.Name)
		cleanTarget := filepath.Clean(targetPath)
		if !strings.HasPrefix(cleanTarget, cleanDest+string(os.PathSeparator)) {
			return fmt.Errorf("非法路径: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, f.Mode()); err != nil {
				return err
			}
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
			_ = outFile.Close()
			return err
		}

		_, copyErr := io.Copy(outFile, rc)
		closeErr := rc.Close()
		outCloseErr := outFile.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if outCloseErr != nil {
			return outCloseErr
		}
	}
	return nil
}
