# 圆桌会议模式详解

## 启动方式

```bash
# CLI 交互模式
fkteams -m group

# 直接查询
fkteams -m group -q "讨论一下微服务架构的优劣"

# Web 模式中通过侧边栏选择讨论模式
fkteams web
```

## 工作原理

圆桌会议模式模拟了一场专家研讨会：

1. **问题提出**：用户提出问题或任务
2. **轮流发言**：每个配置的模型依次针对问题发表观点
3. **观点参考**：后发言的模型可以看到前面模型的观点，并在此基础上补充或提出不同见解
4. **多轮迭代**：根据 `max_iterations` 配置进行多轮讨论，逐步深化分析
5. **形成共识**：最终综合各方观点，给出更全面准确的答案

## 配置

在 `~/.fkteams/config/config.toml` 中配置圆桌会议成员。成员的模型通过 `model` 字段引用 `[[models]]` 中定义的具名模型。

```toml
# 先定义模型
[[models]]
name = "deepseek"
provider = "deepseek"
base_url = "https://api.deepseek.com/v1"
api_key = "your_deepseek_key"
model = "deepseek-chat"

[[models]]
name = "claude"
provider = "claude"
base_url = "https://api.anthropic.com"
api_key = "your_claude_key"
model = "claude-3-sonnet"

# 圆桌会议配置
[roundtable]
max_iterations = 2  # 讨论轮数

[[roundtable.members]]
index = 0
name = "深度求索"
desc = "深度求索聊天模型，擅长逻辑分析"
model = "deepseek"

[[roundtable.members]]
index = 1
name = "克劳德"
desc = "克劳德聊天模型，擅长创意思维"
model = "claude"
```

### 成员配置参数

| 参数    | 说明                           | 必填 |
| ------- | ------------------------------ | ---- |
| `index` | 发言顺序（从 0 开始）          | ✓    |
| `name`  | 成员名称                       | ✓    |
| `desc`  | 成员描述（帮助理解其专长）     | ✓    |
| `model` | 引用 `[[models]]` 中的模型名称 | ✓    |

## 适用场景

- **复杂决策**：需要从多角度分析的重要决策
- **创意头脑风暴**：激发不同模型的创意火花
- **观点验证**：让多个模型相互验证，减少单一模型的偏见
- **深度分析**：需要多轮思考才能得出结论的复杂问题

## 配置建议

- 选择不同特点的模型作为讨论成员，以获得更多元的观点
- `max_iterations` 建议设置为 1-3，过多轮次可能导致观点趋同
- 可以给每个成员设置描述性的 `desc`，帮助理解其专长
