# 公网壳镜像。git-ferry-core 以 Go module 版本依赖（go.mod），无需同级源码目录。
#   docker build -t git-ferry:latest .
#   跨架构(如 ARM64 麒麟):docker buildx build --platform linux/arm64 .
#
# 容器内构建的三个实测要点(每一处都是踩出来的):
#
# 1. GOWORK=off 必须显式给——仓库根的 go.work use 了 ../../go-git-platform 与
#    ../git-ferry-core 两个**同级源码目录**,.dockerignore 已排除 go.work,
#    但宿主若在 workspace 内发起 build,ENV 兜底保证容器内永不解析 go.work。
#
# 2. GOPRIVATE 三家(org 前缀)直连 GitHub:goproxy 对**刚发布**的 tag 有负缓存
#    (404 被缓存),CI 追新版本时走代理会间歇性拉不到;直连最稳。
#    GOPRIVATE 同时跳过 sumdb 校验,自家模块不需要第三方校验和。
#
# 3. 基础镜像经 ARG 可切官方/Daocloud 镜像源:默认官方;本机网络拉不动
#    registry-1.docker.io 时(实测 context deadline exceeded)用
#      --build-arg REGISTRY_MIRROR=docker.m.daocloud.io/library
#    覆盖即可,不改文件。
#
# 注意:镜像内只带 config.example.yaml;部署必须挂载真实 conf/(含 server.api_key),
# 否则启动即 fail-fast——防止把开发默认密钥带进生产。

ARG REGISTRY_MIRROR=
FROM --platform=$BUILDPLATFORM ${REGISTRY_MIRROR}golang:1.27-alpine AS builder

ARG TARGETARCH

WORKDIR /src

RUN apk add --no-cache git

COPY go.mod go.sum ./

# 见顶部说明 1/2:容器内显式配代理与 GOWORK,go mod download 才不挂死。
ENV GOPROXY=https://goproxy.cn,direct \
    GOSUMDB=sum.golang.google.cn \
    GOPRIVATE=github.com/oagentkit/*,github.com/oekit/*,github.com/yi-nology/* \
    GOWORK=off

RUN go mod download

COPY . .

ARG VERSION=docker-dev
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -trimpath \
    -ldflags "-s -w -X github.com/yi-nology/git-ferry/internal/version.Version=${VERSION}" \
    -o /out/git-ferry .

FROM ${REGISTRY_MIRROR}alpine:3.20

WORKDIR /app

RUN apk --no-cache add ca-certificates wget \
    && addgroup -S appgroup && adduser -S appuser -G appgroup

COPY --from=builder /out/git-ferry .
# 只带示例配置:真实配置(api_key 等)由部署方挂载提供
COPY --from=builder /src/conf/config.example.yaml ./conf/config.example.yaml

RUN mkdir -p /app/data && chown appuser:appgroup /app/data

VOLUME /app/data

EXPOSE 8890

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://localhost:8890/health || exit 1

USER appuser

CMD ["./git-ferry"]
