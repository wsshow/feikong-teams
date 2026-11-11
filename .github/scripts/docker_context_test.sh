#!/usr/bin/env bash
# 用隔离夹具验证本地数据与环境文件不会进入镜像构建上下文。
set -euo pipefail
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT
mkdir -p "$fixture/context/data" "$fixture/context/node_modules" "$fixture/context/web/dist"
cp .dockerignore "$fixture/context/.dockerignore"
printf 'fixture\n' > "$fixture/context/.env"
printf 'fixture\n' > "$fixture/context/.env.local"
printf 'fixture\n' > "$fixture/context/data/state.db"
printf 'fixture\n' > "$fixture/context/node_modules/package.js"
printf 'fixture\n' > "$fixture/context/web/dist/index.html"
printf 'source\n' > "$fixture/context/source.go"
printf 'FROM scratch\nCOPY . /\n' > "$fixture/context/Dockerfile"
docker buildx build --output "type=local,dest=$fixture/output" "$fixture/context"
test -f "$fixture/output/source.go"
for excluded in .env .env.local data node_modules web/dist; do
    test ! -e "$fixture/output/$excluded"
done
