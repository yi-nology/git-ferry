# Git Sync Service

[![CI](https://github.com/yi-nology/git-sync-service/actions/workflows/ci.yml/badge.svg)](https://github.com/yi-nology/git-sync-service/actions/workflows/ci.yml)
[![Release](https://github.com/yi-nology/git-sync-service/actions/workflows/release.yml/badge.svg)](https://github.com/yi-nology/git-sync-service/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/yi-nology/git-sync-service)](https://goreportcard.com/report/github.com/yi-nology/git-sync-service)
[![Go Version](https://img.shields.io/github/go-mod/go-version/yi-nology/git-sync-service)](https://go.dev/)
[![License](https://img.shields.io/github/license/yi-nology/git-sync-service)](LICENSE)
[![Latest Release](https://img.shields.io/github/v/release/yi-nology/git-sync-service)](https://github.com/yi-nology/git-sync-service/releases/tag/v1.10.0)

A service for synchronizing Git repositories across different platforms.

## Features

- Multi-platform Git synchronization
- Scheduled sync with cron support
- Webhook-based real-time sync
- RESTful API for manual operations
- SQLite and MySQL database support

## Related repositories

本服务是三仓架构中的**公网壳**，以 Go module 版本依赖引擎库：

| Repository | Import path | Role |
|------------|-------------|------|
| [git-sync-core](https://github.com/yi-nology/git-sync-core) | `github.com/yi-nology/git-sync-core` | Sync engine library (no HTTP)，当前 `v0.1.0` |
| **git-sync-service**（本仓） | `github.com/yi-nology/git-sync-service` | Public shell: hz API + Vue UI |
| [git-sync-intranet](https://github.com/yi-nology/git-sync-intranet) | `github.com/yi-nology/git-sync-intranet` | Intranet shell (gateway/SSO auth) |

单仓即可构建（`git clone` 后 `go build`），无需同级 checkout。

本地联调未发布的 core 时，可临时：

```bash
# 可选：本地 workspace，不提交 go.work
go work init . ../git-sync-core
```

### Intranet auth env

见 [git-sync-intranet](https://github.com/yi-nology/git-sync-intranet)：`INTRANET_AUTH_MODE` / `INTRANET_AUTH_USER_HEADER` / `INTRANET_AUTH_ALLOW_EMPTY`。

### Build image

```bash
make docker-build
```

## Installation

### From Release

Download the latest binary from [Releases](https://github.com/yi-nology/git-sync-service/releases).

### From Source

```bash
git clone https://github.com/yi-nology/git-sync-service.git
cd git-sync-service
go build -o git-sync-service .
```

## Usage

```bash
# Run the service
./git-sync-service

# Or use make
make run
```

## Configuration

### Environment Variables

The following environment variables are required:

| Variable | Required | Description |
|----------|----------|-------------|
| `ENCRYPTION_KEY` | Yes | Encryption key for credential storage (AES-256-GCM). Min 1 byte; keys shorter than 32 bytes are hashed via SHA-256. |

Create a `.env` file from the example:

```bash
cp .env.example .env
# Edit .env and set ENCRYPTION_KEY
```

Generate a secure key:

```bash
openssl rand -base64 32
```

### Config File

Create a `conf/config.yaml` file with your configuration:

```yaml
server:
  host: "0.0.0.0"
  port: 8890

database:
  driver: sqlite
  source: "data/git-sync.db"
```

## Development

```bash
# Install dependencies (tidy both modules)
make tidy

# Run tests (core + shell)
make test

# Build
make build

# Lint (requires golangci-lint; run in both module roots)
golangci-lint run
(cd core && golangci-lint run)
```

## API Documentation

Once the server is running, access the API at `http://localhost:8890`.

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Project Stats

<!-- STATS_START -->
| Metric | Value |
|--------|-------|
| Test Files | 45 |
| Total Tests | 330 |
<!-- STATS_END -->
