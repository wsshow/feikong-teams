package commands

import (
	"errors"
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// loadEnv 加载 .env 文件，若文件不存在则静默跳过（支持 Docker 等通过系统环境变量配置的场景）
func loadEnv() error {
	if err := godotenv.Load(); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		fmt.Println("加载 .env 文件失败:", err)
		return err
	}
	return nil
}
