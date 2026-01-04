package common

import "os"

func GenerateExampleEnv(filePath string) error {
	exampleContent := `# 这是一个示例的环境变量配置文件
# 请将此文件复制为 .env 并根据需要进行修改

# 模型配置配置
FEIKONG_OPENAI_BASE_URL =
FEIKONG_OPENAI_API_KEY = 
FEIKONG_OPENAI_MODEL = 

# 网络搜索工具配置代理
FEIKONG_PROXY_URL = 

# SSH 远程服务器配置
FEIKONG_SSH_VISITOR_ENABLED = false
FEIKONG_SSH_HOST =
FEIKONG_SSH_USERNAME = 
FEIKONG_SSH_PASSWORD =
`

	return os.WriteFile(filePath, []byte(exampleContent), 0644)
}
