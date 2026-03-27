# 聊天通道

聊天通道允许将智能体接入外部即时通讯平台，在 `web` 或 `serve` 模式下自动连接并处理消息。目前支持 QQ、Discord 和微信三个平台。

## 架构概览

每个通道实现统一的 `Channel` 接口，通过 `Bridge` 桥接到智能体引擎。通道在服务启动时自动连接，支持独立配置运行模式。

```
用户消息 → Channel（QQ/Discord/微信）→ Bridge → 智能体引擎 → Bridge → Channel → 回复用户
```

## 通用配置

所有通道配置位于 `config/config.toml` 的 `[channels.*]` 下，共有以下通用字段：

| 字段      | 说明                                                                                 |
| --------- | ------------------------------------------------------------------------------------ |
| `enabled` | 是否启用该通道                                                                       |
| `mode`    | 智能体模式：`team`（默认团队）、`deep`、`roundtable`、`custom` 或智能体名称如 `小助` |

## QQ 机器人

### 前置步骤

1. 前往 [QQ 开放平台](https://q.qq.com/#) 注册并创建机器人应用
2. 应用审核通过后，在凭据页面复制 AppID 和 AppSecret
3. 新机器人默认处于**沙箱模式**，需在沙箱配置中添加测试用户和群才能交互

### 配置

```toml
[channels.qq]
enabled = true
app_id = "your_qq_bot_app_id"       # QQ 机器人 AppID
app_secret = "your_qq_bot_secret"   # QQ 机器人 AppSecret
sandbox = true                      # 是否使用沙箱环境（开发阶段建议开启）
mode = "team"                       # 智能体模式
```

### 消息类型

| 消息类型 | 描述           | 触发条件         |
| -------- | -------------- | ---------------- |
| C2C      | 私聊（一对一） | 用户发送任意消息 |
| GroupAT  | 群聊           | 用户必须 @机器人 |

支持文字、图片、语音、视频、文件等多媒体消息的接收与发送。使用 WebSocket 模式实时通信，token 自动刷新。

## Discord 机器人

### 前置步骤

1. 前往 [Discord Developer Portal](https://discord.com/developers/applications) 创建应用
2. 在 Bot 页面添加机器人并复制 Token
3. 启用 **MESSAGE CONTENT INTENT**（Bot 设置页）
4. OAuth2 → URL Generator，Scopes 选 `bot`，Permissions 选 `Send Messages` + `Read Message History`
5. 使用生成的链接邀请机器人到你的服务器

### 配置

```toml
[channels.discord]
enabled = true
token = "your_discord_bot_token"    # Discord Bot Token
allow_from = ""                     # 允许的用户 ID，逗号分隔（空则允许所有人）
mode = "team"                       # 智能体模式
```

支持私聊（DM）和服务器频道（@机器人）消息，支持文字和文件附件。需要网络代理时设置环境变量 `FEIKONG_PROXY_URL`。

## 微信机器人

### 前置步骤

直接启用并在终端微信扫码授权即可。登录后会自动保存凭证，下次启动无需重复扫码。

### 配置

```toml
[channels.weixin]
enabled = true
log_level = "info"                         # 日志级别：debug, info, warn, error, silent
allow_from = ""                            # 允许的用户 ID，逗号分隔（空则允许所有人）
mode = "team"                              # 智能体模式
```

首次启动时会生成登录二维码，扫码完成授权后自动保存凭证。

## 扩展新平台

通道层支持扩展，只需实现 `channels.Channel` 接口并通过 `channels.RegisterFactory` 注册即可：

```go
func init() {
    channels.RegisterFactory("your_platform", NewChannel)
}
```
