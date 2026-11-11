# ---- 前端构建阶段 ----
FROM oven/bun:1-alpine AS web-builder

WORKDIR /src/web

COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile

COPY web ./
RUN bun run build

# ---- Go 构建阶段 ----
FROM golang:1.26.11-alpine AS builder

WORKDIR /build

# 先复制依赖文件，利用 Docker 缓存
COPY go.mod go.sum ./
RUN go mod download

# 复制源码并编译
COPY . .
COPY --from=web-builder /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X 'fkteams/internal/app/version.version=$(cat VERSION 2>/dev/null || echo dev)'" \
    -o fkteams ./cmd/fkteams

# ---- 运行阶段 ----
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata git

WORKDIR /app

# 从构建阶段复制二进制
COPY --from=builder /build/fkteams .

# 创建运行时目录
RUN mkdir -p config workspace history/input_history history/chat_history \
    scheduler/results sessions share skills runtime log

EXPOSE 23456

ENTRYPOINT ["./fkteams"]
CMD ["web", "--host", "0.0.0.0"]
