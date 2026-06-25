# fkteams 非空小队

fkteams（FeiKong Teams，非空小队）是一个开源的多智能体协作 AI 助手，适合代码开发、资料研究、数据分析、远程运维和自动化任务。它支持 Web UI、CLI、OpenAI 兼容 API 和消息通道（Discord / QQ / 微信）多种入口。

![非空小队架构简介](./docs/images/fkteams.png)

## 演示图

|                     登录界面                     |                   主界面                    |
| :----------------------------------------------: | :-----------------------------------------: |
| ![登录界面](./docs/images/fkteams_web_login.png) | ![主界面](./docs/images/fkteams_web_mp.png) |

|                   审批                    |                    子任务进行中                    |
| :---------------------------------------: | :------------------------------------------------: |
| ![审批](./docs/images/fkteams_web_sp.png) | ![子任务进行中](./docs/images/fkteams_web_ing.png) |

|                    子任务完成                     |                   文件管理                    |
| :-----------------------------------------------: | :-------------------------------------------: |
| ![子任务完成](./docs/images/fkteams_web_done.png) | ![文件管理](./docs/images/fkteams_web_fm.png) |

|                       文件分享                       |                        密码访问                         |
| :--------------------------------------------------: | :-----------------------------------------------------: |
| ![文件分享](./docs/images/fkteams_web_fileshare.png) | ![密码访问](./docs/images/fkteams_web_fileshare_mm.png) |

|                        分享预览                         |                        批量分享预览                         |
| :-----------------------------------------------------: | :---------------------------------------------------------: |
| ![分享预览](./docs/images/fkteams_web_fileshare_yl.png) | ![批量分享预览](./docs/images/fkteams_web_fileshare_pl.png) |

|                    并行子任务                     |
| :-----------------------------------------------: |
| ![并行子任务](./docs/images/fkteams_cli_task.png) |

|                    非交互模式                    |
| :----------------------------------------------: |
| ![非交互模式](./docs/images/fkteams_cli_fjh.png) |

|                   交互模式                    |
| :-------------------------------------------: |
| ![交互模式](./docs/images/fkteams_cli_jh.png) |

## 功能特性

- **多智能体协作**：按任务自动协同代码、搜索、数据分析、远程运维等专业能力
- **多入口使用**：支持 Web UI、CLI、OpenAI 兼容 API，以及 QQ、Discord、微信等消息通道
- **灵活工作模式**：支持团队模式、深度模式、圆桌会议模式和自定义模式
- **工具与扩展**：内置文件、命令、搜索、文档、表格、SSH 等工具，并支持 MCP、Skills、自定义智能体和工作区 `AGENTS.md`
- **自定义智能体配置**：Web 设置页可创建智能体，并通过搜索、多选和工具说明快速配置内置工具与 MCP 工具
- **长任务体验**：任务可在后台运行，刷新页面或断开连接后仍可回到同一会话继续查看，Web 会恢复上次存在的会话，侧边栏会显示会话状态，运行中后续队列按轮次展示并保持可管理
- **并行成员交互**：子智能体可并行执行并独立等待用户回答，多个成员同时提问时可按问题分别回复
- **多模态与推理展示**：支持文本、图片、音频、视频和文件输入，刷新后保留附件展示，可流式展示推理模型思考过程
- **友好错误提示**：常见模型能力、认证、限流和网络错误会显示可读解释，并保留可展开的技术详情
- **长期记忆与定时任务**：支持跨会话记忆、自然语言定时任务和后台独立执行
- **模型接入**：支持 OpenAI 兼容供应商，并可通过 OAuth 登录 GitHub Copilot

## 安装

一键安装脚本会自动下载最新版本并解压到 `~/.fkteams/bin`（Windows 为 `%USERPROFILE%\.fkteams\bin`），同时将该目录添加到 PATH。

**Linux / macOS**

```bash
curl -fsSL https://raw.githubusercontent.com/wsshow/feikong-teams/main/install.sh | bash
```

**Windows (PowerShell)**

```powershell
powershell -c "irm https://raw.githubusercontent.com/wsshow/feikong-teams/main/install.ps1 | iex"
```

> 如需自定义安装目录，可在执行前设置环境变量 `FKTEAMS_INSTALL_DIR`：
>
> - Linux/macOS：`export FKTEAMS_INSTALL_DIR=/your/path`
> - Windows：`$env:FKTEAMS_INSTALL_DIR = "D:\fkteams"`

