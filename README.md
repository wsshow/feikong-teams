# fkteams

一个基于多智能体协作的命令行 AI 助手，采用监督者模式架构，通过多个专业智能体协同工作来完成复杂的编程和系统任务。

## 特性

- **监督者模式架构**：由"统御"智能体负责任务规划和智能体调度
- **多智能体协作**：包含四个专业智能体，各司其职
  - **小搜 (Searcher)**：信息搜索专家，支持 DuckDuckGo 网络搜索
  - **小码 (Coder)**：代码专家，支持安全的文件读写操作（限制在 code 目录）
  - **小令 (Cmder)**：命令行专家，支持跨平台命令执行
  - **小天 (Storyteller)**：讲故事专家，擅长创作和叙述
- **会话记忆**：支持上下文持久化，多轮对话记忆
- **流式输出**：实时显示智能体思考过程和工具调用
- **安全限制**：文件操作限制在指定目录，避免意外修改系统文件
- **彩色交互**：使用 pterm 提供美观的命令行交互体验

## 技术栈

- [Cloudwego Eino](https://github.com/cloudwego/eino) v0.7.15 - AI 编程框架
- OpenAI 兼容 API - 支持自定义 Base URL
- DuckDuckGo Search - 网络搜索工具

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
FEIKONG_OPENAI_MODEL=gpt-4

# 网络搜索工具配置（可选）
FEIKONG_PROXY_URL=http://127.0.0.1:7890
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
请输入您的问题: 帮我创建一个 Python 的 Hello World 程序
```

输入 `q`、`quit` 或直接回车退出程序。

## 安全说明

- **文件操作限制**：小码智能体的文件操作被限制在可执行文件同级的 `code/` 目录下，防止误操作系统文件
- **命令执行权限**：小令智能体会根据当前操作系统类型（Windows/Linux/macOS）执行相应的命令
- **工具调用可视化**：所有工具调用都会在终端显示，提供透明度

## 开发

### 项目结构

```
feikong-teams/
├── agents/           # 智能体实现
│   ├── leader/       # 统御智能体
│   ├── coder/        # 小码智能体
│   ├── searcher/     # 小搜智能体
│   ├── cmder/        # 小令智能体
│   └── storyteller/  # 小天智能体
├── tools/            # 工具实现
│   ├── command/      # 命令行工具
│   ├── file.go       # 文件操作工具
│   └── duckduckgo.go # 网络搜索工具
├── common/           # 公共组件
├── print/            # 输出格式化
└── main.go           # 入口文件
```

### 构建

```bash
# 清理构建产物
make clean

# 构建当前平台
make build

# 修改 Makefile 中的 os-archs 变量以支持其他平台
# 例如：os-archs=darwin:arm64 linux:amd64 windows:amd64
```

## 致谢

- [Cloudwego Eino](https://github.com/cloudwego/eino) - 强大的 AI 编程框架
- [Pterm](https://github.com/pterm/pterm) - 美观的终端 UI 库