# fkteams 文档中心

这里汇总 fkteams 的使用、配置、扩展、接口和架构文档。第一次使用建议先阅读[使用指南](./usage.md)和[配置指南](./configuration.md)。

## 入门

| 文档 | 内容 |
| --- | --- |
| [使用指南](./usage.md) | 运行模式、模型登录、CLI 命令、工作区约定和定时任务 |
| [配置指南](./configuration.md) | 配置文件、模型、智能体、工具、环境变量和数据目录 |
| [部署指南](./deployment.md) | 发行版安装、源码构建、前端开发和 Docker 部署 |

## 协作与扩展

| 文档 | 内容 |
| --- | --- |
| [自定义智能体](./custom-agents.md) | 创建和配置专属智能体 |
| [圆桌会议模式](./roundtable.md) | 多智能体讨论的使用和配置 |
| [Skills 指南](./skills.md) | 安装、创建和管理技能 |
| [MCP 工具集成](./mcp.md) | 接入 MCP 服务和外部工具 |
| [JavaScript 扩展](./javascript.md) | 在 Web 中创建工具和流程 Hooks |
| [聊天通道](./channels.md) | 配置 Discord、QQ 和微信通道 |

## 核心能力

| 文档 | 内容 |
| --- | --- |
| [个人助手](./personal-assistant.md) | 常驻会话、任务跟进与后台结果通知 |
| [助手交互界面](./assistant-ui.md) | 表单、图表和输入框命令 |
| [长期记忆](./memory.md) | 助手主动读写、存储和检索机制 |
| [多模态支持](./multimodal.md) | 图片、音频、视频和文件输入 |
| [推理模型支持](./reasoning.md) | 推理过程的流式输出与展示 |

## API 与集成

命令行集成也可参考仓库附带的 [fkteams 技能](./skills/fkteams/SKILL.md)。

| 文档 | 内容 |
| --- | --- |
| [API 总览](./api/README.md) | HTTP、SSE、WebSocket 和 OpenAI 兼容接口导航 |
| [聊天接口](./api/chat.md) | 同步聊天、WebSocket 与多模态输入 |
| [会话接口](./api/sessions.md) | 会话查询、更新和管理 |
| [流式协议](./api/stream.md) | 实时事件、任务队列和运行中输入 |
| [配置接口](./api/config.md) | Web 配置管理接口 |
| [文件接口](./api/files.md) | 工作区文件管理接口 |
| [会话分享接口](./api/shares.md) | 会话公开链接和访问控制 |
| [技能接口](./api/skills.md) | 技能搜索、安装和管理 |
| [记忆接口](./api/memory.md) | 长期记忆管理接口 |
| [定时任务接口](./api/schedule.md) | 任务调度和执行结果接口 |
| [OpenAI 兼容接口](./api/openai.md) | OpenAI API 兼容调用方式 |
| [文件预览与分享](./api/preview.md) | 文件链接、密码和在线预览 |
| [其他接口](./api/misc.md) | 健康检查、登录、AI 草稿与系统控制 |

## 设计与安全

| 文档 | 内容 |
| --- | --- |
| [架构设计](./architecture.md) | 分层架构、核心接口和扩展边界 |
| [事件协议](./events.md) | CLI、Web、API 和通道共用的事件约定 |
| [存储与升级](./storage.md) | 数据位置、首次升级、备份和恢复 |
| [Web 主题](./web-theme.md) | 前端配色、样式与会话间距约定 |
| [开发指南](../AGENTS.md) | 编码约定、常用命令和验证要求 |
| [安全说明](./security.md) | 工具权限、路径边界和部署安全建议 |

## 界面预览

### Web

![fkteams Web 主界面](./images/fkteams_web_main.png)

### 终端

![fkteams 终端 TUI](./images/fkteams_tui_main.png)
