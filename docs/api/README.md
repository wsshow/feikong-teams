# API 文档

## 通用约定

### 统一响应格式

所有 HTTP API 使用统一的 JSON 响应结构：

```json
{
  "code": 0,
  "message": "success",
  "data": <any>
}
```

| 字段      | 说明                       |
| --------- | -------------------------- |
| `code`    | `0` 成功，`1` 失败         |
| `message` | 描述信息                   |
| `data`    | 业务数据（失败时无此字段） |

### 中间件

#### CORS 跨域

所有请求自动处理跨域：

- `Access-Control-Allow-Origin`：动态镜像请求 `Origin`
- `Access-Control-Allow-Headers: *`
- `Access-Control-Allow-Methods: POST, GET, OPTIONS, DELETE, PUT`
- `Access-Control-Allow-Credentials: true`
- OPTIONS 预检请求直接返回 204

#### 请求体大小限制

请求体上限 **100MB**。超出时返回：

| 状态码 | message                | 说明       |
| ------ | ---------------------- | ---------- |
| 413    | request body too large | 请求体过大 |

### 认证机制

通过配置文件 `config.toml` 控制是否启用：

```toml
[server.auth]
enabled = true
username = "admin"
password = "your_password"
secret = "your_jwt_secret"  # Token 签名密钥（启用时必填）
```

**Token 传递方式**（优先级从高到低）：

1. `Authorization: Bearer <token>` 请求头
2. `?token=<token>` URL 参数
3. `fk_token` Cookie

**免认证路径**：`/login`、`/api/fkteams/login`、`/static/*`

**认证失败行为**：

- API 请求（`/api/*`、`/ws`）→ HTTP 401，`{"code": 1, "message": "未登录或登录已过期"}`
- 页面请求 → 返回登录页面

---

## 接口目录

所有 HTTP API 挂载在 `/api/fkteams` 路径下。

| 分类     | 文档                    | 端点数 |
| -------- | ----------------------- | ------ |
| 通用     | [通用接口](misc.md)     | 4      |
| 聊天     | [聊天接口](chat.md)     | 2      |
| 流式任务 | [流式任务](stream.md)   | 7      |
| 会话     | [会话管理](sessions.md) | 5      |
| 文件     | [文件管理](files.md)    | 7      |
| 预览     | [文件预览](preview.md)  | 5      |
| 记忆     | [长期记忆](memory.md)   | 3      |
| 定时任务 | [定时任务](schedule.md) | 2      |

### 配置与系统管理

| 方法 | 路径                                | 说明                               |
| ---- | ----------------------------------- | ---------------------------------- |
| GET  | `/api/fkteams/config`               | 获取当前配置                       |
| PUT  | `/api/fkteams/config`               | 更新配置                           |
| GET  | `/api/fkteams/config/tools`         | 获取所有可用工具名称列表           |
| GET  | `/api/fkteams/config/template-vars` | 获取模板变量（系统提示词可用变量） |
| GET  | `/api/fkteams/providers`            | 获取已知模型提供者列表             |
| POST | `/api/fkteams/providers/models`     | 查询指定提供者的可用模型           |
| POST | `/api/fkteams/shutdown`             | 优雅关闭服务                       |
| POST | `/api/fkteams/restart`              | 重启服务                           |

### OpenAI 兼容 API

需通过 `[openai_api] api_keys` 配置密钥，使用 `Authorization: Bearer <api_key>` 认证。

| 方法 | 路径                   | 说明                 |
| ---- | ---------------------- | -------------------- |
| GET  | `/v1/models`           | 获取可用模型列表     |
| POST | `/v1/chat/completions` | 聊天补全（支持流式） |

### 页面路由

| 方法 | 路径         | 说明                     |
| ---- | ------------ | ------------------------ |
| GET  | `/`          | 首页                     |
| GET  | `/chat`      | 聊天页面（同首页）       |
| GET  | `/login`     | 登录页面（仅认证启用时） |
| GET  | `/p/:linkId` | 文件预览页面             |
| GET  | `/static/*`  | 静态资源                 |
