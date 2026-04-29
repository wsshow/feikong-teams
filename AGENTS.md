# fkteams

基于 CloudWeGo Eino ADK 的多智能体协作系统，CLI + Web 双界面。

## 构建与测试

```bash
go build ./...          # 编译
go vet ./...            # 静态检查
make build              # 构建到 release/
```

## 项目架构

- **入口**: `main.go` → `commands/` (Cobra)
- **引擎**: `engine/` — 统一生命周期入口，`RunConfig` 集中管理 context 装配和回调（OnStart → OnInterrupt → OnFinish），各入口点不再手动装配 context
- **智能体**: `agents/` — 通过 `AgentBuilder` 流式构建，`registry.go` 延迟加载
- **工具**: `tools/` — `GetToolsByName()` 统一注册
- **事件**: `fkevent/` — `types.go` 定义 EventType/ActionType/NotifyType 常量，禁止使用字符串字面量
- **Web**: `server/` — `lifecycle.Service` 接口 + Gin 路由
- **通道**: `channels/` — Discord/微信/QQ 消息桥接
- **数据目录**: `~/.fkteams/{workspace,scheduler,sessions,history,config,log}`

## 代码风格

1. **错误信息英文，注释中文**
2. **禁止 emoji 图形字符**（文字符号如 ✓✗ 允许）
3. **向 `strings.Builder` 写格式化内容用 `fmt.Fprintf(&sb, ...)`**，不用 `sb.WriteString(fmt.Sprintf(...))`
4. **用 `any` 替代 `interface{}`**
5. **工具函数不返回 error**：将错误信息放入响应的 `ErrorMessage` 字段并返回 nil
6. **初始化函数必须返回 error**，不使用 `log.Fatal`

## 开发约定

- 新智能体必须使用 `agents/common/builder.go` 的 `AgentBuilder` 创建
- 新工具必须在 `tools/tools.go` 的 `GetToolsByName()` 注册
- 新配置项必须添加到 `config/config.go` 的 `GenerateExample()`
- 功能变更必须同步更新 `README.md`
- 事件处理使用 `fkevent/types.go` 中的类型常量，禁止事件类型的字符串字面量
- `RunConfig.OnInterrupt` 为 nil 时自动使用 `AutoRejectHandler`
