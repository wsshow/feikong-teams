# API 文档

本文档对应当前后端路由实现。Web 模式下包含页面、静态资源、HTTP API、WebSocket、SSE 和 OpenAI 兼容 API；纯 API 模式只注册 API 路由。

## 通用约定

除 `/v1/*` OpenAI 兼容接口和文件/流式下载接口外，HTTP API 使用统一 JSON 响应：

```json
{
  "code": 0,
  "message": "success",
  "data": {}
}
```

失败响应：

```json
{
  "code": 1,
  "error_code": "not_found",
  "message": "error message"
}
```

| 字段 | 说明 |
| ---- | ---- |
| `code` | `0` 成功，`1` 失败 |
| `error_code` | 稳定错误码，例如 `invalid_argument`、`not_found`、`conflict`、`unavailable`、`internal` |
| `message` | 响应描述 |
| `data` | 业务数据，失败时通常省略 |

## 认证

### Web/API 登录认证

当配置 `[server.auth] enabled = true` 时，大多数 `/api/fkteams/*` 和 `/ws` 需要登录 Token。

认证中间件按请求读取热更新配置。修改用户名、密码或 Secret 会使已有 Token 失效；浏览器页面请求会重定向到 `/login?next=<原地址>`，API 请求返回 `401`。

免普通登录认证路径：

- `/login`
- `/favicon.ico`
- `/api/fkteams/login`
- `/api/fkteams/favicon`
- `/assets/*`
- `/p/*`、`/s/*`
- `GET` / `HEAD` `/api/fkteams/preview/*`
- `/api/fkteams/public/session-shares/*`
- `/v1/*`，使用独立 API Key

Token 传递方式：

| 方式 | 示例 |
| ---- | ---- |
| Header | `Authorization: Bearer <token>` |
| Query | `?token=<token>` |
| Cookie | `fk_token=<token>` |

登录接口见 [通用接口](misc.md)。

### OpenAI 兼容 API Key

`/v1/*` 使用独立 API Key 认证，配置来源为 `[openai_api] api_keys`。请求必须携带：

```http
Authorization: Bearer <api_key>
```

详见 [OpenAI 兼容 API](openai.md)。

## 中间件行为

| 能力 | 行为 |
| ---- | ---- |
| CORS | 默认允许同源和本机开发来源；跨域部署需配置 `[server] allow_origins` |
| Body Limit | 请求体上限 100MB，超出返回 413 `request body too large` |
| 静态资源缓存 | Vite 构建产物 `/assets/*` 返回 `public, max-age=31536000, immutable`；HTML 返回 `no-cache` |

## API 分册

| 文档 | 覆盖范围 |
| ---- | -------- |
| [通用接口](misc.md) | 健康检查、登录、版本、智能体、favicon、系统控制 |
| [聊天接口](chat.md) | HTTP 聊天、WebSocket 协议、事件结构 |
| [流式任务](stream.md) | 后台任务、SSE 订阅、队列、HITL 审批和提问 |
| [会话管理](sessions.md) | 会话列表、创建、加载、删除、重命名、当前智能体 |
| [文件管理](files.md) | 工作区文件列表、搜索、上传、分片、下载、删除、内联访问 |
| [文件预览](preview.md) | 文件分享链接、预览、渲染、撤销 |
| [会话分享](shares.md) | 会话分享链接、公开访问、密码访问 |
| [长期记忆](memory.md) | 记忆列表、删除、清空 |
| [定时任务](schedule.md) | 调度任务列表、取消、结果、历史 |
| [配置与模型](config.md) | 配置读写、工具名、模板变量、模型提供者 |
| [技能管理](skills.md) | 已安装技能、市场搜索、安装、删除、文件浏览 |
| [OpenAI 兼容 API](openai.md) | `/v1/models`、`/v1/chat/completions` |

## 路由总表

### 根路由与页面

| 方法 | 路径 | 说明 |
| ---- | ---- | ---- |
| GET | `/` | Web 首页 |
| GET | `/chat` | Web 首页别名 |
| GET | `/login` | 登录页 |
| GET | `/p/:linkId` | 文件预览页 |
| GET | `/s/:shareID` | 会话分享页 |
| GET | `/assets/*filepath` | 静态资源 |
| GET | `/favicon.ico` | favicon |

### 公共与系统

| 方法 | 路径 | 说明 |
| ---- | ---- | ---- |
| GET | `/health` | 兼容健康检查（等同存活检查） |
| GET | `/live` | 进程存活检查 |
| GET | `/ready` | 核心 Runtime 就绪检查 |
| GET | `/ws` | WebSocket 聊天和任务事件通道 |
| POST | `/api/fkteams/login` | 登录获取 Token；未启用认证时返回 `404` |
| GET | `/api/fkteams/version` | 版本信息 |
| GET | `/api/fkteams/agents` | 可用智能体列表 |
| GET | `/api/fkteams/favicon` | favicon 代理 |
| POST | `/api/fkteams/shutdown` | 优雅关闭 |
| POST | `/api/fkteams/restart` | 优雅重启 |

### 聊天与流式任务

