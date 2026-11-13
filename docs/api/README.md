# API 文档

`fkteams web` 与 `fkteams serve` 提供相同的 API，默认地址为 `http://localhost:23456`；前者同时提供 Web 页面。团队任务使用 `/api/fkteams/*`，模型代理使用 `/v1/*`。

## 通用响应

除 OpenAI 兼容接口、文件下载与 SSE 外，接口使用统一 JSON：

```json
{"code":0,"message":"success","data":{}}
```

失败同时使用相应 HTTP 状态码：

```json
{"code":1,"error_code":"not_found","message":"error message","data":null}
```

`error_code` 可为 `invalid_argument`、`not_found`、`conflict`、`unavailable`、`internal` 等。**各分册的响应示例默认只展示 `data`**；请求示例不加响应包装。

常见状态：400 参数无效、401 未认证、404 资源不存在、409 状态或版本冲突、413 请求过大、429 限流、500 内部错误、503 服务不可用。

## 认证

启用 `[server.auth] enabled = true` 后，普通 API 和 `/ws` 需要登录。调用 `POST /api/fkteams/login` 获取 `HttpOnly` 的 `fk_token` Cookie，或在非浏览器客户端使用 `Authorization: Bearer <token>`。Token 不接受 URL 查询参数。

用户名、密码或 Secret 经配置接口修改后，旧 Token 失效；Web 页面可原地重新登录并恢复任务订阅，后台任务继续执行。登录方式见[通用接口](misc.md#登录与退出)。健康检查同样经过普通认证，启用认证时探针需带凭据。

以下入口不要求普通登录：

- 登录页、登录/退出接口、静态资源和 favicon。
- `/p/*`、`/s/*`、文件预览读取/密码验证及公开会话分享接口；这些入口仍执行分享自身的密码和期限校验。
- `/v1/*`，使用独立的 `[openai_api] api_keys` 和 Bearer Header。

## 接口导航

| 文档 | 内容 |
| --- | --- |
| [聊天](chat.md) | 同步 HTTP、WebSocket、多模态输入 |
| [流式任务](stream.md) | 后台执行、展示快照、SSE、续问/转向队列、审批和提问 |
| [会话](sessions.md) | 会话列表、历史、修改、删除与常驻助手 |
| [文件](files.md) | 浏览、搜索、文本编辑、上传和下载 |
| [文件预览与分享](preview.md) | 文件链接、密码验证、在线预览和撤销 |
| [会话分享](shares.md) | 分享授权与公开对话读取 |
| [长期记忆](memory.md) | 查询、新增、修改、删除和清空 |
| [定时任务](schedule.md) | 创建、改期、取消、删除、结果与通知 |
| [配置与模型](config.md) | 配置读写、工具目录、模型提供者 |
| [技能](skills.md) | 本地创建、市场安装、文件编辑和删除 |
| [通用接口](misc.md) | 健康检查、登录、版本、AI 草稿、脚本试运行与系统控制 |
| [OpenAI 兼容 API](openai.md) | 模型列表与 Chat Completions 代理 |

首次存储升级的维护接口见[存储与升级](../storage.md#cli-与纯-api)。运行事件与展示契约见[事件协议](../events.md)。

## 跨域与请求大小

跨域访问通过 `[server] allow_origins` 配置完整 Origin。默认忽略代理来源头，反向代理部署需配置 `trusted_proxies` 并重启，见[服务与认证](../configuration.md#服务与认证)。

请求体按路由限制：控制请求 16 KiB，小型 JSON 256 KiB，配置和 AI 草稿等 4 MiB，文件编辑 JSON 16 MiB，聊天和模型代理 32 MiB。分片上传请求 65 MiB、普通上传请求 101 MiB，全局上限 128 MiB；文件自身的限制见[文件接口](files.md)。

Web 静态资源 `/assets/*` 使用一年 immutable 缓存，HTML 使用 `no-cache`。
