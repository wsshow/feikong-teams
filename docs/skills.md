# Skills 指南

## 什么是 Skills？

Skills 是一组指令、脚本和资源的集合，fkteams(统御) 可以动态加载这些 Skills 来提升在特定任务上的表现。Skills 教会 fkteams(统御) 如何以可重复的方式完成特定任务，无论是按照公司的品牌指南创建文档，还是使用组织特定的工作流程分析数据，或者自动化个人任务。

## Skills 配置

skills目录：{FEIKONG_WORKSPACE_DIR}/skills/{用户技能目录}

「用户技能目录」是一个独立的子目录，必须包含: `SKILL.md`

`SKILL.md` 文件示例：

```markdown
---
name: "数据分析"
description: "一套用于数据分析的技能，包含数据清洗、统计分析和数据可视化的指令和脚本。"
---

## 具体的数据分析技能描述...
```

## 推荐的 Skills

- https://github.com/anthropics/skills/tree/main/skills/skill-creator
