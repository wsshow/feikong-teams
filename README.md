# fkteams

一个基于多智能体协作的命令行 AI 助手，支持**团队模式**和**多智能体讨论模式（圆桌会议**两种工作方式，通过多个专业智能体协同工作来完成复杂的编程和系统任务。

## 特性

### 双模式架构

#### 团队模式 (Team Mode)
- **监督者模式架构**：由"统御"智能体负责任务规划和智能体调度
- **多智能体协作**：包含五个专业智能体，各司其职
  - **小搜 (Searcher)**：信息搜索专家，支持 DuckDuckGo 网络搜索
  - **小码 (Coder)**：代码专家，支持安全的文件读写操作（限制在 code 目录）
  - **小令 (Cmder)**：命令行专家，支持跨平台命令执行
  - **小访 (Visitor)**：SSH 访问专家，支持通过 SSH 连接远程服务器执行命令和传输文件
  - **小天 (Storyteller)**：讲故事专家，擅长创作和叙述

#### 多智能体讨论模式 / 圆桌会议模式 (Roundtable Mode)
- **多模型协作**：支持配置不同的 AI 模型（如 DeepSeek、Claude、GPT 等）作为圆桌讨论成员
- **多轮深度讨论**：多个智能体就同一问题进行多轮讨论，充分吸纳不同角度的分析意见
- **观点融合**：每个讨论者参考他人意见的同时给出独到见解，最终形成更全面准确的结论
- **可配置迭代**：通过 `max_iterations` 控制讨论轮数，平衡讨论深度与效率

### 无缝模式切换
- **实时切换**：在运行时输入 `switch_work_mode` 即可在团队模式和讨论模式之间切换
- **上下文保留**：切换模式时完整保留所有对话历史，无需重新开始
- **灵活应对**：简单任务用团队模式快速执行，复杂决策切换到讨论模式深入分析

### 完整的历史记录
- **会话记忆**：支持上下文持久化，多轮对话记忆
- **讨论日志**：记录所有智能体的发言和讨论过程
- **Markdown 导出**：可将完整聊天历史导出为 Markdown 文件，便于存档和分享
- **历史加载**：支持加载之前保存的聊天历史，继续未完成的对话

### 其他特性
- **流式输出**：实时显示智能体思考过程和工具调用
- **安全限制**：文件操作限制在指定目录，避免意外修改系统文件
- **彩色交互**：使用 pterm 提供美观的命令行交互体验
- **自动更新**：支持在线检查和更新到最新版本

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

### 3. 配置圆桌会议成员（可选）

生成示例配置文件：

```bash
./fkteams -c
# 或
./fkteams --generate-config
```

编辑 `config/config.toml` 配置圆桌会议成员：

```toml
[roundtable]
max_iterations = 2  # 讨论轮数

[[roundtable.members]]
index = 0
name = '深度求索'
desc = '深度求索聊天模型，擅长逻辑分析'
base_url = 'https://api.deepseek.com/v1'
api_key = 'your_deepseek_api_key'
model_name = 'deepseek-chat'

[[roundtable.members]]
index = 1
name = '克劳德'
desc = '克劳德聊天模型，擅长创意思维'
base_url = 'https://api.anthropic.com/v1'
api_key = 'your_claude_api_key'
model_name = 'claude-3-sonnet'

[[roundtable.members]]
index = 2
name = 'GPT'
desc = 'OpenAI GPT 模型，擅长综合分析'
base_url = 'https://api.openai.com/v1'
api_key = 'your_openai_api_key'
model_name = 'gpt-4'
```

### 4. 运行

#### 方式一：直接运行

```bash
# 默认启动团队模式
go run main.go

# 启动多智能体讨论模式
go run main.go -m group
```

#### 方式二：编译后运行

```bash
make build

# 默认启动团队模式
./release/fkteams_darwin_arm64

# 启动多智能体讨论模式
./release/fkteams_darwin_arm64 -m group
```

### 5. 使用

启动后，在命令行输入你的问题或任务：

```
请输入您的问题: 帮我写几篇相互关联的小小说，然后创建一个网站来展示这些小说。
```

#### 常用命令

| 命令                            | 说明                              |
| ------------------------------- | --------------------------------- |
| `quit` / `q`                    | 退出程序                          |
| `switch_work_mode`              | 切换工作模式（团队模式/讨论模式） |
| `save_chat_history`             | 保存聊天历史                      |
| `load_chat_history`             | 加载聊天历史                      |
| `clear_chat_history`            | 清空聊天历史                      |
| `save_chat_history_to_markdown` | 导出聊天历史为 Markdown 文件      |
| `clear_todo`                    | 清空待办事项                      |
| `help`                          | 显示帮助信息                      |

#### 命令行参数

| 参数                | 简写 | 说明                                       |
| ------------------- | ---- | ------------------------------------------ |
| `--work-mode`       | `-m` | 工作模式: `team`（团队）或 `group`（讨论） |
| `--version`         | `-v` | 显示版本信息                               |
| `--update`          | `-u` | 检查并更新到最新版本                       |
| `--generate-env`    | `-g` | 生成示例 .env 文件                         |
| `--generate-config` | `-c` | 生成示例配置文件                           |

## 圆桌会议模式详解

### 工作原理

圆桌会议模式模拟了一场专家研讨会：

1. **问题提出**：用户提出问题或任务
2. **轮流发言**：每个配置的模型依次针对问题发表观点
3. **观点参考**：后发言的模型可以看到前面模型的观点，并在此基础上补充或提出不同见解
4. **多轮迭代**：根据 `max_iterations` 配置进行多轮讨论，逐步深化分析
5. **形成共识**：最终综合各方观点，给出更全面准确的答案

### 适用场景

- **复杂决策**：需要从多角度分析的重要决策
- **创意头脑风暴**：激发不同模型的创意火花
- **观点验证**：让多个模型相互验证，减少单一模型的偏见
- **深度分析**：需要多轮思考才能得出结论的复杂问题

### 配置建议

- 选择不同特点的模型作为讨论成员，以获得更多元的观点
- `max_iterations` 建议设置为 1-3，过多轮次可能导致观点趋同
- 可以给每个成员设置描述性的 `desc`，帮助理解其专长

## 安全说明

- **文件操作限制**：小码智能体的文件操作被限制在可执行文件同级的 `code/` 目录下，防止误操作系统文件
- **命令执行权限**：小令智能体会根据当前操作系统类型（Windows/Linux/macOS）执行相应的命令
- **SSH 连接管理**：小访智能体通过 SSH 连接远程服务器，确保连接信息安全存储和使用
- **日志记录**：所有智能体的操作和输出都会被记录，可以主动输出成 markdown 文件，便于审计和调试
- **工具调用可视化**：所有工具调用都会在终端显示，提供透明度
- **环境变量保护**：请确保 `.env` 文件不被泄露，避免敏感信息外泄

## 构建

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