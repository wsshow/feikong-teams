# ---- 构建阶段 ----
FROM golang:1.25-alpine AS builder

WORKDIR /build

# 先复制依赖文件，利用 Docker 缓存
COPY go.mod go.sum ./
RUN go mod download

# 复制源码并编译
COPY . .
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X 'fkteams/version.version=$(cat VERSION 2>/dev/null || echo dev)'" \
    -o fkteams ./main.go

# ---- 运行阶段 ----
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata git

WORKDIR /app

# 从构建阶段复制二进制
COPY --from=builder /build/fkteams .
# 复制并设置 entrypoint 脚本
COPY scripts/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

# 创建运行时目录
RUN mkdir -p config workspace history/input_history history/chat_history \
    scheduler/results sessions log

EXPOSE 23456

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["web"]
