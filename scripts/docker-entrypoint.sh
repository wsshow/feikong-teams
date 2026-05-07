#!/bin/sh
set -e

# 确保导出环境变量，使 fkteams 二进制文件和脚本使用一致的路径
export FEIKONG_APP_DIR="${FEIKONG_APP_DIR:-/app}"
CONFIG_FILE="$FEIKONG_APP_DIR/config/config.toml"

# 1. 如果配置文件不存在，则生成默认配置
if [ ! -f "$CONFIG_FILE" ]; then
    echo "[Entrypoint] 配置文件未找到，正在生成默认配置: $CONFIG_FILE"
    ./fkteams generate config
fi

# 2. 自动将 HOST 从 127.0.0.1 替换为 0.0.0.0 以允许外部访问（Docker 环境必须）
if [ -f "$CONFIG_FILE" ]; then
    echo "[Entrypoint] 检查并修复监听地址 (127.0.0.1 -> 0.0.0.0)..."
    # 修复范围匹配：从 [server] 开始，找到第一个 host 行并替换 IP
    sed -i '/\[server\]/,/host =/ s/127\.0\.0\.1/0.0.0.0/' "$CONFIG_FILE"
fi

# 3. 执行原始命令
echo "[Entrypoint] 启动应用: $@"
exec ./fkteams "$@"
