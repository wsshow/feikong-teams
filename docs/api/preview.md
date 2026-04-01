# 文件预览

## POST /api/fkteams/preview

创建文件预览链接（支持单文件和多文件，可设置过期时间和访问密码）。

**请求 Body**：

单文件：

```json
{
  "file_path": "docs/report.pdf",
  "password": "abc123",
  "expires_in": 7200
}
```

多文件：

```json
{
  "file_paths": ["docs/report.pdf", "images/logo.png"],
  "password": "abc123",
  "expires_in": 7200
}
```

| 字段         | 类型     | 必填 | 说明                                                       |
| ------------ | -------- | ---- | ---------------------------------------------------------- |
| `file_path`  | string   | 条件 | 单文件相对路径（`file_path` 和 `file_paths` 至少提供一个） |
| `file_paths` | string[] | 条件 | 多文件相对路径数组                                         |
| `password`   | string   | 否   | 访问密码，不设则无需密码                                   |
| `expires_in` | int      | 否   | 过期时间（秒），默认 86400（1天），最长 30 天              |

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": "a1b2c3d4e5f6...",
    "file_path": "docs/report.pdf",
    "expires_at": 1700007200,
    "created_at": 1700000000
  }
}
```

**失败响应**：

| 状态码 | message                      | 说明           |
| ------ | ---------------------------- | -------------- |
| 400    | 参数错误                     | JSON 解析失败  |
| 400    | 无效的文件路径               | 路径遍历       |
| 404    | 文件不存在                   | 路径不可达     |
| 500    | 生成链接失败                 | 随机数生成错误 |

---

## GET /api/fkteams/preview/:linkId

通过预览链接访问/下载文件。

**路径参数**：

| 参数     | 类型   | 说明    |
| -------- | ------ | ------- |
| `linkId` | string | 链接 ID |

**请求参数** (Query/Header)：

| 参数                 | 位置   | 说明                       |
| -------------------- | ------ | -------------------------- |
| `password`           | Query  | 访问密码（设置密码时必填） |
| `X-Preview-Password` | Header | 访问密码（备选方式）       |

**成功响应** (200)：

文件内容直接返回。可浏览器预览的类型（图片/PDF/文本/音视频等）以 `Content-Disposition: inline` 返回，其他类型以 `attachment` 方式下载。多文件时打包为 ZIP 返回。

**失败响应**：

| 状态码 | message              | 说明           |
| ------ | -------------------- | -------------- |
| 400    | 缺少链接 ID          | 路径参数为空   |
| 401    | 需要输入访问密码     | 未提供密码     |
| 403    | 密码错误             | 密码不匹配     |
| 404    | 链接不存在或已失效   | 链接未找到     |
| 404    | 文件不存在或已被删除 | 原文件已被移除 |
| 410    | 链接已过期           | 超过过期时间   |

---

## GET /api/fkteams/preview/:linkId/info

获取预览链接的文件信息（不需要密码验证）。

**路径参数**：

| 参数     | 类型   | 说明    |
| -------- | ------ | ------- |
| `linkId` | string | 链接 ID |

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "file_name": "report.pdf",
    "file_size": 51200,
    "file_count": 1,
    "files": [
      {
        "path": "docs/report.pdf",
        "name": "report.pdf",
        "is_dir": false,
        "size": 51200
      }
    ],
    "content_type": "application/pdf",
    "require_password": false,
    "previewable": true,
    "expires_at": 1700007200,
    "created_at": 1700000000
  }
}
```

| 字段               | 说明                               |
| ------------------ | ---------------------------------- |
| `file_name`        | 首个文件的文件名                   |
| `file_size`        | 文件大小（单文件时有效）           |
| `file_count`       | 共享的文件数量                     |
| `files`            | 文件列表信息                       |
| `content_type`     | 文件 MIME 类型                     |
| `require_password` | 是否需要密码                       |
| `previewable`      | 是否可在线预览（单文件且支持类型） |
| `expires_at`       | 过期时间（Unix 时间戳）            |
| `created_at`       | 创建时间（Unix 时间戳）            |

**失败响应**：

| 状态码 | message            | 说明         |
| ------ | ------------------ | ------------ |
| 400    | 缺少链接 ID        | 参数为空     |
| 404    | 链接不存在或已失效 | 未找到链接   |
| 410    | 链接已过期         | 超过过期时间 |

---

## GET /api/fkteams/preview

列出所有有效的预览链接。

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "id": "a1b2c3d4e5f6...",
      "file_path": "docs/report.pdf",
      "expires_at": 1700007200,
      "created_at": 1700000000
    }
  ]
}
```

已过期的链接会自动清理，不在列表中返回。

---

## DELETE /api/fkteams/preview/:linkId

手动删除（撤销）预览链接。

**路径参数**：

| 参数     | 类型   | 说明    |
| -------- | ------ | ------- |
| `linkId` | string | 链接 ID |

**成功响应** (200)：

```json
{
  "code": 0,
  "message": "success",
  "data": null
}
```

**失败响应**：

| 状态码 | message     | 说明       |
| ------ | ----------- | ---------- |
| 400    | 缺少链接 ID | 参数为空   |
| 404    | 链接不存在  | 未找到链接 |
