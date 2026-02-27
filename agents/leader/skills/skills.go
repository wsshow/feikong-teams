package skills

import (
	"context"
	"path/filepath"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/skill"
)

func New(ctx context.Context, safeDir string) (skillsMiddleware adk.AgentMiddleware, err error) {
	skillsDirPath := filepath.Join(safeDir, "skills")
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
