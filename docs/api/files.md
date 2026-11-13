# 文件管理 API

所有路径相对于应用 `workspace/`，拒绝路径逃逸与符号链接绕过。认证与响应包装见 [API 总览](README.md)。文件信息使用：

```json
{"name":"report.txt","path":"docs/report.txt","is_dir":false,"size":1234,"mod_time":1700000000}
```

`size` 为字节，`mod_time` 为 Unix 秒。

## 浏览与搜索

| 接口 | 参数与返回 |
| --- | --- |
| `GET /api/fkteams/files` | 可选 query `path`，空为根目录；返回文件信息数组 |
| `GET /api/fkteams/files/search` | 必填 query `q`，按文件名递归搜索，不区分大小写；返回同结构数组 |

列表隐藏点号开头的条目，文件夹在前，同类型按修改时间倒序、同时间按名称排序。单目录最多 10000 项。搜索最多返回 100 条、深度最多 10 层，同类型按名称排序；遍历超限返回错误。

## 文本编辑

`GET /api/fkteams/files/content?path=docs/report.txt`：

```json
{"path":"docs/report.txt","name":"report.txt","content":"正文","size":6,"mod_time":1700000000}
```

`PUT /api/fkteams/files/content` 保存已有文件：

```json
{"path":"docs/report.txt","content":"更新后的正文"}
```

成功返回 path、name、size、mod_time。仅支持不超过 2 MiB 的 UTF-8 普通文件；内容为空表示清空文件。目标类型或路径无效返回 400，保存前文件被替换返回 409，IO 失败返回 500。

## 下载与内联访问

| 接口 | 行为 |
| --- | --- |
| `GET /api/fkteams/files/download?path=docs/report.txt` | 文件作为 attachment 返回，目录流式打包 ZIP |
| `POST /api/fkteams/files/download/batch` | Body `{paths: ["docs/report.txt", "images/"]}`，返回 download.zip |
| `GET /api/fkteams/files/serve/*filepath` | inline 返回文件；目录尝试 index.html，支持相对资源引用 |

ZIP 过滤隐藏文件，路径校验拒绝符号链接访问。批量最多 128 个路径，归档最多 10000 项、未压缩内容 2 GiB，并发归档超限时拒绝请求。参数错误返回 400，不存在返回 404。公开文件链接使用[预览与分享 API](preview.md)。

## POST /api/fkteams/files/upload

使用 `multipart/form-data`：

| 字段 | 说明 |
| --- | --- |
| `file` | 必填；多个同名字段可上传多个文件 |
| `path` | 可选目标子目录，空为工作区根目录 |

最多 32 个文件，每个不超过 100 MiB，请求总大小不超过 101 MiB。文件名重复或不安全返回 400，超过体积限制返回 413。成功返回文件信息数组；逐文件原子保存，不保证整个批次一起回滚。

## POST /api/fkteams/files/upload/chunk

大文件使用 `multipart/form-data` 分片上传：

| 字段 | 说明 |
| --- | --- |
| `file` | 分片内容 |
| `uploadId` | 客户端生成的稳定上传标识，最多 128 字节 |
| `chunkIndex` | 从 0 开始的分片序号 |
| `totalChunks` | 总分片数，最多 1024 |
| `fileName` | 最终文件名 |
| `path` | 可选目标子目录 |

每片最多 64 MiB，单文件合计最多 4 GiB；临时上传保留 1 小时，并受总容量和并发配额限制。除 path 外字段均必填，同一次上传应保持目标及分片总数一致。

进行中返回：

```json
{"uploadId":"upload-1","chunkIndex":0,"received":1,"total":3,"completed":false}
```

全部分片到齐后合并，返回 `{uploadId, completed: true, file: {...}}`；file 为文件信息。目标在合并完成后原子替换。非法序号、文件名或路径返回 400，上传元数据冲突返回 409，配额不足返回错误。

## DELETE /api/fkteams/files

```json
{"path":"docs/report.txt","force":false}
```

文件或空目录可直接删除；非空目录要求 `force: true`。成功的 data 为 null。不能删除工作区根目录；缺少路径、路径无效或非空目录未确认返回 400，不存在返回 404。
