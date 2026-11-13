# 通用接口

认证与响应包装见 [API 总览](README.md)。

## 健康检查

| 接口 | 说明 |
| --- | --- |
| `GET /health` | 兼容存活检查，返回 `{status: "ok"}` |
| `GET /live` | 进程存活检查，同 health |
| `GET /ready` | 核心 Runtime 自检，成功包含 status: ready 与自检信息，未就绪返回 503 |

健康路由同样经过登录认证。启用认证时探针应携带 Cookie 或 Bearer Token，否则会被重定向到登录页。

## 登录与退出

`POST /api/fkteams/login`：

```json
{"username":"admin","password":"your_password","cookie_only":true}
```

未启用认证时返回 404。成功设置有效期 7 天的 `HttpOnly`、`SameSite=Strict` `fk_token` Cookie，HTTPS 下附带 Secure。

- `cookie_only: true` 返回 `{authenticated: true}`，供 Web 使用。
- 省略或为 false 时返回 `{token: "..."}`，供非浏览器客户端使用 Bearer Header。

凭据错误返回 401，过多尝试返回 429 和 Retry-After。`POST /api/fkteams/logout` 清除 Cookie，即使 Token 已失效仍可调用。

## 基础查询

| 接口 | 响应 |
| --- | --- |
| `GET /api/fkteams/version` | `{version, buildDate}` |
| `GET /api/fkteams/agents` | 智能体数组，字段包括 name、description、可选 aliases |
| `GET /api/fkteams/favicon` | query 传 domain 或 url，可选 size；返回图标二进制，上游不可用时使用 fallback SVG |

## AI 辅助草稿

以下 POST 接口使用默认对话模型，超时 60 秒。响应为待审查草稿，生成后不会直接写配置或安装技能。

| 接口 | 请求字段 | 响应 data |
| --- | --- | --- |
| `/api/fkteams/ai/agents/draft` | instruction；可选 existing_agents、available_tools、available_models、default_model_id | `{agents: [...]}` |
| `/api/fkteams/ai/skills/draft` | instruction；可选 existing_skills | `{skill: {slug, name, description, content}}` |
| `/api/fkteams/ai/javascript/draft` | kind: tool/hook、instruction；可选 existing_ids、current_tool 或 current_hook | `{kind, tool}` 或 `{kind, hook}` |
| `/api/fkteams/ai/text/rewrite` | instruction、text；可选 scenario、context | `{text}` |

JavaScript 草稿在返回前校验语法和入口，默认不启用。工具与角色字段见[配置接口](config.md)和 [JavaScript 扩展](../javascript.md)。请求或生成失败返回 400，客户端主动断开可返回 499。

## POST /api/fkteams/javascript/test

试运行尚未保存的 JavaScript 工具：

```json
{
  "tool": {
    "id": "text_count",
    "name": "字符统计",
    "description": "统计输入长度",
    "read_only": true,
    "parameters": {"type":"object","properties":{"text":{"type":"string"}}},
    "source": "function execute(input) { return String(input.text || '').length; }"
  },
  "input": {"text":"示例"}
}
```

返回 `{result, raw, notices: [{level, message}]}`。试运行会真实访问宿主工具和持久化状态，仍执行工具策略与审批；不是无副作用的语法检查。校验或执行失败返回 400，工具注册表不可用返回 503。

## 系统控制

| 接口 | 行为与响应 data |
| --- | --- |
| `POST /api/fkteams/shutdown` | 请求优雅关闭，返回 `{message: "server is shutting down"}` |
| `POST /api/fkteams/restart` | 请求优雅重启，返回 `{message: "server is restarting"}` |

生命周期控制器不可用时返回 500。存储维护接口见[存储与升级](../storage.md#cli-与纯-api)。
