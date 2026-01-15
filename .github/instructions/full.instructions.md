# fkteams AI 开发指南

## 项目概览

fkteams 是一个基于 CloudWeGo Eino 框架的多智能体协作系统，支持命令行和 Web 双界面模式。核心采用 **监督者模式 (Supervisor Pattern)** 架构，由领导智能体协调多个专业智能体完成复杂任务。

### 三种工作模式

1. **团队模式** (`-m team`): 预定义的 5 个专业智能体（小搜/小码/小令/小访/小天），由"统御"智能体协调
2. **自定义会议模式** (`-m custom`): 通过 `config/config.toml` 自定义智能体及其工具配置
3. **多智能体讨论模式** (`-m group`): 多个 AI 模型进行圆桌会议式讨论

## 核心架构模式

### 智能体创建模式 (Agent Pattern)

所有智能体遵循统一创建模式，参考 [agents/coder/coder.go](../../agents/coder/coder.go):

```go
func NewAgent() adk.Agent {
    ctx := context.Background()
    
    // 1. 初始化工具（如需要）
    tools, err := toolPackage.GetTools()
    
    // 2. 格式化系统提示词（使用当前时间等上下文）
    systemMessages, err := PromptTemplate.Format(ctx, map[string]any{
        "current_time": time.Now().Format("2006-01-02 15:04:05"),
    })
    
    // 3. 创建智能体，注入 Model、Instruction、Tools
    return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
        Name:          "智能体名称",
        Description:   "智能体描述",
        Instruction:   systemMessages[0].Content,
        Model:         common.NewChatModel(),
        MaxIterations: common.MaxIterations, // 固定 60 轮
        ToolsConfig:   adk.ToolsConfig{...},
    })
}
```

**关键约定**:
- 每个智能体包含 `{agent_name}.go` 和 `prompt.go` 两个文件
- `prompt.go` 使用 Eino 的 `compose.PromptTemplate` 定义系统提示词
- 智能体名称使用中文（小码、小搜等），便于用户理解
- 所有智能体最大迭代次数固定为 60 (`common.MaxIterations`)

### 监督者模式 (Supervisor Pattern)

团队和自定义模式使用 Supervisor 协调多智能体，参考 [main.go](../../main.go):

```go
supervisorAgent, err := supervisor.New(ctx, &supervisor.Config{
    Instruction: leaderInstruction,
    Leader:      leaderAgent,              // 协调者智能体
    Agents:      []adk.Agent{agent1, ...}, // 被协调的专业智能体
    Model:       common.NewChatModel(),
})
```

**工作流程**:
1. Leader Agent 分析任务并决定调用哪个 Agent
2. 被调用的 Agent 执行具体工具操作
3. Leader Agent 综合结果并决定下一步

### 工具系统架构

#### 内置工具

工具通过名称获取，参考 [tools/tools.go](../../tools/tools.go) (L17-L62):

```go
func GetToolsByName(name string) ([]tool.BaseTool, error) {
    switch name {
    case "file":    // 文件操作，限制在 ./code 目录
    case "todo":    // 待办事项管理
    case "ssh":     // SSH 远程连接
    case "command": // 命令行执行
    case "search":  // DuckDuckGo 搜索
    default:
        // MCP 工具以 "mcp-" 前缀
        if name, ok := strings.CutPrefix(name, "mcp-"); ok {
            return mcp.GetToolsByName(name)
        }
    }
}
```

**文件工具安全限制**: 所有文件操作自动限制在配置的安全目录内（默认 `./code`），通过 `afero` 的 `BasePathFs` 实现沙箱隔离。

#### MCP 工具集成

支持 Model Context Protocol，配置在 `config/config.toml` 的 `[[custom.mcp_servers]]` 节，支持三种传输方式:
- `transport_type = "http"`: HTTP 连接，需配置 `url`
- `transport_type = "sse"`: Server-Sent Events，需配置 `url`  
- `transport_type = "stdio"`: 标准输入输出，需配置 `command` 和 `args`

MCP 工具通过前缀 `mcp-{server_name}` 引用，工具在首次调用时初始化并缓存。

### 事件系统 (Event System)

双界面模式通过统一的事件系统实现，参考 [fkevent/event.go](../../fkevent/event.go):

```go
var Callback func(event Event) error  // 全局回调函数

func ProcessAgentEvent(ctx context.Context, event *adk.AgentEvent) error {
    // 将 Eino 事件转换为统一的 Event 结构
    // 支持: message, tool_call, tool_result, error
}
```

**CLI 模式**: 直接使用 `pterm` 彩色打印
**Web 模式**: 通过 `Callback` 函数发送 WebSocket 消息到前端

