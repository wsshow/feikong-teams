# fkteams 开发规范

## 项目简介

fkteams 是基于 CloudWeGo Eino 框架的多智能体协作系统，采用监督者模式架构，支持 CLI 和 Web 双界面。

## AI 开发行为规范

### 禁止行为

1. **禁止随意创建文件**: 不要自行创建 markdown 文档、测试文件或其他辅助文件，除非用户明确要求
2. **禁止冗余代码**: 不添加未使用的 import、变量或函数
3. **禁止过度注释**: 只在复杂逻辑处添加必要注释

### 必须执行

1. **功能变更后更新 README.md**: 任何功能新增、修改或删除，必须同步更新 `README.md` 相关章节
2. **新增环境变量更新 .env.example**: 添加新环境变量时，必须在 `.env.example` 中添加对应条目和注释
3. **代码审查**: 编码完成后必须检查代码是否有语法错误、逻辑问题，确保无编译报错
4. **保持简洁**: 代码实现以最简洁有效的方式完成，避免过度设计

## 项目结构

```
fkteams/
├── main.go                 # 程序入口
├── agents/                 # 智能体实现
│   ├── common/             # 公共配置 (NewChatModel, MaxIterations)
│   ├── coder/              # 代码专家 (小码)
│   ├── searcher/           # 搜索专家 (小搜)
│   ├── cmder/              # 命令行专家 (小令)
│   ├── visitor/            # SSH专家 (小访)
│   ├── storyteller/        # 讲故事专家 (小天)
│   ├── analyst/            # 数据分析师
│   ├── leader/             # 团队协调者 (统御)
│   ├── moderator/          # 自定义模式主持人
│   └── ...
├── tools/                  # 工具实现
│   ├── tools.go            # 工具注册入口
│   ├── file/               # 文件操作 (沙箱隔离)
│   ├── git/                # Git 仓库操作
│   ├── excel/              # Excel 文件处理
│   ├── command/            # 命令执行
│   ├── ssh/                # SSH 连接
│   ├── search/             # DuckDuckGo 搜索
│   ├── todo/               # 待办事项管理
│   ├── script/             # 脚本工具
│   │   ├── uv/             # Python uv 工具
│   │   └── bun/            # JavaScript bun 工具
│   ├── mcp/                # MCP 协议工具
│   └── ...
├── config/                 # 配置管理
├── fkevent/                # 事件系统
├── server/                 # Web 服务
└── release/                # 构建输出
```

## 开发约定

### 智能体开发

每个智能体包含两个文件:
- `{name}.go`: 实现 `NewAgent() adk.Agent` 函数
- `prompt.go`: 定义 `PromptTemplate` 系统提示词

```go
// 标准智能体创建模式
func NewAgent() adk.Agent {
    ctx := context.Background()
    tools, _ := toolPackage.GetTools()
    systemMessages, _ := PromptTemplate.Format(ctx, map[string]any{
        "current_time": time.Now().Format("2006-01-02 15:04:05"),
    })
    return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
        Name:          "智能体名称",
        Description:   "智能体描述",
        Instruction:   systemMessages[0].Content,
        Model:         common.NewChatModel(),
        MaxIterations: common.MaxIterations,
        ToolsConfig:   adk.ToolsConfig{...},
    })
}
```

### 工具开发

工具通过 `tools/tools.go` 的 `GetToolsByName(name string)` 注册:
- 内置工具: `file`, `git`, `excel`, `todo`, `ssh`, `command`, `search`, `uv`, `bun`
- MCP 工具: 使用 `mcp-{server_name}` 前缀

#### 工具函数错误处理规范

**关键原则**: 工具函数不应该抛出 Go error，而应该将错误信息放在响应结构体的 `ErrorMessage` 字段中。

```go
// ❌ 错误做法 - 会导致 Agent 流程中断，用户看到系统级错误
func (t *Tool) DoSomething(ctx context.Context, req *Request) (*Response, error) {
    if req.Param == "" {
        return &Response{
            ErrorMessage: "参数不能为空",
        }, fmt.Errorf("param is required")  // ❌ 不要返回 error
    }
    
    if err := someOperation(); err != nil {
        return &Response{
            ErrorMessage: fmt.Sprintf("操作失败: %v", err),
        }, err  // ❌ 不要返回 error
    }
    
    return &Response{Success: true}, nil
}

// ✅ 正确做法 - Agent 可以看到错误信息并继续执行
func (t *Tool) DoSomething(ctx context.Context, req *Request) (*Response, error) {
    if req.Param == "" {
        return &Response{
            Success:      false,
            ErrorMessage: "参数不能为空，请检查输入",
        }, nil  // ✅ 返回 nil
    }
    
    if err := someOperation(); err != nil {
        return &Response{
            Success:      false,
            ErrorMessage: fmt.Sprintf("操作失败: %v，请检查...", err),
        }, nil  // ✅ 返回 nil
    }
    
    return &Response{Success: true, Message: "操作成功"}, nil
}
```

**为什么这样做？**
1. Agent 可以读取错误信息并采取补救措施
2. 用户看到友好的错误提示而非系统异常
3. 不会中断整个执行流程，Agent 可以继续尝试其他方案

**何时可以返回 error？**
仅在工具初始化阶段（如 `NewXXXTools`）可以返回 error，因为初始化失败应该阻止程序启动。

### 命名规范

| 类型 | 规范 | 示例 |
|------|------|------|
| 智能体名称 | 中文 | 小码、小搜 |
| 包/文件名 | 小写英文 | coder, searcher |
| 工具函数 | 下划线分隔 | file_read, ssh_execute |
| 环境变量 | FEIKONG_ 前缀 | FEIKONG_OPENAI_API_KEY |

### 代码风格规范

1. **错误信息使用英文**: `fmt.Errorf`、`errors.New` 等错误信息一律使用英文
   ```go
   // 正确
   return fmt.Errorf("failed to read file: %w", err)
   // 错误
   return fmt.Errorf("读取文件失败: %w", err)
   ```

2. **注释使用中文**: 代码注释、文档注释使用中文
   ```go
   // 初始化文件工具，限制在指定目录内
   func NewFileTools(dir string) (*FileTools, error) {
       // ...
   }
   ```

3. **日志输出**: 面向用户的日志可使用中文，内部调试日志使用英文

### 环境变量

必需:
```bash
FEIKONG_OPENAI_API_KEY    # API 密钥
FEIKONG_OPENAI_BASE_URL   # API 地址
FEIKONG_OPENAI_MODEL      # 模型名称
```

可选 (完整列表见 `.env.example`):
```bash
FEIKONG_WORKSPACE_DIR     # 工作目录 (默认 ./workspace)
FEIKONG_PROXY_URL         # 代理地址
FEIKONG_SSH_*             # SSH 连接配置
```

## 构建与运行

```bash
make build                                    # 构建到 release/
./release/fkteams_darwin_arm64 -m team -q "问题"  # CLI 模式
./release/fkteams_darwin_arm64 -m team -w     # Web 模式 (默认 8080)
```

## 代码风格

1. 错误处理: 初始化失败用 `log.Fatal()`，运行时错误通过事件系统传递
2. 并发安全: WebSocket 连接池使用 `sync.Mutex` 保护
3. 文件操作: 所有路径通过 `afero.BasePathFs` 沙箱隔离
4. 日志格式: `log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)`

## 检查清单

完成开发后确认:

- [ ] 代码无编译错误
- [ ] 无未使用的 import/变量
- [ ] 功能变更已更新 README.md
- [ ] 新环境变量已添加到 .env.example
- [ ] 遵循现有代码风格
