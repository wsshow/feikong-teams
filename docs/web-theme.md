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
- 普通内容通过细边框分隔，取消纸张渐变和偏移硬阴影。`shadow-surface` 用于轻微抬起的区域，`shadow-popover` 用于菜单，`shadow-dialog` 用于模态浮层；对应阴影也在根变量中定义。
- `sketch-surface`、`sketch-inset`、`sketch-rule` 保留现有调用方式，分别提供白色内容表面、浅色输入表面和中性边线。
- Markdown 代码、表格、引用与滚动条复用主题变量。图表数据系列保留 OpenUI 的独立色板。
- OpenUI 由 `web/src/features/chat/openui/theme.ts` 映射相同的表面、文字、交互状态、语义反馈和浮层阴影，弹窗通过同一主题上下文继承配色。

调整主题后运行 `cd web && bun test src`、`make web-typecheck`、`make web-build` 和 `make diff-check`，并核对桌面与窄屏的普通页面、表单、弹窗及助手交互卡片。
