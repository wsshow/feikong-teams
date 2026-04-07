// Package fkenv 集中管理所有 FEIKONG_ 前缀的环境变量
package fkenv

import "os"

// 环境变量名称常量
const (
	AppDir                 = "FEIKONG_APP_DIR"                   // 应用数据目录 (默认 ~/.fkteams)
	ProxyURL               = "FEIKONG_PROXY_URL"                 // 代理地址
	MaxIterations          = "FEIKONG_MAX_ITERATIONS"            // 智能体最大迭代次数
	NoSelfRestart          = "FEIKONG_NO_SELF_RESTART"           // 禁用自动重启（systemd 等场景）
	MaxTokensBeforeSummary = "FEIKONG_MAX_TOKENS_BEFORE_SUMMARY" // 触发摘要的 token 阈值
)

// Get 读取指定环境变量
func Get(key string) string {
	return os.Getenv(key)
}
