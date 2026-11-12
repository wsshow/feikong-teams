# 助手交互界面

Web 常驻助手（`/assistant`）支持在消息中生成卡片和表单。可以直接要求“用表单收集每周总结任务的时间和范围”或“把方案整理成可选择的卡片”。普通回答仍使用 Markdown；是否生成界面取决于模型对任务的判断。

## 使用方式

1. 助手生成卡片，必要时包含单行输入、多行输入或下拉单选。
2. 生成期间控件不可提交。生成结束后填写内容，点击提交或卡片按钮。
3. 操作作为下一条用户消息交给同一个助手；已有任务运行时进入 follow-up 队列。
4. 助手继续调用原有工具处理请求，危险操作仍经过独立的审批流程。提交表单不表示操作已完成或已获审批。

已接受的操作显示为只读，填写结果保存在用户消息中，重新打开会话可以恢复。入队操作可以通过原有队列删除、排序或改为文字消息；删除后原卡片可以再次提交。网络请求失败时当前表单保留填写内容，可重试。

尚未提交的草稿仅保留在当前挂载的界面中，刷新或离开会话后不保证保留。首版不提供跨标签页的操作去重；提交结果不明时，应先检查会话或队列再重试。

仅常驻助手的消息启用交互渲染。普通聊天、分享页、CLI 和消息通道按原有文本方式展示；助手需要在界面之外保留简短说明，用户也可以直接用文字回复。

## 实现约定

- 使用固定版本的 `@openuidev/react-lang`，通过项目已有控件定义 `Card`、`Text`、`Input`、`Textarea`、`Select`、`Form`、`Button`。没有新增模型服务或 Node 后端。
- 每个 `openui-lang` Markdown 围栏包含一个独立的完整 `Card` 程序。渲染器按消息分段的稳定标识与块序号识别来源；普通代码块中的示例不会被执行。
- UI 文本继续通过现有消息 `Content` 流传输和持久化，不增加第二份流式内容或改写历史事实。提交消息使用 `【助手界面操作】` 前缀及版本化 JSON，包含来源、请求和字段值；Web 将其展示为可读文本。
- 输入框和界面操作共用 `sendChatMessage`，复用 `/api/fkteams/stream/start`、任务订阅、队列接纳及失败回滚。
- 仅接受继续对话操作，不接入 `toolProvider`、浏览器 MCP、Query/Mutation、任意 HTML、脚本和链接动作。未知或错误程序提供文字回复的降级提示；组件值按文本渲染。
- 流式未完成、程序不完整、提交中、会话已切换或已有对应提交时禁止交互。UI 操作是用户输入，不是工具审批凭证。
- 界面代码按需加载。字段值不应包含密码、API key 或其他凭据。

## 修改组件与提示词

组件和示例在 `web/src/features/chat/openui/library.tsx` 定义。修改后运行：

```bash
cd web
bun run generate:assistant-ui
bun test src/features/chat/openui
```

脚本从实际组件库生成 `internal/app/agent/catalog/workspaceassistant/openui_prompt.txt`，由 Go 嵌入助手指令。生成文本随源代码维护，前端构建会重新生成；测试检查生成文本与组件定义一致。这样直接构建 Go 时也有可用的提示词。

完整验证使用 `make check` 与 `cd web && bun test src`。Go 的 `TestAssistantUIRoundTripPreservesHistoryAndUsesExistingTools` 覆盖跨回合、历史重新加载及原有工具调用；前端测试覆盖混排、流式禁用、提交、失败回滚、队列、历史恢复和渲染边界。

上游参考：[OpenUI 仓库](https://github.com/thesysdev/openui)、[交互接口](https://www.openui.com/docs/openui-lang/interactivity)。
