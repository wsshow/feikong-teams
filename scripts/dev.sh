#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

# 独立进程组确保退出时同时关闭 Bun/Vite 等子进程。
set -m
web_pid=
backend_pid=

cleanup() {
    trap '' INT TERM HUP
    for pid in "$web_pid" "$backend_pid"; do
        if [ -n "$pid" ]; then
            kill -TERM -- "-$pid" 2>/dev/null || true
        fi
    done
    for pid in "$web_pid" "$backend_pid"; do
        if [ -n "$pid" ]; then
            wait "$pid" 2>/dev/null || true
        fi
    done
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP

(
    cd web
    exec "${BUN:-bun}" run dev --host 127.0.0.1 --port 5173 --strictPort
) &
web_pid=$!

"${AIR:-./release/dev/air}" -c .air.toml &
backend_pid=$!

echo "前端开发地址: http://127.0.0.1:5173"
echo "后端服务地址: http://127.0.0.1:23456"
echo "按 Ctrl+C 同时停止前后端。"

# 兼容 macOS 自带的 Bash 3；任一服务退出时关闭另一服务并返回其退出码。
while kill -0 "$web_pid" 2>/dev/null && kill -0 "$backend_pid" 2>/dev/null; do
    sleep 1
done

if ! kill -0 "$web_pid" 2>/dev/null; then
    wait "$web_pid"
else
    wait "$backend_pid"
fi
