# syntax=docker/dockerfile:1

# Builder: 使用官方 Go 镜像编译静态二进制，减小运行时镜像体积。
FROM golang:1.25-alpine AS builder

WORKDIR /src

# 先复制依赖清单，提升 Docker 层缓存命中率。
COPY go.mod go.sum ./
RUN go mod download

# 再复制源码并编译。
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/go-aria2 ./cmd/go-aria2

# Runtime: 只保留运行 go-aria2 所需的最小环境。
FROM alpine:3

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# 创建非 root 用户，避免下载服务以 root 运行。
RUN addgroup -S goaria2 && adduser -S -G goaria2 -h /app goaria2 \
	&& mkdir -p /config /data /downloads \
	&& chown -R goaria2:goaria2 /app /config /data /downloads

COPY --from=builder /out/go-aria2 /usr/local/bin/go-aria2

USER goaria2

# 常用暴露端口：
# 16800/tcp JSON-RPC
# 6881/tcp,6881/udp BitTorrent
# 4662/tcp ED2K
# 4661/udp ED2K/Kad
EXPOSE 16800/tcp 6881/tcp 6881/udp 4662/tcp 4661/udp

VOLUME ["/config", "/data", "/downloads"]

# 默认从挂载进来的 aria2.conf 启动守护进程。
ENTRYPOINT ["go-aria2"]
CMD ["daemon", "-conf", "/config/aria2.conf"]
