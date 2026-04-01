# Skills 指南

## 什么是 Skills？

Skills 是一组指令、脚本和资源的集合，fkteams(统御) 可以动态加载这些 Skills 来提升在特定任务上的表现。Skills 教会 fkteams(统御) 如何以可重复的方式完成特定任务，无论是按照公司的品牌指南创建文档，还是使用组织特定的工作流程分析数据，或者自动化个人任务。

## Skills 配置

skills目录：`{FEIKONG_APP_DIR}/skills/{用户技能目录}`

「用户技能目录」是一个独立的子目录，必须包含: `SKILL.md`

`SKILL.md` 文件示例：

```markdown
---
name: "数据分析"
description: "一套用于数据分析的技能，包含数据清洗、统计分析和数据可视化的指令和脚本。"
---

## 具体的数据分析技能描述...
```

## 技能管理命令

fkteams 内置了 [SkillHub](https://skillhub.tencent.com/) 作为默认技能市场，你可以搜索、安装和管理技能。

### 列出本地技能

查看已安装在本地的所有技能：

```bash
fkteams skill list
```

输出示例：

```
可用的技能列表
  video-frames  Video Frames
    从视频中提取关键帧并分析
```

### 搜索技能市场

从远程技能市场搜索可用技能：

```bash
fkteams skill search <关键词>
```

支持的参数：

| 参数         | 说明                   | 默认值 |
| ------------ | ---------------------- | ------ |
| `--page`     | 页码                   | 1      |
| `--size`     | 每页数量               | 10     |
| `--provider` | 指定后端（可多次指定） | 全部   |

示例：

```bash
# 搜索 ffmpeg 相关技能
fkteams skill search ffmpeg

# 从指定后端搜索
fkteams skill search ffmpeg --provider SkillHub

# 翻页
fkteams skill search ffmpeg --page 2 --size 20
```

### 安装技能

从技能市场下载并安装技能到本地：

```bash
fkteams skill install <技能slug>
```

支持的参数：

| 参数         | 说明     | 默认值   |
| ------------ | -------- | -------- |
| `--version`  | 指定版本 | 最新版本 |
| `--provider` | 指定后端 | 默认后端 |

示例：

```bash
# 安装最新版本
fkteams skill install video-frames

# 安装指定版本
fkteams skill install video-frames --version 1.0.0

# 从指定后端安装
fkteams skill install video-frames --provider SkillHub
```

技能将安装到 `~/.fkteams/skills/<slug>/` 目录下。如果技能已存在，将覆盖安装。

### 移除技能

移除本地已安装的技能：

```bash
fkteams skill remove <技能slug>
# 或使用别名
fkteams skill rm <技能slug>
```

示例：

```bash
fkteams skill remove video-frames
```

## 推荐的 Skills

- https://github.com/anthropics/skills/tree/main/skills/skill-creator
