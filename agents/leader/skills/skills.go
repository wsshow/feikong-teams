package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/skill"
)

func ensureDir(path string) error {
	_, err := os.Stat(path)
	if err == nil {
		return nil
	}
	if os.IsNotExist(err) {
		return os.MkdirAll(path, 0755)
	}
	return err
}

func New(ctx context.Context, safeDir string) (skillsMiddleware adk.AgentMiddleware, err error) {
	skillsDirPath := filepath.Join(safeDir, "skills")

	if err := ensureDir(skillsDirPath); err != nil {
		return skillsMiddleware, fmt.Errorf("无法创建或访问目录 %s: %w", skillsDirPath, err)
	}

	localBackend, err := skill.NewLocalBackend(&skill.LocalBackendConfig{
		BaseDir: skillsDirPath,
	})
	if err != nil {
		return skillsMiddleware, err
	}
	skillsMiddleware, err = skill.New(ctx, &skill.Config{
		Backend:    localBackend,
		UseChinese: true,
	})
	if err != nil {
		return skillsMiddleware, err
	}
	return skillsMiddleware, nil
}
