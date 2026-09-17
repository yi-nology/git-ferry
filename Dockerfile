# 公网壳镜像。git-sync-core 以 Go module 版本依赖（go.mod），无需同级源码目录。
#   docker build -t git-ferry:latest .

FROM golang:1.26-alpine AS builder

WORKDIR /src

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=docker-dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags "-s -w -X github.com/yi-nology/git-ferry/internal/version.Version=${VERSION}" \
    -o /out/git-ferry .

FROM alpine:3.20

WORKDIR /app

RUN apk --no-cache add ca-certificates wget \
    && addgroup -S appgroup && adduser -S appuser -G appgroup

COPY --from=builder /out/git-ferry .
COPY --from=builder /src/conf ./conf

RUN mkdir -p /app/data && chown appuser:appgroup /app/data

VOLUME /app/data

EXPOSE 8890

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://localhost:8890/health || exit 1

USER appuser

CMD ["./git-ferry"]
