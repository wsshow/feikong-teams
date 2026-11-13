# OpenAI 兼容 API

OpenAI 兼容接口挂载在 `/v1`，使用 `[openai_api] api_keys` 配置的独立 API Key。这里只代理模型请求，不执行团队、工具或应用会话用例；需要这些能力请使用[聊天接口](chat.md)。

本页示例是完整的 OpenAI 格式响应，不使用普通 API 的 code/data 包装。

```http
Authorization: Bearer <api_key>
```

认证失败返回 OpenAI 风格错误：

```json
{
  "error": {
    "message": "invalid API key",
    "type": "invalid_api_key"
  }
}
```

如果未配置 `[openai_api] api_keys`，同样返回 401，提示需配置 `[openai_api] api_keys`。

## GET /v1/models

返回当前配置中的模型 ID，格式兼容 OpenAI Models API。

**成功响应**：

```json
{
  "object": "list",
  "data": [
    {
      "id": "main",
      "object": "model",
      "created": 1760000000,
      "owned_by": "fkteams"
    }
  ]
}
```

`id` 对应 `config.toml` 中 `[[models]].id`。

---

## POST /v1/chat/completions

代理请求到配置的模型后端。请求体与 OpenAI Chat Completions 兼容，`model` 字段应填写本地模型 ID；后端会将其替换为该配置中的真实模型名后转发。

**请求示例**：

```json
{
  "model": "main",
  "messages": [
    {"role": "user", "content": "你好"}
  ],
  "stream": true
}
```

**转发规则**：

- 根据本地模型 ID 查找连接配置。
- 若配置未写 `provider`，会根据 `base_url` 和真实模型名自动检测。
- 若未写 `base_url`，会使用对应 provider 的默认 Base URL；Copilot 使用 Copilot 默认地址。
- 非 Copilot provider 会注入模型配置中的 `api_key` 和 `extra_headers`。
- Copilot provider 使用 OAuth HTTP client。
- 支持流式响应，响应体逐块透传。

**安全透传响应头**：

仅透传以下上游响应头：

- `Content-Type`
- `X-Request-Id`
- `X-Ratelimit-Limit-Requests`
- `X-Ratelimit-Limit-Tokens`
- `X-Ratelimit-Remaining-Requests`
- `X-Ratelimit-Remaining-Tokens`
- `X-Ratelimit-Reset-Requests`
- `X-Ratelimit-Reset-Tokens`

**失败响应**：

| 状态码 | type | message |
| ------ | ---- | ------- |
| 400 | `invalid_request_error` | `failed to read request body` |
| 400 | `invalid_request_error` | `invalid JSON` |
| 400 | `invalid_request_error` | `model is required` |
| 404 | `model_not_found` | `model "<name>" not found` |
| 500 | `server_error` | `no base_url configured for model` |
| 500 | `server_error` | `failed to create proxy request` |
| 502 | `upstream_error` | `upstream request failed` |
