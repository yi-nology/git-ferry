# 公网壳镜像。构建上下文为父目录（需含同级 git-sync-core）：
#   my_project/
#     git-sync-core/
#     git-sync-service/
#   docker build -f git-sync-service/Dockerfile -t git-sync-service:latest .

FROM golang:1.26-alpine AS builder

WORKDIR /src

RUN apk add --no-cache git gcc musl-dev

COPY git-sync-core/go.mod git-sync-core/go.sum ./git-sync-core/
COPY git-sync-service/go.mod git-sync-service/go.sum ./git-sync-service/

RUN go mod download

COPY git-sync-core ./git-sync-core
COPY git-sync-service ./git-sync-service

WORKDIR /src/git-sync-service

ARG VERSION=docker-dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags "-s -w -X github.com/yi-nology/git-sync-service/internal/version.Version=${VERSION}" \
    -o /out/git-sync-service .

FROM alpine:3.20

WORKDIR /app

RUN apk --no-cache add ca-certificates wget \
    && addgroup -S appgroup && adduser -S appuser -G appgroup

COPY --from=builder /out/git-sync-service .
COPY --from=builder /src/git-sync-service/conf ./conf

RUN mkdir -p /app/data && chown appuser:appgroup /app/data

VOLUME /app/data

EXPOSE 8890

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://localhost:8890/health || exit 1

USER appuser

CMD ["./git-sync-service"]
