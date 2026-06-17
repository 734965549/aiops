# 多阶段构建：第一阶段编译 Go 二进制，第二阶段运行轻量镜像。

FROM golang:1.22-alpine AS builder
WORKDIR /workspace

# 优先复制 go.mod / go.sum 以充分利用 layer 缓存。
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_AT=unknown

ENV CGO_ENABLED=0 GOOS=linux

RUN go build -trimpath \
    -ldflags "-s -w \
      -X 'github.com/734965549/aiops/internal/version.Version=${VERSION}' \
      -X 'github.com/734965549/aiops/internal/version.Commit=${COMMIT}' \
      -X 'github.com/734965549/aiops/internal/version.BuildAt=${BUILD_AT}'" \
    -o /out/aiops-api ./cmd/api

FROM alpine:3.20
# wget 用于 HEALTHCHECK；tini 用于正确转发 SIGTERM 给 Go 进程触发优雅退出。
RUN apk --no-cache add ca-certificates tzdata wget tini && \
    cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo "Asia/Shanghai" > /etc/timezone && \
    addgroup -S aiops && adduser -S -G aiops aiops

WORKDIR /app
COPY --from=builder /out/aiops-api /app/aiops-api
COPY migrations /app/migrations

USER aiops

EXPOSE 8080

# 容器内默认健康检查：/readyz 须返回 data.status=ready（含 DB/迁移/Redis 等依赖）。
HEALTHCHECK --interval=15s --timeout=5s --start-period=30s --retries=5 \
  CMD wget -qO- http://127.0.0.1:8080/readyz | grep -q '"status":"ready"' || exit 1

ENTRYPOINT ["/sbin/tini", "--", "/app/aiops-api"]
