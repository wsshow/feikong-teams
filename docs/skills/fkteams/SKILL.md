---
name: fkteams
description: 使用 fkteams CLI 执行任务、切换智能体、恢复会话，以及管理模型、技能和配置。
compatibility: 需要已安装 fkteams；执行对话前需配置可用模型。
metadata:
  author: fkteams
  version: "1.1"
---

# fkteams 命令行指南

先用 `fkteams --help` 或 `fkteams <子命令> --help` 核对当前二进制支持的选项。根据用户目标执行必要操作；安装技能、修改配置或自动批准工具之前，确认这些操作属于用户请求范围。

## 启动与查询

```bash
fkteams login                  # 交互配置模型，无需先生成配置文件
fkteams web                    # 浏览器访问 http://localhost:23456
fkteams serve                  # 仅 API 服务
fkteams                        # 终端交互，默认 team
fkteams -m deep                 # 深度模式
fkteams -m group                # 圆桌讨论
fkteams -q "整理工作区资料"    # 查询后退出，默认保存会话
fkteams -q "临时问答" --temporary
```

`web` 与 `serve` 提供相同的应用 API；`/v1/chat/completions` 只代理模型，不运行团队或工具。

| 参数 | 作用 |
| --- | --- |
| `-m` / `--mode` | team、deep、group |
| `-q` / `--query` | 直接查询，完成后退出 |
| `-r` / `--resume` | 恢复会话 ID，可与 -q 组合 |
| `--temporary` / `--temp` | 不保存会话 |
| `--approve` | 自动批准 all、command、file、git、dispatch，逗号分隔 |

未配置审批的非交互执行默认拒绝高风险操作。自动批准不绕过路径与命令安全检查。

## 单智能体与管道

```bash
fkteams agent list
fkteams agent -n coder
fkteams agent -n fkteams_helper -q "如何配置 MCP？"
cat error.log | fkteams agent -n coder -q "分析日志"
git diff HEAD~1 | fkteams -q "审查改动"
fkteams agent -n researcher -q "检索相关资料" --format json
```

内置角色包括 coordinator、coder、researcher、fkteams_helper、analyst、remote、generalist；实际可用列表以 `agent list` 为准。自定义角色在 `[[agents.items]]` 中配置。

`agent` 支持 `-n/--name`、`-q/--query`、`--format default|json`、`--temporary`、`--approve`。管道自动进入非交互模式；有 -q 时将其作为指令前缀，空管道且无查询时报错。

## 会话与终端交互

```bash
fkteams session list
fkteams -r "<session-id>"
fkteams -r "<session-id>" -q "继续上次的分析"
```

会话默认保存到应用 SQLite，ID 从列表读取，不假定为时间戳或文件名。交互命令：

| 命令 | 作用 |
| --- | --- |
| `help`、`quit` / `q` | 帮助、退出 |
| `list_agents`、`@角色ID [问题]` | 查看、切换角色 |
| `switch_work_mode` | 切换工作模式 |
| `list_chat_history`、`load_chat_history`、`save_chat_history`、`clear_chat_history` | 管理当前会话与历史 |
| `save_chat_history_to_markdown`、`save_chat_history_to_html` | 导出 |
| `list_schedule`、`cancel_schedule`、`delete_schedule` | 查看、取消、删除调度 |
| `list_memory`、`delete_memory`、`clear_memory` | 管理长期记忆 |

终端运行中按 Enter 提交转向，下一次模型调用前消费；Esc 暂停并将未消费转向回填输入框。Web 运行中普通输入默认作为续问排队。

## 模型与认证

```bash
fkteams login openai
fkteams login copilot           # 设备码登录，需有效订阅
fkteams login copilot --import  # 导入 VS Code 凭据
fkteams model ls
fkteams model lr --name main
fkteams model sw --name main
fkteams model rm --name old-model
fkteams logout openai
fkteams auth                    # 交互管理 Web 登录认证
```

登录向导写入 `config/config.toml`。优先使用交互方式输入密钥，避免写入命令记录。模型配置必须有稳定 `id`，默认对话模型应包含 `use_for = ["chat"]`；智能体以 `model_id` 引用。Web 登录与 OpenAI 代理 API Key 分开配置。

## 技能与运行环境

```bash
fkteams skill list
fkteams skill search ffmpeg --page 1 --size 10
fkteams skill install video-frames --version 1.0.0
fkteams skill remove video-frames
fkteams init --all              # uv 与 Bun 工具环境
fkteams init --env uv
fkteams init --env bun
fkteams init --mirror           # 生成镜像源配置
fkteams tool list
fkteams update
```

技能位于 `skills/<slug>/SKILL.md`，安装同名技能会覆盖。`skill search`、`skill install` 可用 --provider 选择市场。

## 配置与数据

`fkteams generate config` 生成供手动编辑的示例，并非首次使用的必需步骤。`fkteams generate apikey` 只生成密钥，需自行写入 `[openai_api] api_keys`；未配置时仅 `/v1/*` 返回 401。

| 环境变量 | 用途与默认值 |
| --- | --- |
| `FEIKONG_APP_DIR` | 应用目录，默认 ~/.fkteams |
| `FEIKONG_PROXY_URL` | 网络代理 |
| `FEIKONG_MAX_ITERATIONS` | 智能体迭代上限，默认 60，0/-1 不限制 |

配置为 `config/config.toml`，业务数据库为 `data/fkteams.db`，默认工作区为 `workspace/`，独立于启动目录。备份时完全退出应用后复制整个应用目录，保留数据库、附件与工作区。
