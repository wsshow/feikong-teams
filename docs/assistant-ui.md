# 助手交互界面

Web 常驻助手（`/assistant`）支持在消息中生成交互界面，使用 OpenUI 官方完整通用组件库（当前版本注册 82 个组件及子组件）。普通回答仍使用 Markdown；模型根据任务选择表达形式，也可以直接指定：

- “把这几个方案做成对比表，按名称筛选。”
- “根据刚才查到的任务数据画趋势图，另一个分页签显示明细。”
- “用步骤和折叠区解释发布流程。”
- “用表单收集时间、范围和通知偏好，范围可以多选。”

支持布局与内容（Stack、Card、Tabs、Accordion、Steps、Carousel、Modal、Markdown、代码和图片）、表格与可编辑表格、折线/柱状/面积/饼/雷达/散点等图表、指标与多种卡片，以及文本、日期、单选、多选、开关、滑块、选择标签和选项卡片。图表必须使用用户提供或工具查询的数据；演示数据需要明确标注。

## 使用方式

1. 助手按内容逐步生成界面，必要时包含输入控件、筛选和详情弹窗。
2. 生成期间控件不可提交。生成结束后填写内容，点击提交或卡片按钮。
3. 操作作为下一条用户消息交给同一个助手；已有任务运行时进入 follow-up 队列。
4. 助手继续调用原有工具处理请求，危险操作仍经过独立的审批流程。提交表单不表示操作已完成或已获审批。

每个代码块只接受一次对话提交。已接受的操作与输入控件显示为只读，Tabs、表格分页等内容浏览仍可用；多个独立提交应使用多个代码块。字段值（包括未修改的默认值、多选、开关、数值和日期）保存在用户消息中，重新打开会话可以恢复。入队操作可以通过原有队列删除、排序或改为文字消息；删除后原界面可以再次提交。网络请求失败时当前表单保留填写内容，可重试。右侧问题导航展示操作的请求文本，消息正文另展示填写结果。

尚未提交的草稿仅保留在当前挂载的界面中，刷新或离开会话后不保证保留。首版不提供跨标签页的操作去重；提交结果不明时，应先检查会话或队列再重试。

仅常驻助手的消息启用交互渲染。普通聊天、分享页、CLI 和消息通道按原有文本方式展示；助手需要在界面之外保留简短说明，用户也可以直接用文字回复。

## 实现约定

- 固定使用 `@openuidev/react-lang` / `@openuidev/lang-core` 1.1.0 与 `@openuidev/react-ui` 0.18.6 的 `openuiLibrary`，保留全部官方组件 schema。宿主补充交互禁用与默认字段值登记，主题映射到项目已有颜色和字体。没有新增模型服务或 Node 后端。
- 表面、按钮状态、语义反馈与浮层阴影统一遵循 [Web 主题](web-theme.md)，保留楷体；图表数据系列沿用官方独立色板。
- 每个 `openui-lang` Markdown 围栏包含一个独立的完整 `root = Stack(...)` 程序。渲染器按消息分段的稳定标识与块序号识别来源；普通代码块中的示例不会被执行。已移除首版自定义组件，不兼容旧 `Card(title, children)` 程序。
- UI 文本继续通过现有消息 `Content` 流传输和持久化，不增加第二份流式内容或改写历史事实。提交消息使用 `【助手界面操作】` 前缀及版本化 JSON，包含来源、请求、字段值、可选动作上下文与局部状态；日期使用 `{$date: ISO字符串}` 存储并在回放时恢复为 Date。Web 将其展示为可读文本。
- 输入框和界面操作共用 `sendChatMessage`，复用 `/api/fkteams/stream/start`、任务订阅、队列接纳及失败回滚。
- `@ToAssistant(message, context)` 通过现有链路继续对话；`@OpenUrl` 只打开无内嵌凭据的 HTTP/HTTPS 链接。支持 `$变量`、`@Set`、`@Reset` 和官方内置数据函数进行本地筛选与内容探索。不接入 `toolProvider`、浏览器 MCP、Query/Mutation 或 `@Run`；程序经同源 lexer 检查动作白名单、长度与嵌套上限，数据提交限制长度、深度与字段类型。未知或错误程序提供文字回复的降级提示。
- 流式未完成、程序不完整、提交中、会话已切换或已有对应提交时禁止输入和动作提交。UI 操作是用户输入，不是工具审批凭证。
- 界面代码按需加载。字段值不应包含密码、API key 或其他凭据。
- 官方完整样式只在全局 `globals.css` 中导入一次，使用 layered 版本。Tailwind 3 的输出分别放入原生 `app-base`、`app-components`、`app-utilities` 层，优先级为基础重置 < OpenUI < 项目组件 < 工具类；运行时主题变量保持官方的无层注入方式。不要再从消息组件或路由加载另一份完整样式。
- 组件库与 `ThemeProvider` 统一从 `@openuidev/react-ui` 根入口导入。当前 0.18.6 的 `genui-lib` 子入口包含独立的主题上下文，混用会导致 portal 弹窗取不到主题；升级依赖时回归消息正文、弹窗和日期选择器的主题一致性。
- Vite 将上游自动加载的 `@openuidev/devtools` 映射为本地空实现，开发环境也不加载 CDN 调试脚本或显示部署推广浮窗。升级 OpenUI 后需检查此映射仍覆盖上游的自动加载入口。
- 生产分包显式隔离 React、状态库、通用样式工具、Markdown 与 OpenUI 的其余运行时依赖，避免组件库反向引用消息分包而发生循环初始化。修改依赖或分包配置后，使用构建产物验证助手交互界面，并检查普通页面不会提前加载整个 OpenUI。

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
