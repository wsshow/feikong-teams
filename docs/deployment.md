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

### 使用 docker-compose（推荐）

1. 编辑 `docker-compose.yml`，确保数据目录挂载正确。

2. 首次启动前，生成示例配置文件：

```bash
mkdir -p data
# 启动一次容器以生成默认配置
docker run --rm \
  -e FEIKONG_APP_DIR=/app \
  -v ./data:/app \
  fkteams generate config
```

3. 启动服务：

```bash
docker compose up -d
```

访问 http://localhost:23456 即可使用。

### 使用 docker run

```bash
# 构建镜像
docker build -t fkteams .

# 运行容器
docker run -d \
  --name fkteams \
  -p 23456:23456 \
  -e FEIKONG_APP_DIR=/app \
  -v ./data/config:/app/config \
  -v ./data/workspace:/app/workspace \
  -v ./data/scheduler:/app/scheduler \
  -v ./data/history:/app/history \
  -v ./data/sessions:/app/sessions \
  -v ./data/share:/app/share \
  -v ./data/log:/app/log \
  fkteams
```

### 说明

- 环境变量通过 `docker-compose.yml` 的 `environment` 或 `docker run -e` 传入，无需 `.env` 文件
- **`FEIKONG_APP_DIR=/app`**：将应用数据目录设置为容器内的 `/app`，与 volume 挂载路径对应
- `config/config.toml` 通过 volume 挂载，可在容器外编辑
- 数据目录（workspace、scheduler、history、sessions、share 等）建议挂载到宿主机以持久化
