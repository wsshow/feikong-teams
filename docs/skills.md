# Skills 指南

Skills 将指令、脚本和资源组织成可按需加载的工作流程，用于补充领域知识或团队约定。技能内容保存在应用目录的 `skills/<slug>/`，每个目录必须包含 `SKILL.md`。

## 创建技能

Web“技能”页支持创建、编辑文件，也可让 AI 起草后再审查保存。手动创建的最小示例：

```markdown
---
name: 代码审查
description: 审查代码改动，检查错误、兼容性与验证结果。
---

1. 阅读改动及相关调用方。
2. 优先报告可复现的问题，并提供文件位置。
3. 说明已完成的验证和未覆盖的风险。
```

把文件保存到 `~/.fkteams/skills/review-helper/SKILL.md`；使用 `FEIKONG_APP_DIR` 时相应替换应用目录。脚本和参考资料放在同一技能目录，通过相对路径引用。完整命令行集成示例见仓库附带的 [fkteams 技能](skills/fkteams/SKILL.md)。

## 搜索、安装与移除

默认市场为 SkillHub：

```bash
fkteams skill list
fkteams skill search ffmpeg
fkteams skill install video-frames
fkteams skill remove video-frames
```

| 命令 | 选项 |
| --- | --- |
| `skill search` | `--page` 默认 1，`--size` 默认 10，`--provider` 可多次指定 |
| `skill install` | `--version` 指定版本，`--provider` 指定市场 |
| `skill remove` | 别名 `skill rm` |

```bash
fkteams skill search ffmpeg --page 2 --size 20
fkteams skill install video-frames --version 1.0.0 --provider SkillHub
```

安装到 `skills/<slug>/`，同名技能会覆盖。启用前检查其中的指令和脚本，实际文件与命令操作仍受工具权限约束。HTTP 集成见[技能 API](api/skills.md)。
