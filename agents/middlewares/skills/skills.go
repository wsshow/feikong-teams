package skills

import (
	"context"
	"fkteams/agents/middlewares/fkfs"
	"fkteams/common"
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

func New(ctx context.Context) (skillsMiddleware adk.ChatModelAgentMiddleware, err error) {
	skillsDirPath := filepath.Join(common.AppDir(), "skills")

	if err := ensureDir(skillsDirPath); err != nil {
		return skillsMiddleware, fmt.Errorf("无法创建或访问目录 %s: %w", skillsDirPath, err)
	}

	fkBackend, err := fkfs.NewLocalBackend(skillsDirPath)
	if err != nil {
		return skillsMiddleware, fmt.Errorf("无法创建本地后端: %w", err)
	}

	localBackend, err := skill.NewBackendFromFilesystem(ctx, &skill.BackendFromFilesystemConfig{
		Backend: fkBackend,
		BaseDir: skillsDirPath,
	})
	if err != nil {
		return skillsMiddleware, err
	}
	skillsMiddleware, err = skill.NewMiddleware(ctx, &skill.Config{
		Backend:    localBackend,
		UseChinese: true,
	})
	if err != nil {
		return skillsMiddleware, err
	}
	return skillsMiddleware, nil
}
