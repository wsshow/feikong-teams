package skill

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"fkteams/internal/app/appdata"
)

const (
	maxSkillArchiveBytes   int64 = 64 << 20
	maxSkillExtractedBytes int64 = 256 << 20
	maxSkillArchiveEntries       = 10_000
)

var skillInstallMu sync.Mutex

func installSkill(ctx context.Context, slug, version string, provider Provider) error {
	if err := validateSkillSlug(slug); err != nil {
		return err
	}
	if provider == nil {
		return fmt.Errorf("skill provider is nil")
	}

	skillInstallMu.Lock()
	defer skillInstallMu.Unlock()

	skillsDir := appdata.SkillsDir()
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return fmt.Errorf("create skills directory: %w", err)
	}
	stagingDir, err := os.MkdirTemp(skillsDir, ".install-"+slug+"-")
	if err != nil {
		return fmt.Errorf("create skill staging directory: %w", err)
	}
	defer os.RemoveAll(stagingDir)

	body, err := provider.Download(ctx, slug, version)
	if err != nil {
		return fmt.Errorf("download skill: %w", err)
	}
	archivePath := filepath.Join(stagingDir, "skill.zip")
	copyErr := writeSkillArchive(ctx, archivePath, body)
	closeErr := body.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return fmt.Errorf("close skill download: %w", closeErr)
	}

	extractedDir := filepath.Join(stagingDir, "content")
	if err := os.Mkdir(extractedDir, 0755); err != nil {
		return fmt.Errorf("create extracted skill directory: %w", err)
	}
	if err := unzipSkill(ctx, archivePath, extractedDir); err != nil {
		return err
	}
	if info, err := os.Stat(filepath.Join(extractedDir, "SKILL.md")); err != nil || !info.Mode().IsRegular() {
		if err != nil {
			return fmt.Errorf("installed skill is missing SKILL.md: %w", err)
		}
		return fmt.Errorf("installed skill SKILL.md is not a regular file")
	}

	return replaceInstalledSkill(filepath.Join(skillsDir, slug), extractedDir, stagingDir)
}

func writeSkillArchive(ctx context.Context, filePath string, body io.Reader) error {
	return writeSkillArchiveLimited(ctx, filePath, body, maxSkillArchiveBytes)
}

func writeSkillArchiveLimited(ctx context.Context, filePath string, body io.Reader, limit int64) error {
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("create skill archive: %w", err)
	}
	written, copyErr := io.Copy(file, io.LimitReader(&skillContextReader{ctx: ctx, reader: body}, limit+1))
	if copyErr == nil && written > limit {
		copyErr = fmt.Errorf("skill archive exceeds %d bytes", limit)
	}
	if copyErr == nil {
		copyErr = file.Sync()
	}
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(filePath)
		return fmt.Errorf("write skill archive: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(filePath)
		return fmt.Errorf("close skill archive: %w", closeErr)
	}
	return nil
}

func unzipSkill(ctx context.Context, src, destDir string) error {
	archive, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("open skill archive: %w", err)
	}
	defer archive.Close()
	if len(archive.File) > maxSkillArchiveEntries {
		return fmt.Errorf("skill archive exceeds %d entries", maxSkillArchiveEntries)
	}

	cleanDest := filepath.Clean(destDir)
	seen := make(map[string]struct{}, len(archive.File))
	var extractedBytes int64
	for _, entry := range archive.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		targetPath, err := safeArchiveTarget(cleanDest, entry.Name)
		if err != nil {
			return err
		}
		if _, exists := seen[targetPath]; exists {
			return fmt.Errorf("duplicate skill archive path: %s", entry.Name)
		}
		seen[targetPath] = struct{}{}

		mode := entry.Mode()
		if mode&os.ModeSymlink != 0 || (!entry.FileInfo().IsDir() && !mode.IsRegular()) {
			return fmt.Errorf("unsupported skill archive entry: %s", entry.Name)
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return fmt.Errorf("create skill directory: %w", err)
			}
			continue
		}
		if entry.UncompressedSize64 > uint64(maxSkillExtractedBytes-extractedBytes) {
			return fmt.Errorf("extracted skill exceeds %d bytes", maxSkillExtractedBytes)
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return fmt.Errorf("create skill parent directory: %w", err)
		}
		if err := extractSkillFile(ctx, entry, targetPath, &extractedBytes); err != nil {
			return err
		}
	}
	return nil
}

func safeArchiveTarget(cleanDest, archivePath string) (string, error) {
	if archivePath == "" || strings.ContainsRune(archivePath, '\x00') {
		return "", fmt.Errorf("invalid skill archive path")
	}
	cleanRelative := filepath.Clean(filepath.FromSlash(archivePath))
	if cleanRelative == "." || filepath.IsAbs(cleanRelative) || filepath.VolumeName(cleanRelative) != "" {
		return "", fmt.Errorf("invalid skill archive path: %s", archivePath)
	}
	targetPath := filepath.Join(cleanDest, cleanRelative)
	relative, err := filepath.Rel(cleanDest, targetPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("invalid skill archive path: %s", archivePath)
	}
	return targetPath, nil
}

func extractSkillFile(ctx context.Context, entry *zip.File, targetPath string, extractedBytes *int64) error {
	reader, err := entry.Open()
	if err != nil {
		return fmt.Errorf("open skill archive entry: %w", err)
	}
	permissions := os.FileMode(0644)
	if entry.Mode().Perm()&0111 != 0 {
		permissions = 0755
	}
	file, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, permissions)
	if err != nil {
		_ = reader.Close()
		return fmt.Errorf("create skill file: %w", err)
	}
	remaining := maxSkillExtractedBytes - *extractedBytes
	written, copyErr := io.Copy(file, io.LimitReader(&skillContextReader{ctx: ctx, reader: reader}, remaining+1))
	if copyErr == nil && written > remaining {
		copyErr = fmt.Errorf("extracted skill exceeds %d bytes", maxSkillExtractedBytes)
	}
	if copyErr == nil {
		copyErr = file.Sync()
	}
	readerCloseErr := reader.Close()
	fileCloseErr := file.Close()
	if copyErr != nil {
		return fmt.Errorf("extract skill file: %w", copyErr)
	}
	if readerCloseErr != nil {
		return fmt.Errorf("close skill archive entry: %w", readerCloseErr)
	}
	if fileCloseErr != nil {
		return fmt.Errorf("close skill file: %w", fileCloseErr)
	}
	*extractedBytes += written
	return nil
}

func replaceInstalledSkill(targetDir, extractedDir, stagingDir string) error {
	backupDir := filepath.Join(stagingDir, "previous")
	hasPrevious := false
	if _, err := os.Lstat(targetDir); err == nil {
		if err := os.Rename(targetDir, backupDir); err != nil {
			return fmt.Errorf("stage previous skill: %w", err)
		}
		hasPrevious = true
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect previous skill: %w", err)
	}
	if err := os.Rename(extractedDir, targetDir); err != nil {
		if hasPrevious {
			if restoreErr := os.Rename(backupDir, targetDir); restoreErr != nil {
				return fmt.Errorf("install skill: %w; restore previous skill: %v", err, restoreErr)
			}
		}
		return fmt.Errorf("install skill: %w", err)
	}
	return nil
}

type skillContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *skillContextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}
