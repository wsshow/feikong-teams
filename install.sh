#!/usr/bin/env bash
# fkteams 安装脚本
# 用法: curl -fsSL https://raw.githubusercontent.com/wsshow/feikong-teams/main/install.sh | bash

set -euo pipefail

GITHUB_REPO="wsshow/feikong-teams"
APP_NAME="fkteams"
INSTALL_DIR="${FKTEAMS_INSTALL_DIR:-${HOME}/fkteams}"

# ---- 颜色输出 ----
tty_escape() { printf "\033[%sm" "$1"; }
tty_mkbold()  { tty_escape "1;$1"; }
tty_blue="$(tty_mkbold 34)"
tty_green="$(tty_mkbold 32)"
tty_yellow="$(tty_mkbold 33)"
tty_red="$(tty_mkbold 31)"
tty_bold="$(tty_mkbold 39)"
tty_reset="$(tty_escape 0)"

info()    { printf "${tty_blue}==>${tty_reset} ${tty_bold}%s${tty_reset}\n" "$*"; }
success() { printf "${tty_green}==>${tty_reset} ${tty_bold}%s${tty_reset}\n" "$*"; }
warn()    { printf "${tty_yellow}警告${tty_reset}: %s\n" "$*" >&2; }
abort()   { printf "${tty_red}错误${tty_reset}: %s\n" "$*" >&2; exit 1; }

# ---- 检测操作系统 ----
detect_os() {
    local os
    os="$(uname -s)"
    case "$os" in
        Linux)  echo "Linux" ;;
        Darwin) echo "Darwin" ;;
        *)      abort "不支持的操作系统: $os (仅支持 Linux 和 macOS)" ;;
    esac
}

# ---- 检测 CPU 架构 ----
detect_arch() {
    local arch
    arch="$(uname -m)"
    case "$arch" in
        x86_64|amd64) echo "x86_64" ;;
        arm64|aarch64) echo "arm64" ;;
        *)             abort "不支持的 CPU 架构: $arch (仅支持 x86_64 和 arm64)" ;;
    esac
}

# ---- 获取最新版本号 ----
get_latest_version() {
    local api_url="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"
    local tag

    if command -v curl &>/dev/null; then
        tag="$(curl -fsSL "$api_url" | grep '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')"
    elif command -v wget &>/dev/null; then
        tag="$(wget -qO- "$api_url" | grep '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')"
    else
        abort "需要 curl 或 wget 才能下载，请先安装其中之一"
    fi

    if [ -z "$tag" ]; then
        abort "无法获取最新版本信息，请检查网络连接"
    fi
    echo "$tag"
}

# ---- 下载文件（自定义进度条 + 断点续传 + 重试）----
download() {
    local url="$1" dest="$2"
    local max_retries=5 retry_delay=3 attempt=0
    local bar_width=40
    local spin_chars=( '|' '/' '-' '\' )

    # 预先获取文件总大小（-L 跟随 GitHub 302 重定向至 CDN）
    local total_size=0
    if command -v curl &>/dev/null; then
        total_size=$(curl -fsSIL --max-time 10 "$url" 2>/dev/null \
            | grep -i '^content-length:' \
            | awk '{gsub(/\r/,""); print $2}' | tail -1)
    elif command -v wget &>/dev/null; then
        total_size=$(wget --spider --server-response -q "$url" 2>&1 \
            | grep -i 'Content-Length:' \
            | awk '{print $2}' | tail -1)
    fi
    # 确保是纯数字
    case "${total_size:-x}" in
        *[!0-9]*|"") total_size=0 ;;
    esac

    while [ "$attempt" -lt "$max_retries" ]; do
        attempt=$(( attempt + 1 ))

        # 后台下载（支持断点续传）
        if command -v curl &>/dev/null; then
            curl -fsSL --continue-at - "$url" -o "$dest" &
        elif command -v wget &>/dev/null; then
            wget --continue -q "$url" -O "$dest" &
        else
            abort "需要 curl 或 wget 才能下载"
        fi
        local dl_pid=$!

        # 自定义进度条：每 0.3s 轮询文件大小
        local spin_idx=0 cur_mb tot_mb pct filled bar i
        while kill -0 "$dl_pid" 2>/dev/null; do
            local current=0
            if [ -f "$dest" ]; then
                current=$(wc -c < "$dest" 2>/dev/null | awk '{print $1}') || current=0
            fi
            current="${current:-0}"
            case "${current}" in *[!0-9]*) current=0 ;; esac

            cur_mb=$(awk "BEGIN{printf \"%.1f\", ${current}/1048576}" 2>/dev/null || echo "?")

            if [ "$total_size" -gt 0 ] 2>/dev/null; then
                pct=$(( current * 100 / total_size ))
                [ "$pct" -gt 100 ] && pct=100
                filled=$(( pct * bar_width / 100 ))

                bar="" i=0
                while [ "$i" -lt "$filled" ]; do bar="${bar}#"; i=$(( i+1 )); done
                while [ "$i" -lt "$bar_width" ]; do bar="${bar}-"; i=$(( i+1 )); done

                tot_mb=$(awk "BEGIN{printf \"%.1f\", ${total_size}/1048576}" 2>/dev/null || echo "?")
                printf "\r  [%s] %3d%%  %s MB / %s MB" "$bar" "$pct" "$cur_mb" "$tot_mb" >&2
            else
                # 总大小未知：旋转动画 + 已下载量
                printf "\r  %s  %s MB 已下载" "${spin_chars[$(( spin_idx % 4 ))]}" "$cur_mb" >&2
                spin_idx=$(( spin_idx + 1 ))
            fi
            sleep 0.3
        done

        wait "$dl_pid"
        local exit_code=$?
        printf "\n" >&2

        if [ "$exit_code" -eq 0 ]; then
            return 0
        fi

        if [ "$attempt" -lt "$max_retries" ]; then
            warn "下载出错（第 ${attempt}/${max_retries} 次），${retry_delay}s 后重试..."
            sleep "$retry_delay"
        fi
    done

    abort "下载失败，已重试 ${max_retries} 次，请检查网络连接"
}

