# 配置与模型 API

## GET /api/fkteams/config

获取当前配置。响应会对敏感字段脱敏：

- `models[].api_key` 永远返回空字符串，并用 `models[].has_api_key` 标识是否已配置。
- `server.auth.password`、`server.auth.secret`、`agents.items[].ssh.password`、`channels.qq.app_secret` 返回 `"***"`。
- `channels.discord.token` 只保留末 4 位。
- `agents.items` 返回合并后的全局智能体目录，包含内置智能体的名称、描述、工具和提示词。

**成功响应**：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "models": [
      {
        "id": "main",
        "name": "主力模型",
        "use_for": ["chat", "agent"],
        "provider": "openai",
        "base_url": "https://api.openai.com/v1",
        "api_key": "",
        "has_api_key": true,
        "model": "gpt-5"
      }
    ]
  }
}
```

---

## PUT /api/fkteams/config

保存完整配置并热重载运行时依赖。

**请求 Body**：完整 `config.Config` JSON。建议先 `GET /api/fkteams/config`，在返回结构上修改后整体提交。

敏感字段合并规则：

| 字段 | 保留旧值条件 |
| ---- | ------------ |
| `models[].api_key` | 提交空字符串时按 `original_id` 或 `id` 还原旧密钥 |
| `server.auth.password` / `server.auth.secret` | 提交 `"***"` 时保留旧值 |
| `agents.items[].ssh.password` | 提交 `"***"` 时保留旧值 |
| `channels.qq.app_secret` | 提交 `"***"` 时保留旧值 |
| `channels.discord.token` | 提交掩码值（包含 `**`）时保留旧值 |

保存后会：

- 保存配置文件。
- 重载智能体注册表。
- 清除 Runner 缓存。
- 清除 MCP 工具缓存。
- 重置消息通道 bridge。
- 尝试重建长期记忆 LLM 客户端。

**成功响应**：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "auth_changed": false
  }
}
```

**失败响应**：

| 状态码 | message | 说明 |
| ------ | ------- | ---- |
| 400 | `invalid config: ...` | 请求体不是合法配置 |
| 400 | `duplicate model id: <id>` | 模型 ID 重复 |
| 400 | `model use_for "<use>" is configured by both <id> and <id>` | 模型用途重复 |
| 500 | `failed to save config: ...` | 保存失败 |

---

## GET /api/fkteams/config/tools

返回所有可配置的工具名称。

```json
{
  "code": 0,
  "message": "success",
  "data": ["file", "command", "search"]
}
```

---

## GET /api/fkteams/config/template-vars

返回系统提示词模板可用变量。

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "name": "os_type",
      "description": "操作系统类型",
      "example": "darwin"
    },
    {
      "name": "os_arch",
      "description": "系统架构",
      "example": "arm64"
    },
    {
      "name": "workspace_dir",
      "description": "工作目录路径",
      "example": "/path/to/workspace"
    }
  ]
}
```

---

## GET /api/fkteams/providers

返回已注册的模型提供者信息。

```json
{
  "code": 0,
  "message": "success",
  "data": []
}
```

具体字段由 `providers.ListProviders()` 返回，通常包含提供者标识、显示名称、默认 Base URL 等信息。

---

## POST /api/fkteams/providers/models

查询指定提供者可用模型列表。

**请求 Body**：

```json
{
  "provider": "openai",
  "base_url": "https://api.openai.com/v1",
  "api_key": "sk-..."
}
```

| 字段 | 类型 | 必填 | 说明 |
| ---- | ---- | ---- | ---- |
| `provider` | string | 是 | 提供者类型 |
| `base_url` | string | 否 | 提供者 Base URL |
| `api_key` | string | 否 | API Key；为空且 `base_url` 匹配已有模型配置时会复用已保存密钥 |

**成功响应**：

```json
{
  "code": 0,
  "message": "success",
  "data": ["gpt-5", "gpt-5-mini"]
}
```

**失败响应**：

| 状态码 | message | 说明 |
| ------ | ------- | ---- |
| 400 | `参数错误: ...` | 请求体解析失败 |
| 400 | `provider 不能为空` | 缺少提供者 |
| 400 | 上游错误详情 | 模型列表查询失败 |
