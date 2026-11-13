# 技能管理 API

基础路径 `/api/fkteams/skills`，认证与响应包装见 [API 总览](README.md)。技能目录与编写方式见 [Skills 指南](../skills.md)。

## 本地技能

| 接口 | 行为与响应 data |
| --- | --- |
| `GET /api/fkteams/skills` | `{skills: [{slug, name, description}], total}` |
| `POST /api/fkteams/skills` | 创建技能，返回 `{skill: {slug, name, description}, message}` |
| `DELETE /api/fkteams/skills/:slug` | 删除整个技能，返回 `{slug, message}` |

创建请求：

```json
{"slug":"review-helper","name":"代码审查","description":"检查改动和验证结果","content":""}
```

`slug` 由 1–64 个小写字母、数字、`_`、`-` 组成，首位为字母或数字。`name` 省略时使用 slug，`content` 省略时生成基础 `SKILL.md`；同名目录已存在返回 400。

## 市场搜索与安装

`GET /api/fkteams/skills/search` 参数：

| 参数 | 默认与说明 |
| --- | --- |
| `q` | 必填关键词 |
| `page` | 默认 1 |
| `size` | 默认 20，最大 50 |
| `sort` / `order` | 默认 downloads / desc |

返回 `{skills: [...], total, page, size}`。缺少关键词返回 400，无市场提供者返回 503，查询失败返回 500。

`POST /api/fkteams/skills/install`：

```json
{"slug":"video-frames"}
```

使用默认市场安装最新版本，成功返回 `{slug, message: "skill installed"}`。需要指定版本或市场时使用 CLI。

## 技能文件

所有 `path` 都相对于当前技能目录，路径逃逸与符号链接访问会被拒绝。

| 接口 | 参数与响应 |
| --- | --- |
| `GET /api/fkteams/skills/:slug/files` | 可选 query `path`；返回 `{slug, files}` 文件树 |
| `POST /api/fkteams/skills/:slug/files` | Body `{path, content, is_dir}`，创建文件或目录 |
| `GET /api/fkteams/skills/:slug/file` | 必填 query `path`；返回 `{slug, path, content}` |
| `PUT /api/fkteams/skills/:slug/file` | Body `{path, content}`，保存文件 |
| `DELETE /api/fkteams/skills/:slug/file` | 必填 query `path`，删除文件或目录 |

文件写操作成功返回 `{slug, path, message}`。`SKILL.md` 不能单独删除；移除整个技能请使用技能删除接口。读取文件不存在返回 404，写入参数或路径错误返回 400。AI 草稿生成见[通用接口](misc.md#ai-辅助草稿)。
