# 高级功能

## 长期记忆

fkteams 内置了全局长期记忆模块，能够跨会话自动记住用户的各类信息，在后续对话中自动召回相关记忆，让助手越用越顺手。

### 工作原理

1. **自动提取**：每次对话结束后，系统在后台异步调用 LLM 从对话中提取五类记忆：
   - **用户偏好**（preference）：喜好、厌恶、习惯、风格倾向
   - **个人信息**（fact）：身份、背景、环境、关系等客观事实
   - **经验教训**（lesson）：踩过的坑、需要避免的做法
   - **决策结论**（decision）：确定的方案、选定的方向
   - **认知洞察**（insight）：观点、原则、价值判断
2. **智能去重**：提取的记忆会与已有记忆自动去重（基于摘要包含关系和标签重叠度）
3. **BM25 检索**：用户每次提问时，系统基于 BM25 算法从记忆库中召回最相关的条目
4. **上下文注入**：召回的记忆以结构化格式注入到 Agent 的系统提示词中

### 存储位置

记忆数据持久化在 `{FEIKONG_WORKSPACE_DIR}/memory/index.json`，格式为 JSON，可直接查看和手动编辑。

### 使用说明

在 `.env` 文件中设置以下环境变量来启用长期记忆功能：

```env
FEIKONG_MEMORY_ENABLED = true
```

启用后，CLI 模式和 Web 模式均自动工作，无需额外配置。

## 多模态支持

fkteams 支持多模态输入，允许用户在对话中发送文本、图片、音频、视频和文件。

### 支持的内容类型

| 类型           | 说明                                              | 字段                       |
| -------------- | ------------------------------------------------- | -------------------------- |
| `text`         | 文本内容                                          | `text`                     |
| `image_url`    | 图片 URL（支持 `detail` 精度控制: high/low/auto） | `url`, `detail`            |
| `image_base64` | Base64 编码图片                                   | `base64_data`, `mime_type` |
| `audio_url`    | 音频 URL                                          | `url`                      |
| `video_url`    | 视频 URL                                          | `url`                      |
| `file_url`     | 文件 URL                                          | `url`                      |

### WebSocket 消息格式

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

### HTTP API

HTTP POST `/api/chat` 同样支持 `contents` 字段，格式与 WebSocket 一致。

> **注意**：多模态功能的实际效果取决于所使用的模型是否支持对应的输入类型（如视觉理解需要 GPT-4o、Claude 等多模态模型）。

## 推理模型支持

fkteams 原生支持推理/思考模型（如 DeepSeek-R1、OpenAI o1/o3 等），能够流式展示模型的思考过程。

### 工作方式

- **流式思考输出**：推理模型在生成最终回答前的思考过程，会通过 `reasoning_chunk` 事件实时输出
- **CLI 模式**：思考内容以灰色斜体显示（[思考] ...），与正式回答区分
- **Web 模式**：通过 WebSocket 的 `reasoning_chunk` 事件传递，前端可自行定制展示样式
- **历史记录**：思考内容以 `reasoning` 类型事件记录在对话历史中

### 事件类型

| 事件                                | 说明                             |
| ----------------------------------- | -------------------------------- |
| `reasoning_chunk`                   | 推理/思考过程的流式增量内容      |
| `message`（含 `reasoning_content`） | 非流式完整消息，包含推理内容字段 |

> **注意**：只有支持推理/思考的模型才会产生此类事件，普通模型不受影响。
