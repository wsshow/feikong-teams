# JavaScript 扩展

fkteams 内置了隔离的同步脚本运行时。打开 Web 配置页的“脚本”页签，可以手写或通过 AI 生成两类扩展：

- 自定义工具：注册为模型可发现、可调用的新工具。
- 流程 Hook：在运行、事件、工具调用和模型请求边界观察或控制流程。

AI 草稿会先校验语法和入口函数，审查后启用并保存。新的工具和 Hook 对后续任务生效，无需重启。

## 自定义工具

每个工具必须提供唯一 ID、用途描述、参数 JSON Schema 和 `execute(input, context)` 函数：

```javascript
function execute(input, context) {
  const text = String(input.text || "");
  return {
    characters: Array.from(text).length,
    words: text.trim() ? text.trim().split(/\s+/).length : 0,
    call_id: context.call_id,
  };
}
```

对应的参数 Schema：

```json
{
  "type": "object",
  "properties": {
    "text": {
      "type": "string",
      "description": "需要统计的文本"
    }
  },
  "required": ["text"]
}
```

`input` 是模型生成的参数对象。`context` 包含调用元数据和自动接入的宿主函数：

| API | 返回值 | 说明 |
| --- | --- | --- |
| `context.workspace.read(path, options?)` | 文件文本 | 读取工作区文件；可传 `start_line`、`end_line` |
| `context.workspace.write(path, content)` | 写入结果 | 覆盖写入工作区文件 |
| `context.workspace.append(path, content)` | 写入结果 | 追加内容，文件不存在时自动创建 |
| `context.workspace.replace(path, oldText, newText)` | 编辑结果 | 精确替换唯一匹配的文本 |
| `context.workspace.patch(diff)` | 补丁结果 | 应用 unified diff，可同时修改多个文件 |
| `context.workspace.list(path)` | 目录文本 | 列出工作区目录 |
| `context.workspace.glob(pattern, options?)` | 路径数组 | 按文件名匹配；可传 `path` |
| `context.workspace.grep(pattern, options?)` | 匹配数组 | 搜索内容；可传 `path`、`include`、`use_regex`、`context`、`max_count` |
| `context.shell.run(command, options?)` | 命令结果 | 在工作区执行命令；可传 `timeout`、`reason`、`background` 等参数 |
| `context.web.search(query, options?)` | 搜索结果数组 | 搜索网络；可传 `time_range` |
| `context.web.fetch(url, options?)` | 网页正文 | 获取网页；可传 `format`、`timeout` |
| `context.state.get/set/delete/keys` | JSON 值 | 使用按工具 ID 隔离的持久化状态 |
| `context.notify(message, level?)` | 无 | 向当前任务发送 `info`、`warn` 或 `error` 通知，默认 `info` |
| `context.log.debug/info/warn/error(value)` | 无 | 写入带工具 ID 的服务日志 |

顶层元数据包括 `tool_name`、`call_id` 和 `session_id`。返回值可以是字符串、数字、布尔值、数组或对象；非字符串值会序列化为 JSON 后返回给模型。

下面的工具读取工作区文件并保存调用次数，不需要额外配置宿主能力：

```javascript
function execute(input, { workspace, state }) {
  const count = (state.get("count") || 0) + 1;
  state.set("count", count);
  const content = workspace.read(input.path);
  return { count, content };
}
```

这些函数由 Go 注入脚本运行时，内部复用现有工具，因此仍会经过工具 Hook、安全策略、审批、串行化和工作区路径限制。便捷函数发现底层工具返回 `error_message` 时会直接抛出异常，可用 `try/catch` 做降级处理。

少数便捷函数无法覆盖的高级场景，可以使用 `context.tools.call(group, tool, args)` 调用已注册的内置或 MCP 工具。它属于底层兼容接口，需要知道准确的工具组、工具名和参数契约；不允许调用 `javascript` 工具组，以避免失控的递归调用。普通脚本和 AI 生成脚本应优先使用上表中的宿主函数。

Web 编辑器支持为当前未保存的工具填写 JSON 输入并试运行。试运行使用与正式调用相同的脚本运行时、宿主函数、工具注册表和 HookBus，并展示原始返回值与脚本通知。它会真实调用工具、写入脚本状态并可能产生副作用；破坏性能力仍需通过既有审批。

启用工具并保存后，内置协调者会自动加载 `javascript` 工具组。自定义智能体可以在“智能体”页签中显式添加“JavaScript 扩展”工具组。

## 流程 Hooks

Hook 必须定义 `handle(hook)`。返回对象支持以下字段：

| 字段 | 说明 |
| --- | --- |
| `action` | `continue`、`skip` 或 `reject` |
| `message` | 拒绝等动作的说明 |
| `payload` | 改写后的载荷；需要修改时应显式返回 |

例如，在命令执行前拒绝包含 `sudo` 的参数：

```javascript
function handle(hook) {
  if (hook.payload.tool_name !== "execute") {
    return { action: "continue" };
  }
  const args = JSON.parse(hook.payload.args || "{}");
  if (/\bsudo\b/.test(String(args.command || ""))) {
    return { action: "reject", message: "sudo is not allowed" };
  }
  return { action: "continue" };
}
```

可用扩展点：

| 扩展点 | 载荷 | 可改写内容 |
| --- | --- | --- |
| `before_run` | `{input:{context,message}}` | 本轮输入 |
| `after_run` | `{input,result,error}` | 仅观察 |
| `on_event` | `{event}` | 事件或跳过分发 |
| `before_tool_call` | `{tool_name,args,meta}` | JSON 参数字符串，或拒绝调用 |
| `after_tool_call` | `{tool_name,args,result,error,meta}` | 仅观察 |
| `before_model_request` | `{messages,meta}` | 发给模型的消息 |
| `after_model_response` | `{message,usage,error,meta}` | 仅观察 |

`hook` 顶层还包含 `point`、`session_id`、`run_id` 和 `turn_id`。多个 Hook 按优先级从小到大执行，前一个 Hook 返回的载荷会传给下一个 Hook。

错误策略支持 `fail`、`warn` 和 `ignore`。未填写时，运行前扩展点默认失败即中止；事件和运行后扩展点默认记录警告并继续。

## 执行边界

- 每次调用都使用独立 Runtime，不共享脚本全局状态。
- 脚本最大 64 KiB，单次默认超时 200 毫秒。工具脚本可配置 10–120000 毫秒，便于等待网络或命令工具；流程 Hook 最多 5000 毫秒，避免长时间阻塞关键边界。
- 超时或上下文取消会中断脚本。
- 不提供 `require`、`process`、定时器或其他 Node.js API；文件、命令和网络访问使用 `context` 中的同步宿主函数。
- 持久化状态按脚本 ID 隔离，单个脚本最多 256 个键、1 MiB 数据，状态存入应用 SQLite，删除脚本不会自动删除其状态。
- 当前只支持同步脚本，不要使用 `async`、Promise 或异步回调。
- JavaScript 配置属于管理能力。对外开放 Web 服务时应启用认证，并只允许可信管理员编辑或启用脚本。

AI 生成只能减少样板工作，不能替代代码审查。启用前应检查循环规模、正则表达式复杂度、参数边界和返回数据大小。
