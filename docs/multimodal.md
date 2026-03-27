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

## WebSocket 消息格式

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

也可以使用 Base64 编码的图片：

```json
{
  "type": "chat",
  "session_id": "default",
  "contents": [
    {"type": "text", "text": "描述这张图片"},
    {"type": "image_base64", "base64_data": "...", "mime_type": "image/png"}
  ]
}
```

## HTTP API

HTTP POST `/api/chat` 同样支持 `contents` 字段，格式与 WebSocket 一致。

> **注意**：多模态功能的实际效果取决于所使用的模型是否支持对应的输入类型（如视觉理解需要 GPT-4o、Claude 等多模态模型）。
