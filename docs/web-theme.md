# Web 主题

Web UI 使用“清透蓝白”浅色主题，延续 OpenUI 的蓝色与中性色体系，保留楷体和队徽。主内容、侧栏共用极浅冷白底色，卡片和浮层使用纯白；选中态通过浅蓝底色和蓝色文字表达。

## 配色与状态

| 用途 | 色值 | 主题变量 |
| --- | --- | --- |
| 页面、侧栏 | `#FAFCFF` | `--background`、`--sidebar` |
| 卡片、弹窗 | `#FFFFFF` | `--card`、`--popover` |
| 主操作 | `#2563EB` | `--primary` |
| 主操作悬停、按下 | `#1D4ED8`、`#1E40AF` | `--primary-hover`、`--primary-pressed` |
| 选中项、用户消息 | `#EFF6FF` | `--accent` |
| 正文 | `#1E293B` | `--foreground` |
| 说明、占位文字 | `#64748B` | `--muted-foreground` |
| 输入区域底色 | `#F8FAFC` | `--muted` |
| 常规分隔、输入框边线 | `#E2E8F0`、`#CBD5E1` | `--border`、`--input` |

禁用操作继续使用淡色，焦点使用蓝色边线与浅蓝焦点环。危险、警告与成功反馈分别使用 `destructive`、`warning`、`success` 语义变量，不用品牌蓝替代状态提示。正文、说明及可操作按钮的普通字号对比度由 `web/src/styles/theme.test.ts` 校验。

## 样式约定

- 颜色统一在 `web/src/styles/globals.css` 的根变量中定义；组件通过 Tailwind 语义类引用，不另写米黄色填充、蓝灰边框或固定色值阴影。
- 普通内容通过细边框和柔和阴影分层，取消纸张渐变和偏移硬阴影。`shadow-surface` 用于普通卡片，`shadow-raised` 用于输入区和可点击卡片的悬停、焦点状态，`shadow-popover` 用于菜单，`shadow-dialog` 用于模态浮层；对应阴影也在根变量中定义。
- 按钮使用 `shadow-control` 表达轻微厚度，主按钮使用同色系 `shadow-primary`，悬停时增强、按下时收敛，禁用时移除阴影。幽灵按钮与普通导航保持平整，避免所有元素同时悬浮。
- `sketch-surface`、`sketch-inset`、`sketch-rule` 保留现有调用方式，分别提供带轻阴影的白色内容表面、带内阴影的浅色输入表面和中性边线。OpenUI 的 `card` 同样轻微抬起，`sunk` 保持内凹语义。
- 配置目录卡片在网格中同排等高，标题区预留操作按钮高度，描述区预留两行，摘要信息靠底对齐；窄屏允许内容自然增高，不通过固定高度裁掉信息。
- Markdown 代码、表格、引用与滚动条复用主题变量。图表数据系列保留 OpenUI 的独立色板。
- OpenUI 由 `web/src/features/chat/openui/theme.ts` 映射相同的表面、文字、交互状态、语义反馈和浮层阴影，弹窗通过同一主题上下文继承配色。

调整主题后运行 `cd web && bun test src`、`make web-typecheck`、`make web-build` 和 `make diff-check`，并核对桌面与窄屏的普通页面、表单、弹窗及助手交互卡片。

## 会话间距

会话排版以 4px 为基础单位。正文、思考和工具行之间，以及同一位助手连续模型输出之间，统一使用 `--chat-part-gap`（12px）；新一轮用户或助手消息使用 `--chat-turn-gap`（24px）。连续工具行按列表紧凑排列，内部与跨消息边界均不额外加间距。Markdown 首尾块的外边距归零，由会话容器控制分段留白。

“已思考”和工具按钮采用相同的最小行高：桌面 40px，窄屏 44px。主会话、成员详情和分享页复用这些规则，间距不因持久化消息的切分方式而变化。
