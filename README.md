# fkteams

一个基于多智能体协作的命令行 AI 助手，采用监督者模式架构，通过多个专业智能体协同工作来完成复杂的编程和系统任务。

## 特性

- **监督者模式架构**：由"统御"智能体负责任务规划和智能体调度
- **多智能体协作**：包含五个专业智能体，各司其职
  - **小搜 (Searcher)**：信息搜索专家，支持 DuckDuckGo 网络搜索
  - **小码 (Coder)**：代码专家，支持安全的文件读写操作（限制在 code 目录）
  - **小令 (Cmder)**：命令行专家，支持跨平台命令执行
  - **小访 (Visitor)**：SSH 访问专家，支持通过 SSH 连接远程服务器执行命令和传输文件
  - **小天 (Storyteller)**：讲故事专家，擅长创作和叙述
- **会话记忆**：支持上下文持久化，多轮对话记忆
- **流式输出**：实时显示智能体思考过程和工具调用
- **安全限制**：文件操作限制在指定目录，避免意外修改系统文件
- **彩色交互**：使用 pterm 提供美观的命令行交互体验

## 快速开始

### 1. 克隆项目

```bash
git clone https://github.com/yourusername/feikong-teams.git
cd feikong-teams
```

### 2. 配置环境变量

复制 `.env.example` 为 `.env` 并配置：

```bash
cp .env.example .env
```

编辑 `.env` 文件，填写必要的配置：

```env
# 模型配置
FEIKONG_OPENAI_API_KEY=your_api_key_here
FEIKONG_OPENAI_BASE_URL=https://api.openai.com/v1
FEIKONG_OPENAI_MODEL=gpt-5

# 网络搜索工具配置（可选）
FEIKONG_PROXY_URL=http://127.0.0.1:7890

# SSH 访问者智能体配置（可选）
FEIKONG_SSH_VISITOR_ENABLED=true # 设置为 true 启用小访智能体
FEIKONG_SSH_HOST=ip:port
FEIKONG_SSH_USERNAME=your_ssh_user
FEIKONG_SSH_PASSWORD=your_ssh_password
```

### 3. 运行

#### 方式一：直接运行

```bash
go run main.go
```

#### 方式二：编译后运行

```bash
make build
./release/fkteams_darwin_arm64  # macOS ARM64
```

### 4. 使用

启动后，在命令行输入你的问题或任务：

```
请输入您的问题: 帮我写几篇相互关联的小小说，然后创建一个网站来展示这些小说。
```

输入 `q`、`quit` 或直接回车退出程序。

## 安全说明

- **文件操作限制**：小码智能体的文件操作被限制在可执行文件同级的 `code/` 目录下，防止误操作系统文件
- **命令执行权限**：小令智能体会根据当前操作系统类型（Windows/Linux/macOS）执行相应的命令
- **SSH 连接管理**：小访智能体通过 SSH 连接远程服务器，确保连接信息安全存储和使用
- **日志记录**：所有智能体的操作和输出都会被记录，可以主动输出成 markdown 文件，便于审计和调试
- **工具调用可视化**：所有工具调用都会在终端显示，提供透明度
- **环境变量保护**：请确保 `.env` 文件不被泄露，避免敏感信息外泄

### 构建

```bash
# 清理构建产物
make clean

# 构建当前平台
make build

# 修改 Makefile 中的 os-archs 变量以支持其他平台
# 例如：os-archs=darwin:arm64 linux:amd64 windows:amd64
```

## 许可证

本项目采用 MIT 许可证。详见 [LICENSE](LICENSE) 文件。

## 致谢

- [go-prompt](https://github.com/c-bata/go-prompt) - 交互式命令行提示库
- [Pterm](https://github.com/pterm/pterm) - 美观的终端 UI 库
- [Cloudwego Eino](https://github.com/cloudwego/eino) - 强大的 AI 编程框架