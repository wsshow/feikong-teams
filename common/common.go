package common

import (
	"os"
)

func GenerateExampleEnv(filePath string) error {
	exampleContent := `# 这是一个示例的环境变量配置文件
# 请将此文件复制为 .env 并根据需要进行修改

# 模型配置配置
FEIKONG_OPENAI_BASE_URL = https://api.openai.com/v1
FEIKONG_OPENAI_API_KEY = xxxxx
FEIKONG_OPENAI_MODEL = GPT-5

# 配置代理：网络搜索工具、程序更新等
FEIKONG_PROXY_URL = http://127.0.0.1:7890

# 文件工具的使用目录, 默认为: ./workspace
FEIKONG_FILE_TOOL_DIR = ./workspace

# Todo工具的使用目录, 默认为: ./workspace
FEIKONG_TODO_TOOL_DIR = ./workspace

# git 工具的使用目录, 默认为: ./workspace
FEIKONG_GIT_TOOL_DIR = ./workspace

# excel 工具的使用目录, 默认为: ./workspace
FEIKONG_EXCEL_TOOL_DIR = ./workspace

# uv 工具的使用目录, 默认为: ./workspace
FEIKONG_UV_TOOL_DIR = ./workspace

# bun 工具的使用目录, 默认为: ./workspace
FEIKONG_BUN_TOOL_DIR = ./workspace

# 代码助手
FEIKONG_CODER_ENABLED = false

# 本地命令行助手
FEIKONG_CMDER_ENABLED = false

# 数据分析师
FEIKONG_ANALYST_ENABLED = false

# SSH 远程服务器配置
FEIKONG_SSH_VISITOR_ENABLED = false
FEIKONG_SSH_HOST =
FEIKONG_SSH_USERNAME = 
FEIKONG_SSH_PASSWORD =
`

	return os.WriteFile(filePath, []byte(exampleContent), 0644)
}
