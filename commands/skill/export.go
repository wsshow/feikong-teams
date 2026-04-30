package skill

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	commonPkg "fkteams/common"

	"github.com/goccy/go-yaml"
)

// LocalSkillInfo represents a locally installed skill
type LocalSkillInfo struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ListLocalSkills lists all locally installed skills
func ListLocalSkills() ([]LocalSkillInfo, error) {
	skillsDir := filepath.Join(commonPkg.AppDir(), "skills")

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read skills dir: %w", err)
	}

	var skills []LocalSkillInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillFile := filepath.Join(skillsDir, entry.Name(), "SKILL.md")
		data, err := os.ReadFile(skillFile)
		if err != nil {
			continue
		}

		content := string(data)
		parts := strings.SplitN(content, "---", 3)
		if len(parts) < 3 {
			continue
		}

		var info struct {
			Name        string `yaml:"name"`
			Description string `yaml:"description"`
		}
		if err := yaml.Unmarshal([]byte(parts[1]), &info); err != nil {
			continue
		}

		name := info.Name
		if name == "" {
			name = entry.Name()
		}

		skills = append(skills, LocalSkillInfo{
			Slug:        entry.Name(),
			Name:        name,
			Description: strings.Join(strings.Fields(info.Description), " "),
		})
	}

	return skills, nil
}

// SkillFileEntry represents a file in a skill directory
type SkillFileEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

// ListSkillFiles lists files in a skill directory (non-recursive for a given subpath)
func ListSkillFiles(slug, subPath string) ([]SkillFileEntry, error) {
	skillsDir := filepath.Join(commonPkg.AppDir(), "skills", slug)

	// prevent path traversal
	cleanSub := filepath.Clean(filepath.ToSlash(subPath))
	if strings.HasPrefix(cleanSub, "..") {
		return nil, fmt.Errorf("invalid path")
	}

	targetDir := filepath.Join(skillsDir, cleanSub)
	cleanTarget := filepath.Clean(targetDir)
	if !strings.HasPrefix(cleanTarget, filepath.Clean(skillsDir)+string(os.PathSeparator)) && cleanTarget != filepath.Clean(skillsDir) {
		return nil, fmt.Errorf("invalid path")
	}

	entries, err := os.ReadDir(targetDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("skill %s is not installed", slug)
		}
		return nil, err
	}

	var result []SkillFileEntry
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		result = append(result, SkillFileEntry{
			Name:  e.Name(),
			Path:  filepath.ToSlash(filepath.Join(cleanSub, e.Name())),
			IsDir: e.IsDir(),
			Size:  info.Size(),
		})
	}
	return result, nil
}

// ReadSkillFile reads a file from a skill directory
func ReadSkillFile(slug, filePath string) (string, error) {
	skillsDir := filepath.Join(commonPkg.AppDir(), "skills", slug)
	fullPath := filepath.Join(skillsDir, filePath)

	// prevent path traversal
	cleanPath := filepath.Clean(fullPath)
	if !strings.HasPrefix(cleanPath, filepath.Clean(skillsDir)+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid path")
	}

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// InstallSkillFromProvider installs a skill from the given provider
func InstallSkillFromProvider(ctx context.Context, slug, version string, provider Provider) error {
	return installSkill(ctx, slug, version, provider)
}

// RemoveLocalSkill removes an installed skill by slug
func RemoveLocalSkill(slug string) error {
	targetDir := filepath.Join(commonPkg.AppDir(), "skills", slug)
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		return fmt.Errorf("skill %s is not installed", slug)
	}
	return os.RemoveAll(targetDir)
}
