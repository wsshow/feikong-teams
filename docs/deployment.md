# 部署指南

## 构建

```bash
# 清理构建产物
make clean

# 构建当前平台
make build

# 修改 Makefile 中的 os-archs 变量以支持其他平台
# 例如：os-archs=darwin:arm64 linux:amd64 windows:amd64
```

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
  fkteams fkteams generate config
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
  -v ./data/history:/app/history \
  -v ./data/sessions:/app/sessions \
  -v ./data/result:/app/result \
  -v ./data/log:/app/log \
  fkteams
```

### 说明

- 环境变量通过 `docker-compose.yml` 的 `environment` 或 `docker run -e` 传入，无需 `.env` 文件
- **`FEIKONG_APP_DIR=/app`**：将应用数据目录设置为容器内的 `/app`，与 volume 挂载路径对应
- `config/config.toml` 通过 volume 挂载，可在容器外编辑
- 数据目录（workspace、history、sessions 等）建议挂载到宿主机以持久化