| 方法 | 路径 | 说明 |
| ---- | ---- | ---- |
| POST | `/api/fkteams/chat` | HTTP 同步聊天 |
| POST | `/api/fkteams/stream/start` | 启动后台流式任务；运行中则追加 follow-up |
| POST | `/api/fkteams/stream/steer` | 向运行中任务追加 steering |
| GET | `/api/fkteams/stream/queue/:sessionID` | 查询未消费队列 |
| PATCH | `/api/fkteams/stream/queue/:sessionID/:queueID` | 修改队列项 |
| DELETE | `/api/fkteams/stream/queue/:sessionID/:queueID` | 删除队列项 |
| POST | `/api/fkteams/stream/queue/:sessionID/:queueID/kind` | 切换队列项类型 |
| POST | `/api/fkteams/stream/queue/:sessionID/:queueID/move` | 调整同类队列项顺序 |
| POST | `/api/fkteams/stream/stop/:sessionID` | 请求停止运行中任务 |
| GET | `/api/fkteams/stream/subscribe/:sessionID` | SSE 订阅事件流 |
| GET | `/api/fkteams/stream/status/:sessionID` | 查询任务或会话状态 |
| GET | `/api/fkteams/stream/events/:sessionID` | 拉取已缓冲事件 |
| POST | `/api/fkteams/stream/approval` | 提交 HITL 审批决定 |
| POST | `/api/fkteams/stream/ask-response` | 提交 ask_questions 回答 |

### 会话、文件、分享

| 方法 | 路径 | 说明 |
| ---- | ---- | ---- |
| GET | `/api/fkteams/sessions` | 会话列表 |
| POST | `/api/fkteams/sessions` | 创建会话 |
| GET | `/api/fkteams/sessions/:sessionID` | 加载会话历史 |
| PATCH | `/api/fkteams/sessions/:sessionID` | 更新会话标题、收藏状态或当前智能体 |
| DELETE | `/api/fkteams/sessions/:sessionID` | 删除会话 |
| POST | `/api/fkteams/sessions/rename` | 重命名会话 |
| POST | `/api/fkteams/sessions/agent` | 更新会话当前智能体 |
| GET | `/api/fkteams/files` | 文件列表 |
| GET | `/api/fkteams/files/search` | 文件搜索 |
| GET | `/api/fkteams/files/download` | 下载文件或目录 ZIP |
| POST | `/api/fkteams/files/download/batch` | 批量 ZIP 下载 |
| POST | `/api/fkteams/files/upload` | 上传文件 |
| POST | `/api/fkteams/files/upload/chunk` | 分片上传 |
| DELETE | `/api/fkteams/files` | 删除文件或目录 |
| GET | `/api/fkteams/files/serve/*filepath` | 内联访问工作区文件 |
| POST | `/api/fkteams/preview` | 创建文件预览链接 |
| GET | `/api/fkteams/preview` | 预览链接列表 |
| GET | `/api/fkteams/preview/:linkId` | 访问预览链接 |
| GET | `/api/fkteams/preview/:linkId/info` | 预览链接信息 |
| GET | `/api/fkteams/preview/:linkId/render/*filepath` | 渲染预览资源 |
| DELETE | `/api/fkteams/preview/:linkId` | 删除预览链接 |
| POST | `/api/fkteams/session-shares` | 创建会话分享 |
| GET | `/api/fkteams/session-shares` | 会话分享列表 |
| DELETE | `/api/fkteams/session-shares/:shareID` | 删除会话分享 |
| GET | `/api/fkteams/public/session-shares/:shareID/info` | 公开会话分享信息 |
| POST | `/api/fkteams/public/session-shares/:shareID/access` | 访问公开会话分享内容 |

### 配置、模型、技能、记忆、调度

| 方法 | 路径 | 说明 |
| ---- | ---- | ---- |
| GET | `/api/fkteams/config` | 获取脱敏配置 |
| PUT | `/api/fkteams/config` | 保存配置并热重载 |
| GET | `/api/fkteams/config/tools` | 可用工具名列表 |
| GET | `/api/fkteams/config/template-vars` | 系统提示词模板变量 |
| GET | `/api/fkteams/providers` | 已注册模型提供者 |
| POST | `/api/fkteams/providers/models` | 查询提供者模型列表 |
| GET | `/api/fkteams/skills` | 已安装技能 |
| GET | `/api/fkteams/skills/search` | 搜索技能市场 |
| POST | `/api/fkteams/skills/install` | 安装技能 |
| DELETE | `/api/fkteams/skills/:slug` | 删除技能 |
| GET | `/api/fkteams/skills/:slug/files` | 技能文件树 |
| GET | `/api/fkteams/skills/:slug/file` | 技能文件内容 |
| GET | `/api/fkteams/memory` | 长期记忆列表 |
| DELETE | `/api/fkteams/memory` | 删除指定记忆 |
| POST | `/api/fkteams/memory/clear` | 清空长期记忆 |
| GET | `/api/fkteams/schedules` | 定时任务列表 |
| POST | `/api/fkteams/schedules/:id/cancel` | 取消定时任务 |
| GET | `/api/fkteams/schedules/:id/result` | 最新执行结果 |
| GET | `/api/fkteams/schedules/:id/history` | 历史结果列表 |
| GET | `/api/fkteams/schedules/:id/history/:filename` | 历史结果内容 |

### OpenAI 兼容

| 方法 | 路径 | 说明 |
| ---- | ---- | ---- |
| GET | `/v1/models` | OpenAI 格式模型列表 |
| POST | `/v1/chat/completions` | 代理到配置的模型后端 |
