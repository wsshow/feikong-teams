# 通用接口

## GET /health

健康检查接口，用于容器编排和负载均衡的健康探测。

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "status": "ok"
  }
}
```

---

## POST /api/fkteams/login

> 仅在认证启用时可用（`FEIKONG_LOGIN_ENABLED=true` 时注册该路由）

用户登录获取 Token。

**请求 Body**：

```json
{
  "username": "string",
  "password": "string"
}
```

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "token": "hex_payload.hex_signature"
  }
}
```

**失败响应**：

| 状态码 | message          | 说明           |
| ------ | ---------------- | -------------- |
| 400    | 请求格式错误     | 请求体解析失败 |
| 401    | 用户名或密码错误 | 凭证不匹配     |

Token 格式为 `hex(payload).hex(hmac-sha256)`，payload 为 `username|expiry(RFC3339)`，有效期 7 天。Token 由客户端自行管理，服务端不设置 Cookie。

---

## GET /api/fkteams/version

获取服务版本信息。

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "version": "0.0.1",
    "buildDate": "2025-01-01 00:00:00"
  }
}
```

---

## GET /api/fkteams/agents

获取所有可用智能体列表。

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "name": "string",
      "description": "string"
    }
  ]
}
```
