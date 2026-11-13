# 文件预览与分享 API

通过文件链接公开预览或下载工作区文件。管理操作使用普通登录认证，访问链接使用独立密码与期限校验。响应包装见 [API 总览](README.md)。

## POST /api/fkteams/preview

创建单文件或多文件链接：

```json
{"file_paths":["docs/report.pdf","images/logo.png"],"password":"example-password","expires_in":7200}
```

| 字段 | 说明 |
| --- | --- |
| `file_path` / `file_paths` | 单路径或路径数组，至少一个；均相对于工作区 |
| `password` | 可选访问密码 |
| `expires_in` | 秒，默认 1 天、最长 30 天，负数表示永不过期 |

成功返回：

```json
{"id":"preview-1","file_path":"docs/report.pdf","file_paths":["docs/report.pdf","images/logo.png"],"expires_at":1760007200,"created_at":1760000000}
```

浏览器访问 `/p/:linkId`。路径无效返回 400，文件不存在返回 404，创建失败返回 500。

## 链接管理

| 接口 | 行为 |
| --- | --- |
| `GET /api/fkteams/preview` | 返回有效链接数组，字段同创建响应；过期条目会清理 |
| `DELETE /api/fkteams/preview/:linkId` | 撤销链接，成功 data 为 null，不存在返回 404 |

## 信息与密码验证

`GET /api/fkteams/preview/:linkId/info` 无需普通登录或密码，返回预览所需信息：

```json
{
  "file_name": "report.pdf",
  "file_size": 51200,
  "file_count": 1,
  "files": [{"path":"docs/report.pdf","name":"report.pdf","is_dir":false,"size":51200}],
  "content_type": "application/pdf",
  "require_password": false,
  "authorized": true,
  "previewable": true,
  "expires_at": 1760007200
}
```

密码保护的链接在授权前只返回基本信息，省略完整路径、文件清单和大小。

`POST /api/fkteams/preview/:linkId/auth` 验证密码：

```json
{"password":"example-password"}
```

成功返回 `{authenticated: true}` 并设置有效期一小时的 `HttpOnly`、`SameSite=Strict` `fk_preview_{linkId}` Cookie，HTTPS 下附带 Secure。凭据绑定链接、密码哈希与期限；链接失效或凭据过期后不能继续访问。

非浏览器客户端也可在文件请求中发送 `X-Preview-Password` Header。密码不接受 query 参数。错误密码或缺少密码返回 401，尝试过多返回 429 和 Retry-After。

## 读取与渲染

| 接口 | 行为 |
| --- | --- |
| `GET /api/fkteams/preview/:linkId` | 单文件直接返回，图片/PDF/文本/音视频等 inline，其他 attachment；多文件返回 ZIP |
| `GET /api/fkteams/preview/:linkId/render/*filepath` | inline 返回主文件或其相对资源；空路径重定向至主文件名 |

浏览器自动携带预览 Cookie，包括 iframe 内相对资源请求。资源访问受分享目录和工作区边界限制，不能通过相对路径读取目录外文件。

这组公开读取接口的常见失败：401 需密码、403 路径禁止、404 链接或文件不存在、410 已过期、429 密码尝试限流。无密码链接的持有者可以直接读取，请在分享前检查内容。
