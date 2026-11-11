#!/usr/bin/env bash
# 在目标架构验证软件安装、运行时、Web 启动与数据卷。
set -euo pipefail

image=${1:?usage: docker_smoke.sh IMAGE ARCH VERSION BUILD_TIME}
arch=${2:?missing architecture}
version=${3:?missing version}
build_time=${4:?missing build time}

docker run --rm -i --platform "linux/$arch" --entrypoint bash \
    -e EXPECTED_ARCH="$arch" -e EXPECTED_VERSION="$version" -e EXPECTED_BUILD_TIME="$build_time" \
    "$image" -se <<'CONTAINER'
set -euo pipefail
. /etc/os-release
test "$ID" = ubuntu
test "$VERSION_ID" = 24.04
test "$(id -u)" = 0
test "$(dpkg --print-architecture)" = "$EXPECTED_ARCH"
test "$FEIKONG_APP_DIR" = /app
for tool in bash curl wget git ssh unzip zip xz tar gcc g++ make pkg-config \
    python python3 pip3 uv uvx bun bunx node npm npx jq rg less nano ps rsync ip ss ping dig; do
    command -v "$tool"
done
fkteams --version | grep -F "$EXPECTED_VERSION ($EXPECTED_BUILD_TIME)"

# 真正安装一个系统包，确认包管理器、网络与写权限均可用。
apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends tree
tree --version

printf 'int main(void) { return 0; }\n' | gcc -x c -o /tmp/compiler-smoke -
/tmp/compiler-smoke
python3 -m venv /tmp/python-smoke
/tmp/python-smoke/bin/python -m pip --version
uv venv --python /usr/bin/python3 /tmp/uv-smoke
uv pip install --python /tmp/uv-smoke/bin/python six==1.17.0
/tmp/uv-smoke/bin/python -c 'import six; assert six.__version__ == "1.17.0"'
mkdir -p /tmp/bun-smoke
cd /tmp/bun-smoke
bun add is-number@7.0.0
bun -e 'if (!require("is-number")(42)) process.exit(1)'
npm install --prefix /tmp/npm-smoke is-number@7.0.0
node -e 'if (!require("/tmp/npm-smoke/node_modules/is-number")(42)) process.exit(1)'
npx --version
CONTAINER

container=
volume=$(docker volume create)
cleanup() {
    status=$?
    if [[ -n "$container" ]]; then
        if [[ "$status" -ne 0 ]]; then docker logs "$container"; fi
        docker rm -f "$container" >/dev/null
    fi
    docker volume rm "$volume" >/dev/null
    exit "$status"
}
trap cleanup EXIT

# 整体挂载空数据卷不能遮挡程序，生成的配置需要在下一容器中可用。
docker run --rm --platform "linux/$arch" -v "$volume:/app" "$image" generate config
container=$(docker run -d --platform "linux/$arch" -v "$volume:/app" "$image")
wait_ready() {
    for ((attempt = 0; attempt < 60; attempt++)); do
        if docker exec "$container" curl -fsS http://127.0.0.1:23456/health >/dev/null; then
            return 0
        fi
        sleep 1
    done
    return 1
}
wait_ready
docker exec "$container" curl -fsS http://127.0.0.1:23456/ | grep -F '<div id="root">'
docker exec "$container" test -s /app/config/config.toml
docker exec "$container" test -s /app/data/fkteams.db
docker exec "$container" touch /app/workspace/persistence-smoke

# 重建容器后验证应用与数据卷，避免只覆盖同一容器的 restart。
docker stop --time 20 "$container" >/dev/null
docker rm "$container" >/dev/null
container=$(docker run -d --platform "linux/$arch" -v "$volume:/app" "$image")
wait_ready
docker exec "$container" test -f /app/workspace/persistence-smoke
docker exec "$container" curl -fsS http://127.0.0.1:23456/api/fkteams/version \
    | jq -e --arg version "$version" --arg time "$build_time" \
        '.data.version == $version and .data.buildDate == $time'
