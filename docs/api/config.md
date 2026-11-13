# 配置与模型 API

配置字段与 TOML 示例见[配置指南](../configuration.md)，认证与响应包装见 [API 总览](README.md)。

## GET /api/fkteams/config

返回配置 JSON，`agents.items` 包含合并后的内置与自定义角色。敏感字段经过脱敏：

| 字段 | 返回形式 |
| --- | --- |
| `models[].api_key` | 空字符串，`has_api_key` 表示是否已配置 |
| `models[].original_id` | 当前稳定 ID，供修改时保留密钥归属 |
| `models[].extra_headers` | 已配置时为 `"***"` |
| `openai_api.api_keys[]`、`channels.discord.token` | 仅保留末 4 位的掩码 |
| 认证密码、Secret、SSH 密码、QQ app_secret | `"***"` |

## PUT /api/fkteams/config

先 GET，在返回结构上修改，再整体提交完整配置。任务执行或其他配置更新期间返回 409，请等空闲后重试。

| 字段 | 保存时如何保留旧值 |
| --- | --- |
| `models[].api_key` | 空字符串按 original_id 或 id 恢复；不按数组位置、URL 猜测 |
| `models[].extra_headers` | `"***"` 按稳定 ID 保留；空字符串清空 |
| `openai_api.api_keys[]` | 掩码唯一匹配旧值，允许重排或删除 |
| 认证密码、Secret、SSH 密码、QQ app_secret | `"***"` 保留 |
| `channels.discord.token` | 掩码值保留 |

成功后保存配置、重载智能体目录、清理 Runner/MCP 缓存、重置通道 Runner 和记忆开关，返回 `{auth_changed: boolean}`。监听地址、后台服务启停和通道连接变更仍需重启。

配置无效（例如 ID 或模型用途重复、认证字段缺失、代理格式不合法）返回 400，保存失败返回 500。`supports_vision` 的省略、true、false 含义见[多模态支持](../multimodal.md#视觉输入配置)。

## 工具与提示词目录

| 接口 | 响应 data |
| --- | --- |
| `GET /api/fkteams/config/tools` | 可配置工具组名数组，如 `["file", "command", "search"]` |
| `GET /api/fkteams/config/tool-catalog` | 工具组目录，包含 name、display_name、description、category、builtin、included_tools |
| `GET /api/fkteams/config/template-vars` | `{name, description, example}` 数组，包含 os_type、os_arch、workspace_dir |

## 模型提供者

`GET /api/fkteams/providers` 返回提供者信息数组，包括标识、展示名称、默认地址等。

`POST /api/fkteams/providers/models` 查询上游可用模型：

```json
{"provider":"openai","base_url":"https://api.openai.com/v1","model_id":"main","api_key":""}
```

| 字段 | 说明 |
| --- | --- |
| `provider` | 必填提供者类型 |
| `base_url` | 可选服务地址 |
| `api_key` | 可选；留空时按 model_id / original_id 复用保存的密钥 |
| `model_id` / `original_id` | 稳定模型身份，不按 URL 匹配旧密钥 |
| `extra_headers` | 额外请求头，`"***"` 按模型身份恢复旧值 |

成功返回 `[{"id":"model-name"}]` 形式的模型数组。参数或上游查询失败返回 400，未装配模型提供者注册表返回 503。