# ---- SHA256 校验 ----
compute_sha256() {
    local file="$1"
    if command -v sha256sum &>/dev/null; then
        sha256sum "$file" | awk '{print $1}'
    elif command -v shasum &>/dev/null; then
        shasum -a 256 "$file" | awk '{print $1}'
    else
        # 无法校验时给出警告，不阻断安装
        warn "未找到 sha256sum 或 shasum，跳过完整性校验"
        echo "skip"
    fi
}

verify_checksum() {
    local checksums_file="$1"
    local zip_file="$2"
    local zip_name
    zip_name="$(basename "$zip_file")"

    local expected
    expected="$(grep " ${zip_name}$" "$checksums_file" | awk '{print $1}')"

    if [ -z "$expected" ]; then
        warn "checksums.txt 中未找到 ${zip_name} 的校验值，跳过校验"
        return
    fi

    local actual
    actual="$(compute_sha256 "$zip_file")"

    if [ "$actual" = "skip" ]; then
        return
    fi

    if [ "$actual" != "$expected" ]; then
        abort "SHA256 校验失败！\n  期望: ${expected}\n  实际: ${actual}\n文件可能已损坏，请重试"
    fi

    success "SHA256 校验通过"
}

# ---- 更新 Shell 配置文件，将安装目录加入 PATH ----
add_to_path() {
    local dir="$1"

    # 当前 session 已在 PATH 中则跳过
    if [[ ":${PATH}:" == *":${dir}:"* ]]; then
        info "${dir} 已在 PATH 中，无需修改"
        return
    fi

    local shell_name profile_file
    shell_name="$(basename "${SHELL:-sh}")"

    case "$shell_name" in
        bash)
            if [ -f "${HOME}/.bash_profile" ]; then
                profile_file="${HOME}/.bash_profile"
            else
                profile_file="${HOME}/.bashrc"
            fi
            ;;
        zsh)
            profile_file="${HOME}/.zshrc"
            ;;
        fish)
            profile_file="${HOME}/.config/fish/config.fish"
            ;;
        *)
            profile_file="${HOME}/.profile"
            ;;
    esac

    local export_line
    if [ "$shell_name" = "fish" ]; then
        export_line="fish_add_path ${dir}"
    else
        export_line="export PATH=\"\${PATH}:${dir}\""
    fi

    # 幂等写入：若已有该行则不重复添加
    if ! grep -qF "$dir" "$profile_file" 2>/dev/null; then
        {
            printf '\n# fkteams\n'
            echo "$export_line"
        } >> "$profile_file"
        success "已将 ${dir} 添加到 PATH（${profile_file}）"
        warn "请重启终端，或执行: source ${profile_file}"
    else
        info "${dir} 已在 ${profile_file} 中配置"
    fi
}

# ---- 主流程 ----
main() {
    local os arch tag version zip_name download_url

    os="$(detect_os)"
    arch="$(detect_arch)"

    info "正在获取最新版本..."
    tag="$(get_latest_version)"
    # GoReleaser 打包时去掉了 v 前缀
    version="${tag#v}"

    zip_name="feikong-teams_${version}_${os}_${arch}.zip"
    download_url="https://github.com/${GITHUB_REPO}/releases/download/${tag}/${zip_name}"

    info "版本   : ${tag}"
    info "平台   : ${os}/${arch}"
    info "安装目录: ${INSTALL_DIR}"

    # 创建临时目录（tmp_dir 不能用 local，否则 EXIT trap 无法访问）
    tmp_dir="$(mktemp -d)"
    trap 'rm -rf "${tmp_dir:-}"' EXIT

    # 下载
    info "正在下载 ${zip_name}..."
    download "$download_url" "${tmp_dir}/${zip_name}"

    # 下载 checksums.txt 并校验
    checksums_url="https://github.com/${GITHUB_REPO}/releases/download/${tag}/checksums.txt"
    info "正在验证文件完整性..."
    download "$checksums_url" "${tmp_dir}/checksums.txt"
    verify_checksum "${tmp_dir}/checksums.txt" "${tmp_dir}/${zip_name}"

    # 检查 unzip 是否可用
    if ! command -v unzip &>/dev/null; then
        abort "需要 unzip 才能解压，请先安装: sudo apt install unzip 或 brew install unzip"
    fi

    # 创建安装目录
    mkdir -p "${INSTALL_DIR}"

    # 解压（直接将文件解压到安装目录，覆盖已有文件）
    info "正在解压..."
    unzip -q -o "${tmp_dir}/${zip_name}" "${APP_NAME}" -d "${INSTALL_DIR}" 2>/dev/null || \
        unzip -q -o "${tmp_dir}/${zip_name}" -d "${INSTALL_DIR}"

    # 确保二进制有执行权限
    chmod +x "${INSTALL_DIR}/${APP_NAME}"

    success "${APP_NAME} 已安装至 ${INSTALL_DIR}/${APP_NAME}"

    # 将安装目录加入 PATH
    add_to_path "${INSTALL_DIR}"

    success "安装完成！运行 '${APP_NAME} --version' 验证安装。"
}

main "$@"
