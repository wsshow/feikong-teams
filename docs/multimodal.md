# 多模态支持

fkteams 支持多模态输入，允许用户在对话中发送文本、图片、音频、视频和文件。

## 支持的内容类型

| 类型           | 说明                                              | 字段                       |
| -------------- | ------------------------------------------------- | -------------------------- |
| `text`         | 文本内容                                          | `text`                     |
| `image_url`    | 图片 URL（支持 `detail` 精度控制: high/low/auto） | `url`, `detail`            |
| `image_base64` | Base64 编码图片                                   | `base64_data`, `mime_type` |
| `audio_url`    | 音频 URL                                          | `url`                      |
| `video_url`    | 视频 URL                                          | `url`                      |
| `file_url`     | 文件 URL                                          | `url`                      |

## 消息格式

通过 WebSocket 发送多模态消息时，使用 `contents` 字段：

```json
{
  "type": "chat",
  "session_id": "default",
  "contents": [
    {"type": "text", "text": "这张图片里有什么？"},
    {"type": "image_url", "url": "https://example.com/cat.jpg", "detail": "high"}
  ]
}
```

HTTP 聊天、流式任务和 WebSocket 均使用同一 `contents` 结构。Base64 图片与供应商托管 `file_id` 的字段见[聊天 API](api/chat.md#多模态输入)。

## 视觉输入配置

在设置 → 模型中编辑模型的“视觉输入”，或在对应 `[[models]]` 配置 `supports_vision`：`true` 允许图片请求，`false` 在请求前拒绝图片，省略时保留供应商适配器的原有行为。能力由用户按实际模型配置，开启后仍需要上游服务支持图片。DeepSeek 显式开启时通过 OpenAI 兼容接口发送图片，避免原生 SDK 提前拒绝。

## 历史记录

会话历史会保留用户消息中的多模态附件，用于 Web UI 刷新后的展示。后续轮次构建模型上下文时，历史附件不会再次发送给模型，而是替换为附件清单和稳定 ID，例如 `history:000000:00:01`，避免不支持图片的模型因历史图片反复报错。

模型需要查看历史附件时，可以调用内置只读能力（无需在智能体工具配置中开启）：

- `session_attachment_list`：列出当前会话历史中的附件。
- `session_attachment_read`：按附件 ID 读取附件元数据、URL 或小体积 data URL。

音频、视频、文件等输入同样取决于上游模型与适配器支持，不能仅凭配置开启。
