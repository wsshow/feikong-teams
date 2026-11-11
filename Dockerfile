# syntax=docker/dockerfile:1

ARG BUN_VERSION=1.4.4
ARG UV_VERSION=0.13.7
ARG NODE_VERSION=24.22.2

# ---- 前端构建阶段：始终使用构建机架构 ----
FROM --platform=$BUILDPLATFORM oven/bun:${BUN_VERSION}-debian AS web-builder

WORKDIR /src/web

COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile

COPY web ./
RUN bun run build

# ---- Go 构建阶段 ----
FROM --platform=$BUILDPLATFORM golang:1.26.11-alpine AS builder

WORKDIR /build

# 先复制依赖文件，利用 Docker 缓存
ARG GOPROXY=https://proxy.golang.org,direct
COPY go.mod go.sum ./
RUN go mod download

# 复制源码并编译
COPY . .
COPY --from=web-builder /src/web/dist ./web/dist
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath \
    -ldflags "-s -w -X 'fkteams/internal/app/version.version=${VERSION}' -X 'fkteams/internal/app/version.buildTime=${BUILD_TIME}'" \
    -o fkteams ./cmd/fkteams

# ---- 目标架构的语言运行时 ----
FROM oven/bun:${BUN_VERSION}-debian AS bun-runtime
FROM ghcr.io/astral-sh/uv:${UV_VERSION} AS uv-runtime
FROM node:${NODE_VERSION}-bookworm-slim AS node-runtime

# ---- 智能体工作环境 ----
FROM ubuntu:24.04

# 保留包管理器与编译工具，允许智能体在容器内按需安装依赖。
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    bash ca-certificates tzdata curl wget git openssh-client \
    unzip zip xz-utils tar gzip bzip2 \
    build-essential pkg-config libssl-dev libffi-dev \
    python3 python-is-python3 python3-venv python3-pip python3-dev \
    jq ripgrep less nano procps psmisc file rsync iproute2 iputils-ping dnsutils \
    && rm -rf /var/lib/apt/lists/*

COPY --from=bun-runtime /usr/local/bin/bun /usr/local/bin/bun
COPY --from=uv-runtime /uv /uvx /usr/local/bin/
COPY --from=node-runtime /usr/local/bin/node /usr/local/bin/node
COPY --from=node-runtime /usr/local/lib/node_modules/npm /usr/local/lib/node_modules/npm
RUN ln -s bun /usr/local/bin/bunx \
    && ln -s ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
    && ln -s ../lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx

ENV FEIKONG_APP_DIR=/app \
    LANG=C.UTF-8 \
    PATH=/root/.local/bin:/root/.bun/bin:${PATH}

WORKDIR /app

# 程序与数据分离，整体挂载 /app 时不会遮挡可执行文件。
COPY --from=builder /build/fkteams /usr/local/bin/fkteams

# 创建运行时目录
RUN mkdir -p config workspace history/input_history history/chat_history \
    scheduler/results sessions share skills runtime log data javascript

# 系统包安装需要容器内 root；应用的工具权限与审批仍按原有策略执行。
USER root

EXPOSE 23456

ENTRYPOINT ["fkteams"]
CMD ["web", "--host", "0.0.0.0"]
