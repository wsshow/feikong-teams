# 部署指南

## 安装发行版

一键安装脚本会下载最新发行版并安装到 `~/.fkteams/bin`，Windows 默认安装到 `%USERPROFILE%\.fkteams\bin`。

Linux / macOS：

```bash
curl -fsSL https://raw.githubusercontent.com/wsshow/feikong-teams/main/install.sh | bash
```

Windows PowerShell：

```powershell
powershell -c "irm https://raw.githubusercontent.com/wsshow/feikong-teams/main/install.ps1 | iex"
```

也可以从 [GitHub Releases](https://github.com/wsshow/feikong-teams/releases) 下载对应平台的压缩包。

如需修改安装目录，请在运行脚本前设置 `FKTEAMS_INSTALL_DIR`：

```bash
# Linux / macOS
export FKTEAMS_INSTALL_DIR=/your/path
```

```powershell
# Windows PowerShell
$env:FKTEAMS_INSTALL_DIR = "D:\fkteams"
```

## 从源码构建

源码构建需要 Go 和 Bun。前端构建产物 `web/dist` 不提交到仓库，Go 编译前需要先生成；Makefile 的构建目标会自动完成 Bun 依赖安装和前端生产构建。

```bash
# 构建当前平台
make native

# 构建指定平台
make build t=linux:amd64

# 构建预设平台（darwin/arm64、windows/amd64、linux/amd64）
make all

# 清理 release/ 和 web/dist
make clean
```

构建产物写入 `release/fkteams_<goos>_<goarch>`，Windows 产物带 `.exe` 后缀。

### 源码开发运行

在 macOS / Linux 上，同时开发前后端只需在仓库根目录执行：

```bash
make dev
```

命令会安装前端依赖、生成 Go 嵌入所需的前端产物，并将固定版本的 Air 安装到 `release/dev/`。首次运行需要联网下载依赖，无需全局安装 Air。

访问 <http://127.0.0.1:5173> 开发页面：Vite 提供前端热更新，并将 API 和 WebSocket 代理到 `127.0.0.1:23456`。Air 监听 `cmd/`、`internal/` 下的 Go 源码与提示词模板，以及 `go.mod`、`go.sum` 和 `web/embed.go`；编译成功后自动重启后端，编译失败时保留上一次成功启动的服务。后端重启会中断正在执行的请求。

按 `Ctrl+C` 同时停止前后端；任一开发进程退出时也会停止另一进程。端口被占用时请先停止已有服务，开发命令不会主动终止占用端口的其他进程。

也可以分别在两个终端运行：

```bash
make dev-web       # 仅启动 Vite 热更新服务器
make dev-backend   # 仅启动 Air 自动编译重启后端
```

开发服务沿用 `~/.fkteams` 中的应用配置和数据，也可通过 `FEIKONG_APP_DIR` 指定其他目录。开发工具、二进制和日志均位于不提交的 `release/` 下，`make clean` 会将其清理。

以下命令用于不启用自动重启的源码运行：

```bash
# 生成嵌入 Go 二进制的前端产物
make web-build

# 启动 Web 服务
go run ./cmd/fkteams web

# 启动 CLI
go run ./cmd/fkteams

# 启动纯 API 服务
go run ./cmd/fkteams serve
```

### 前端开发

```bash
cd web
bun install
bun run dev
```

前端开发服务器用于热更新；需要从 Go Web 服务验证完整嵌入式产物时，重新执行 `make web-build`。

## Docker 部署

官方镜像为 [`fkteams/fkteams`](https://hub.docker.com/r/fkteams/fkteams)，支持 `linux/amd64` 和 `linux/arm64`，Docker 会自动选择当前主机的架构。正式版本提供 `v主版本.次版本.补丁版本` 标签，`latest` 跟随最新正式版本；生产部署可固定版本标签。

镜像基于 Ubuntu 24.04，预装 Bash、Git/SSH 客户端、curl/wget、压缩解压工具、jq、ripgrep、进程与网络诊断工具，以及 GCC/G++/Make、Python 3.12/venv/pip、uv/uvx、Bun/bunx、Node.js 24/npm/npx。默认以容器内 root 运行，智能体可通过命令工具按需安装系统软件；应用已有的路径限制、危险命令检查和审批策略仍然生效。

### 使用 Docker Compose（推荐）

仓库的 `docker-compose.yml` 默认拉取已发布镜像，并将宿主机 `./data` 整体挂载到容器 `/app`。

```bash
mkdir -p data
docker compose pull

# 仅在首次使用时生成配置，已有配置请保留
docker compose run --rm fkteams generate config
```

编辑 `data/config/config.toml` 配置模型后启动：

```bash
docker compose up -d
```

访问 <http://localhost:23456> 即可使用。升级镜像时执行 `docker compose pull && docker compose up -d`。

从旧版分目录挂载的 Compose 配置升级时，先停止旧容器并备份数据。旧配置未挂载的 `/app/data`（SQLite 数据库）和 `/app/javascript` 可能仍在旧容器内，需要在删除旧容器前复制到宿主机 `./data/data` 和 `./data/javascript`；仅复制原来挂载的几个子目录不足以保存全部状态。

### 使用 docker run

```bash
mkdir -p data
docker run --rm -v "$(pwd)/data:/app" \
  fkteams/fkteams:latest generate config

# 配置 data/config/config.toml 后启动
docker run -d \
  --name fkteams \
  --restart unless-stopped \
  -p 23456:23456 \
  -v "$(pwd)/data:/app" \
  fkteams/fkteams:latest
```

镜像默认使用 `web --host 0.0.0.0` 启动，`FEIKONG_APP_DIR` 默认为 `/app`。程序位于 `/usr/local/bin/fkteams`，整体挂载数据目录不会遮挡程序。环境变量可通过 Compose 的 `environment` 或 `docker run -e` 传入。

### 智能体安装软件与数据持久化

系统软件通过 `apt-get update && apt-get install -y <软件包>` 安装。Python 项目使用 uv 或虚拟环境，避免直接覆盖 Ubuntu 管理的系统 Python 包，例如：

```bash
uv venv /app/workspace/.venv
uv pip install --python /app/workspace/.venv/bin/python requests
```

- `/app` 数据卷保存配置、SQLite 数据库、工作区、会话附件、Skills 和扩展状态；放在工作区内的项目依赖也随数据卷保留。
- 同一容器停止再启动后，临时安装的软件仍然存在。
- 升级镜像并重建容器后，通过 apt 安装的系统软件及未挂载目录内的全局依赖需要重新安装。可维护项目依赖清单与安装脚本，或将固定系统依赖写入自己的派生 Dockerfile。

例如固定安装额外工具：

```dockerfile
FROM fkteams/fkteams:latest
RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg \
    && rm -rf /var/lib/apt/lists/*
```

### 本地构建与验证

```bash
docker build --build-arg VERSION=dev \
  --build-arg BUILD_TIME=local -t fkteams:local .

# 在对应架构主机执行（amd64 主机将 arm64 改为 amd64）
bash .github/scripts/docker_smoke.sh fkteams:local arm64 dev local
```

构建阶段使用原生架构构建前端并交叉编译 Go；运行阶段使用目标架构的 Ubuntu、Node.js、Bun 和 uv。Dockerfile 中固定了 Bun、uv、Node.js 版本，可通过 `BUN_VERSION`、`UV_VERSION`、`NODE_VERSION` 构建参数显式升级。

默认 Go 依赖代理为 `https://proxy.golang.org,direct`。网络受限时可通过 `--build-arg GOPROXY=https://goproxy.cn,direct` 指定可访问的代理。

### 自动发布到 Docker Hub

在 GitHub 仓库 **Settings → Secrets and variables → Actions** 中配置：

| 类型 | 名称 | 内容 |
| --- | --- | --- |
| Repository variable | `DOCKERHUB_USERNAME` | 对 `fkteams/fkteams` 有写权限的 Docker Hub 登录账号 |
| Repository secret | `DOCKERHUB_TOKEN` | 该账号有读写权限的 Docker Hub access token |

登录账号可以与镜像命名空间不同。令牌只保存到 GitHub Secrets，不写入源码、镜像或构建参数。

将包含 workflow 的提交推送到 GitHub 后，创建并推送版本 tag 即会触发 `.github/workflows/docker.yml`：

```bash
git tag v0.30.0
git push origin v0.30.0
```

发布流程：

1. 校验 tag 格式，执行 `make check`、前端测试、发布策略测试和构建上下文隔离测试。
2. 在 `ubuntu-24.04`、`ubuntu-24.04-arm` 原生 runner 上分别构建镜像，并上传尚无版本标签的候选 digest。
3. 两种架构分别验证系统软件安装、Python/JavaScript 包安装、编译工具、版本信息、Web 页面及数据卷重建恢复。
4. 两边都成功后，将通过测试的 digest 合并为双架构镜像，发布 `fkteams/fkteams:v0.30.0`。

`v0.30.0-rc.1` 等预发布只更新自己的版本标签。只有当前远端 Git tags 中版本号最高的正式版本才更新 `latest`，补发较旧版本不会回退它。该判断以 Git tag 为准：若更高正式版本已打 tag 但构建失败，较旧版本也不会接管 `latest`；修复并重跑更高版本的 workflow 即可。

tag 格式为 `vMAJOR.MINOR.PATCH[-PRERELEASE]`，不支持 Docker 标签无法表示的 `+build` 后缀。失败后可在 Actions 中重跑；镜像发布与现有 GoReleaser 二进制发布独立触发，各自显示执行结果。版本标签各自发布，只有 `latest` 更新串行执行；短时间推送多个正式版本时应确认最高版本的 `latest` job 成功，必要时重跑被 GitHub 等待队列替换的 job。
