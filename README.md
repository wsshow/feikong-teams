<div align="center">
  <h1>fkteams 非空小队</h1>
  <p>
    <a href="https://github.com/wsshow/feikong-teams/releases"><img src="https://img.shields.io/github/v/release/wsshow/feikong-teams" alt="Release"></a>
    <a href="./go.mod"><img src="https://img.shields.io/github/go-mod/go-version/wsshow/feikong-teams" alt="Go Version"></a>
    <a href="./LICENSE"><img src="https://img.shields.io/github/license/wsshow/feikong-teams" alt="License"></a>
  </p>
  <p><strong>让专业智能体协作完成复杂任务</strong></p>
  <p>一个可本地运行的开源多智能体协作 AI 助手，面向软件开发、资料研究、数据分析、远程运维和自动化任务，并通过 Web、CLI、API 与消息通道提供一致的长任务体验。</p>
</div>

![fkteams Web 任务执行界面](./docs/images/fkteams_web_task.png)

## 为什么选择 fkteams

- **真正的多智能体协作**：协调者按任务调度代码、研究、分析、远程运维等专业智能体，支持团队、深度和圆桌讨论模式。
- **多入口一致体验**：Web、CLI、OpenAI 兼容 API、Discord、QQ 和微信共享同一套会话与执行能力。
- **面向长任务设计**：支持后台执行、断线恢复、运行中转向与续问队列，并可随时查看成员进度和工具调用。
- **能力扩展灵活**：内置文件、命令、搜索、文档、表格、Git、SSH 等工具，可通过 MCP、Skills、自定义智能体和工作区规则继续扩展。
- **从对话到自动化**：支持多模态输入、长期记忆、定时任务、文件分享和独立智能体执行。
- **本地数据与安全边界**：会话、配置和工作区默认保存在本地；高风险工具支持权限策略、人工确认和执行审计。

![非空小队能力概览](./docs/images/fkteams.png)

## 快速开始

### 1. 安装

Linux / macOS：

```bash
curl -fsSL https://raw.githubusercontent.com/wsshow/feikong-teams/main/install.sh | bash
```

Windows PowerShell：

```powershell
powershell -c "irm https://raw.githubusercontent.com/wsshow/feikong-teams/main/install.ps1 | iex"
```

也可以从 [GitHub Releases](https://github.com/wsshow/feikong-teams/releases) 手动下载。自定义安装目录等说明见[部署指南](./docs/deployment.md#安装发行版)。

### 2. 配置模型

```bash
fkteams login
```

登录向导支持常见 OpenAI 兼容模型服务和 GitHub Copilot。更多登录方式及手动配置方法见[配置指南](./docs/configuration.md)和[使用指南](./docs/usage.md#模型登录与管理)。

### 3. 启动 Web 界面

```bash
fkteams web
```

打开 <http://localhost:23456>，即可创建第一个多智能体任务。

启用 Web 登录认证后，凭据变更或登录过期不会丢失当前页面和后台任务；重新登录后 Web UI 会自动恢复任务事件流。认证配置和部署建议见[配置指南](./docs/configuration.md#服务与认证)。

## 使用方式

| 入口 | 启动方式 | 适用场景 |
| --- | --- | --- |
| Web UI | `fkteams web` | 日常使用、长任务跟踪和可视化管理 |
| CLI / TUI | `fkteams` | 终端工作流、开发与运维 |
| API 服务 | `fkteams serve` | 应用集成和自动化调用 |
| 消息通道 | 配置后启动 Web 服务 | Discord、QQ、微信机器人 |

CLI 也支持直接查询、管道输入、恢复会话和调用指定智能体。完整命令见[使用指南](./docs/usage.md)，接口定义见 [API 文档](./docs/api/README.md)。

## 扩展能力

- 使用 [Skills](./docs/skills.md) 为智能体提供可复用的领域知识和工作流程。
- 通过 [MCP](./docs/mcp.md) 接入外部工具和服务。
- 创建[自定义智能体](./docs/custom-agents.md)，组合专属模型、提示词和工具。
- 使用工作区 `AGENTS.md` 为整个项目提供统一约定。

## 从源码构建

源码构建需要 Go 和 Bun：

```bash
git clone https://github.com/wsshow/feikong-teams.git
cd feikong-teams
make native
```

构建产物位于 `release/`。跨平台构建、前端开发和 Docker 部署见[部署指南](./docs/deployment.md)。

## 文档

从[文档中心](./docs/README.md)查看完整文档，或直接进入常用主题：

| 主题 | 文档 |
| --- | --- |
| 入门与配置 | [使用指南](./docs/usage.md) · [配置指南](./docs/configuration.md) |
| 智能体协作 | [自定义智能体](./docs/custom-agents.md) · [圆桌会议](./docs/roundtable.md) |
| 能力扩展 | [Skills](./docs/skills.md) · [MCP](./docs/mcp.md) · [消息通道](./docs/channels.md) |
| 集成与部署 | [API](./docs/api/README.md) · [部署指南](./docs/deployment.md) |
| 设计与安全 | [架构设计](./docs/architecture.md) · [事件协议](./docs/events.md) · [安全说明](./docs/security.md) |

## 许可证

本项目采用 [MIT License](./LICENSE)。

## 致谢

fkteams 基于 [CloudWeGo Eino](https://github.com/cloudwego/eino)、[Bubble Tea](https://github.com/charmbracelet/bubbletea)、[Pterm](https://github.com/pterm/pterm) 和 [MCP Go](https://github.com/mark3labs/mcp-go) 等优秀开源项目构建。