### 历史记录管理

会话历史通过 `fkevent.GlobalHistoryRecorder` 管理:
- `LoadFromDefaultFile()`: 加载默认历史文件
- `GetMessages()`: 获取历史消息用于上下文注入
- `RecordUserInput()` / `RecordAgentResponse()`: 记录对话
- `SaveToMarkdownWithTimestamp()`: 导出为 Markdown

历史文件存储在 `release/history/` 目录，按会话 ID 分文件存储。

## 开发工作流

### 构建与运行

```bash
# 开发构建（当前平台）
make build  # 输出到 release/ 目录

# 运行命令行模式
./release/fkteams_darwin_arm64 -m team -q "你的问题"

# 运行 Web 模式（默认 8080 端口）
./release/fkteams_darwin_arm64 -m team -w
```

### 环境变量配置

必需变量（参考 [agents/common/common.go](../../agents/common/common.go)):
```bash
FEIKONG_OPENAI_API_KEY    # OpenAI API 密钥
FEIKONG_OPENAI_BASE_URL   # API 基础 URL
FEIKONG_OPENAI_MODEL      # 模型名称（如 deepseek-chat）
```

可选变量:
```bash
FEIKONG_FILE_TOOL_DIR     # 文件工具安全目录（默认 ./code）
FEIKONG_TODO_TOOL_DIR     # Todo 工具数据目录（默认 ./todo）
FEIKONG_SSH_HOST          # SSH 连接主机
FEIKONG_SSH_USERNAME      # SSH 用户名
FEIKONG_SSH_PASSWORD      # SSH 密码
```

### 添加新智能体

1. 在 `agents/` 创建新目录 `agents/newagent/`
2. 创建 `newagent.go` 实现 `NewAgent()` 函数
3. 创建 `prompt.go` 定义 `PromptTemplate`
4. 如需新工具，在 `tools/` 添加工具包
5. 在 `main.go` 或 `server/handler/websocket.go` 注册智能体

参考 [agents/coder/](../../agents/coder/) 的完整实现。

### 自定义模式配置

编辑 `config/config.toml` 的 `[custom]` 部分:

```toml
[custom.moderator]
name = "主持人"
desc = "负责协调讨论"
system_prompt = "你是一个公正的主持人..."
base_url = "https://api.example.com/v1"
api_key = "your_key"
model_name = "deepseek-chat"

[[custom.agents]]
name = "专家1"
desc = "领域专家"
system_prompt = "你是一个专家..."
tools = ["file", "mcp-server-name"]  # 内置工具或 MCP 工具
# ... 其他配置

[[custom.mcp_servers]]
name = "example-server"
enabled = true
transport_type = "stdio"
command = "npx"
args = ["-y", "@modelcontextprotocol/server-example"]
```

## 关键技术约定

### 依赖管理
- 使用 Go 1.25.3+
- 核心框架: CloudWeGo Eino (ADK 和预构建组件)
- UI 库: pterm (CLI), Gin + WebSocket (Web)
- 文件系统: spf13/afero (沙箱隔离)

### 错误处理
- 工具初始化失败使用 `log.Fatal()` 快速失败
- 运行时错误通过 Event 系统传递，由 `fkevent.ProcessAgentEvent` 统一处理
- WebSocket 连接错误会自动清理并通知前端

### 并发安全
- WebSocket 连接池使用 `sync.Mutex` 保护 (`server/handler/websocket.go`)
- 每个 WebSocket 连接有独立的 context 和 cancel 函数
- MCP 工具实现工具缓存避免重复初始化

### 命名约定
- 智能体使用中文名称（用户友好）
- 文件/包使用英文小写（技术标准）
- 工具名称使用下划线分隔（`file_read`, `ssh_execute`）
- 环境变量统一前缀 `FEIKONG_`

## 常见陷阱

⚠️ **文件工具路径**: 永远不要在文件工具外部直接操作文件系统，所有路径自动沙箱化
⚠️ **智能体迭代**: 不要修改 `MaxIterations` 除非理解整个系统影响
⚠️ **MCP 工具缓存**: MCP 工具首次初始化后缓存，配置变更需重启程序
⚠️ **历史记录**: 在非交互模式使用 `-q` 时，历史会自动加载和保存
⚠️ **WebSocket 生命周期**: 每个 WebSocket 请求创建新 Runner，连接断开需清理资源

## 调试技巧

- 使用 `log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)` 查看详细日志
- 查看 `release/history/` 目录的对话记录文件
- Web 模式打开浏览器控制台查看 WebSocket 消息
- 团队模式可在 Leader 的 prompt 中查看决策逻辑
