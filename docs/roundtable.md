# 圆桌会议模式详解

圆桌会议模式让多个配置成员按顺序讨论同一个问题，并由主持流程汇总观点。

## 启动方式

```bash
fkteams -m group
fkteams -m group -q "讨论一下微服务架构的优劣"
fkteams web
```

## 配置

先在[模型池](configuration.md#模型池)中配置模型。以下示例引用 `main` 和 `fast`，成员按数组顺序发言：

```toml
[roundtable]
max_iterations = 2

[[roundtable.members]]
id = "logic"
name = "逻辑分析"
description = "擅长逻辑分析"
model_id = "main"
prompt = ""

[[roundtable.members]]
id = "creative"
name = "创意视角"
description = "擅长创意思维"
model_id = "fast"
prompt = "你是一个擅长创意思维的圆桌讨论者。请优先提出不同视角和替代方案。"
```

成员字段：

| 参数 | 说明 |
| ---- | ---- |
| `id` | 稳定成员 ID |
| `name` | 展示名称 |
| `description` | 能力描述 |
| `model_id` | 引用 `[[models]].id` |
| `prompt` | 可选自定义提示词，留空使用内置圆桌讨论者提示词 |

## 建议

- 为成员选择不同特点的模型，以获得更多元的观点。
- `max_iterations` 是讨论轮数，建议 1–3；设为 `0` 时持续讨论，直到手动取消。
- 用清晰的 `description` 标注成员专长，方便讨论过程保持角色边界。