也可以直接在 [GitHub Releases](https://github.com/wsshow/feikong-teams/releases) 页面手动下载对应平台的压缩包。

## 快速开始

> **快速体验**：安装完成后，只需要配置模型并运行 `fkteams web` 即可立即体验 Web 界面！

### 1. 配置模型

推荐使用登录向导：

```bash
fkteams login
```

也可以直接指定供应商：

```bash
fkteams login openai
fkteams login deepseek
fkteams login copilot
```

或生成配置文件后手动编辑：

```bash
fkteams generate config
```

编辑 `~/.fkteams/config/config.toml`，填写模型配置：

```toml
[[models]]
name = "default"
provider = "openai"
base_url = "https://api.openai.com/v1"
api_key = "your_api_key_here"
model = "gpt-5"
```

GitHub Copilot 用户也可以从 VS Code 已保存的 token 导入（需要 Copilot 订阅）：

```bash
fkteams login copilot --import
```

常用模型管理命令：

```bash
fkteams model ls                     # 列出已配置的模型
fkteams model rm                     # 交互式选择并移除模型配置
fkteams logout openai                # 退出指定供应商
```

> 完整配置项请参考 [配置指南](./docs/configuration.md)

运行期数据默认保存在 `~/.fkteams/` 下，可通过 `FEIKONG_APP_DIR` 覆盖；常用子目录包括 `workspace`、`sessions`、`scheduler`、`history`、`config`、`log`、`share` 和 `runtime`。

如果需要为所有智能体提供项目约定，可在 `~/.fkteams/workspace/` 放置 `AGENTS.md`（也兼容 `Agents.md`）。系统会在每次模型调用前临时注入该文件内容，支持 `@import` 拆分规则文件，内容不会写入会话历史。

### 2. 运行

```bash
# Web 界面模式（推荐）
fkteams web

# 命令行模式
fkteams

# 纯 API 服务
fkteams serve
```

启动后访问 `http://localhost:23456` 即可使用。

> 更多运行模式和命令行参数请参考 [使用指南](./docs/usage.md)

## 构建与部署

```bash
# 从源码构建
git clone https://github.com/wsshow/feikong-teams.git
cd feikong-teams
make native

# 源码直接运行
go run ./cmd/fkteams web

# 或指定平台 / 构建预设平台
make build t=linux:amd64
make all

# Docker 部署
docker compose up -d
```

> 详细部署配置请参考 [部署指南](./docs/deployment.md)

## 内置智能体

| 智能体        | 说明                                     | 默认启用 |
| ------------- | ---------------------------------------- | -------- |
| `@coder`      | 软件工程师，代码实现、调试、重构和验证   | ✓        |
| `@researcher` | 网络研究员，检索、抓取和交叉验证时效信息 | 配置启用 |
| `@analyst`    | 数据分析师，Excel、Python 和文档数据处理 | 配置启用 |
| `@remote`     | 远程运维专家，SSH 服务器连接和系统管理   | 配置启用 |
| `@generalist` | 通用执行助手，综合命令、文件、搜索等工具 | 配置启用 |

> 通过 `[[custom.agents]]` 定义的[自定义智能体](./docs/custom-agents.md)也会自动注册，在任意模式下可通过 `@` 或 `agent` 子命令使用。

## 架构与安全边界

- Web、CLI、API 和消息通道共用同一套执行引擎，会话、历史、流式输出和运行中输入保持一致。
- 智能体、模型、工具和运行时适配层彼此解耦；应用用例层和运行时内核直接依赖 domain/ports 契约，默认 Eino 运行时只在应用 bootstrap 阶段安装。
- 文件、命令、Git、SSH 等高风险能力会经过工具安全策略和人工确认流程；被拒绝的操作不会被自动重试。
- 任务事件基于 domain 事件协议统一分发并记录到历史中，Web 和 CLI 基于同一事件流展示思考、工具调用、成员执行和最终回复。
- 历史记录由内部文件存储适配器管理，事件协议、展示层和持久化实现保持分层。
- Web 和终端中子智能体的提问会保留在对应成员面板内，回答一个问题不会阻塞或误恢复其他并行成员。
- Hooks 通过内部端口契约和运行时总线扩展模型、工具、事件与回合边界；MCP、Skills 和自定义智能体用于扩展运行期能力。
- 内置工具组通过注册表解析，工具策略元数据统一标记只读、破坏性和审批边界。

## 文档导航

| 文档                                    | 说明                                     |
| --------------------------------------- | ---------------------------------------- |
| [配置指南](./docs/configuration.md)     | 环境变量、config.toml 配置               |
| [使用指南](./docs/usage.md)             | 运行模式、CLI 命令、智能体切换、定时任务 |
| [圆桌会议模式](./docs/roundtable.md)    | 多模型讨论模式的原理和配置               |
| [Skills 指南](./docs/skills.md)         | 技能系统的使用和配置                     |
| [MCP 工具集成](./docs/mcp.md)           | MCP 协议集成和常用服务配置               |
| [自定义智能体](./docs/custom-agents.md) | 创建和配置自定义智能体                   |
| [架构设计](./docs/architecture.md)      | 核心接口、运行时隔离、扩展边界           |
| [聊天通道](./docs/channels.md)          | QQ、Discord、微信等平台接入              |
| [长期记忆](./docs/memory.md)            | 记忆提取、存储、检索机制                 |
| [多模态支持](./docs/multimodal.md)      | 图片、音频、视频等多模态输入             |
| [推理模型支持](./docs/reasoning.md)     | 推理/思考模型的流式输出                  |
| [事件协议](./docs/events.md)            | CLI、Web、Stream、通道共用事件约定       |
| [部署指南](./docs/deployment.md)        | 构建、Docker 部署                        |
| [安全说明](./docs/security.md)          | 安全机制和注意事项                       |
| [API 文档](./docs/api/)                 | HTTP/WebSocket API 接口                  |

## 许可证

本项目采用 MIT 许可证。详见 [LICENSE](LICENSE) 文件。

## 致谢

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - 基于 Elm 架构的终端 UI 框架
- [Pterm](https://github.com/pterm/pterm) - 美观的终端 UI 库
- [Cloudwego Eino](https://github.com/cloudwego/eino) - 强大的 AI 编程框架
- [MCP Go](https://github.com/mark3labs/mcp-go) - Go 语言的 MCP 协议实现
- [Model Context Protocol](https://modelcontextprotocol.io/) - AI 工具集成标准协议

## 相关链接

- [MCP 官方文档](https://modelcontextprotocol.io/)
- [MCP 服务器列表](https://github.com/modelcontextprotocol/servers)
- [Cloudwego Eino 文档](https://github.com/cloudwego/eino)
- [项目 GitHub](https://github.com/wsshow/feikong-teams)
