# GitFerry

> Chinese name 「摆渡」— ferry your code where it belongs: between intranets, out to the open-source world, and into backup harbors.

[![CI](https://github.com/yi-nology/git-ferry/actions/workflows/ci.yml/badge.svg)](https://github.com/yi-nology/git-ferry/actions/workflows/ci.yml)
[![Release](https://github.com/yi-nology/git-ferry/actions/workflows/release.yml/badge.svg)](https://github.com/yi-nology/git-ferry/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/yi-nology/git-ferry)](https://goreportcard.com/report/github.com/yi-nology/git-ferry)
[![Go Version](https://img.shields.io/github/go-mod/go-version/yi-nology/git-ferry)](https://go.dev/)
[![License](https://img.shields.io/github/license/yi-nology/git-ferry)](LICENSE)
[![Latest Release](https://img.shields.io/github/v/release/yi-nology/git-ferry)](https://github.com/yi-nology/git-ferry/releases)

**[中文文档](./README.zh-CN.md)**

[Install](#installation) · [Web Console](#web-console) · [CLI & Agent Skills](#cli--agent-skills) · [Ops](#ops-center) · [Config](#configuration) · [Contributing](#contributing)

GitFerry is a self-hosted hub for Git repositories: **sync across platforms**, **publish to the open-source world** (dual-identity module mirrors), and **back up** — all in one place. Ships with a Vue web console, a `gitferry` CLI, and AI Agent Skills.

## Why GitFerry?

- **Multi-platform sync** — GitHub / GitLab / Gitee / GitLink / self-hosted; cron + webhook
- **Mirror hub** — open-source publishing (module path rewrite snapshots, pre-check / build gate / divergence confirm) and repo backup
- **Ops-ready** — health score, inventory, RPO, drift detection, audit hash chain, DR drills
- **Agent-Native** — `gitferry` CLI with structured Envelope output + installable [Agent Skills](./skills/)
- **AI assistant** — eino-based chat ops (29 tools, dangerous ops require confirmation)
- **Self-hosted** — SQLite or MySQL, Docker or binary, MIT licensed

## Features

| Area | Capabilities |
|------|--------------|
| Sync | Multi-platform, cron, webhook, wiki, submodules, partial clone |
| Mirror | Module identity rewrite, dual-repo publish, backup channels |
| Ops | Metrics, failure retry, health score, inventory, templates, audit export |
| Backup | Git bundle + rotation, Merkle manifest, S3/WebDAV/Azure destinations, metadata snapshot |
| DR | DR drill (restore + fsck + refs), RPO/RTO, drift detection |
| Governance | RBAC (admin/operator/readonly), OIDC JWT, audit hash chain, legal hold |
| AI | eino assistant, plan mode, memory, permission tiers, confirm cards |
| Interfaces | Web console, REST API, `gitferry` CLI, Agent Skills |

## Installation

### From Release

Download the latest binary from [Releases](https://github.com/yi-nology/git-ferry/releases):

- `git-ferry-{linux,darwin,windows}-{amd64,arm64}` — server
- `gitferry_{version}_{os}_{arch}.{tar.gz|zip}` — CLI

### From Source

```bash
git clone https://github.com/yi-nology/git-ferry.git
cd git-ferry
make build        # server: output/git-ferry
make build-cli    # CLI:   output/gitferry
```

Requires Go 1.26+. Frontend build needs Node.js 20+.

### Docker

```bash
make docker-build
```

## Quick Start

```bash
cp .env.example .env
# set ENCRYPTION_KEY (see Configuration)
make run
# open http://localhost:8890
```

## Web Console

Vue 3 + Ant Design Vue console, styled after GitHub Enterprise / Linear ops tools (neutral palette, metrics-first dashboard).

| Entry | What it does |
|-------|----------------|
| Dashboard | Metrics + failed tasks “needs attention” + recent sync/repos |
| Sync tasks / History | Task CRUD, filters, batch ops, run detail |
| Repos / Mirror hub | Repo cards, unified config, publish & backup channels |
| Webhook rules / Events | Trigger rules and inbound event stream |
| Ops center | Health score, inventory, templates, deploy keys, cold backup |
| AI assistant | Floating ball / top bar; configure model under System |
| CLI / Agent | System → CLI / Agent: install `gitferry`, Skills, command cheatsheet |
| Platforms | Git host credentials and connection tests |

![Login](docs/screenshots/login.png)
![Dashboard](docs/screenshots/dashboard.png)
![Sync tasks](docs/screenshots/sync-tasks.png)

More screenshots: [`docs/screenshots/`](docs/screenshots/). Frontend dev: [`frontend/README.md`](frontend/README.md).

## CLI & Agent Skills

Agent-Native tooling inspired by [gitlink-cli](https://github.com/ccfos/gitlink-cli).

### gitferry CLI

```bash
export GITFERRY_BASE_URL=http://127.0.0.1:8890
export GITFERRY_TOKEN=your-api-key        # maps to X-API-Key

gitferry auth status
gitferry task +list --format json
gitferry history +list --task t1 --format json
gitferry ops +health --format json
gitferry api GET /api/v1/system/status --format json
```

- Output is a unified Envelope: `{ok, data, error, meta}`
- Dangerous shortcuts (`task +run`, `ops +drill`, …) require confirmation; scripts pass `--yes`
- Shortcuts (`+list` / `+info` / …) for high-frequency ops; `gitferry api` for everything else

Install via npm (after release):

```bash
npm install -g gitferry-cli
gitferry-install-skills
```

### Agent Skills

`skills/` ships structured knowledge for Claude Code / MiMo / Cursor:

| Skill | Covers |
|-------|--------|
| `gitferry-shared` | Auth, Envelope, safety rules |
| `gitferry-repo` / `gitferry-task` / `gitferry-history` | Repos, tasks, run history |
| `gitferry-ops` / `gitferry-workflow` | Ops checks, failure triage, DR drills |

```bash
make install-skills
# or: npx skills add ./skills -y -g
```

Design notes: [`docs/superpowers/plans/2026-09-30-agent-native-cli-skills.md`](docs/superpowers/plans/2026-09-30-agent-native-cli-skills.md).

## Ops Center

Borrowing ideas from gickup / ghorg / Renovate / Scorecards:

| Capability | Entry |
|------------|-------|
| Prometheus metrics | `GET /metrics` |
| Failure compensation | `runwatch` + auto retry; `/api/v1/ops/retry` |
| Notifications | ntfy / gotify + success/fail heartbeats (healthchecks.io) |
| Health score | `/api/v1/ops/health-score` (gold/silver/bronze/basic) |
| Inventory | `/api/v1/ops/inventory` (orphan repos) |
| Policy templates | `/api/v1/ops/templates` + preview/apply (dry-run) |
| Audit export | `/api/v1/ops/audit-report?format=csv` |
| Deploy keys | `/api/v1/ops/deploy-key` (Ed25519) |
| Cold backup | `git_bundle` + `sync.backup_dir` rotation |
| DR drill | `/api/v1/ops/dr-drill` + RPO `/api/v1/ops/rpo` |
| Integrity | Merkle root manifest + verify |
| Metadata snapshot | issues/PR/releases + archives + gists |
| Multi-destination | `sync.backup_destinations` (s3/webdav/azure/local) |
| Lifecycle | auto-discover, drift, force-push policy |
| Governance | RBAC, OIDC, audit chain, legal_hold |
| Rebuild | `/api/v1/ops/rebuild` (fresh full fetch) |
| GitHub archive | `/api/v1/ops/migration` (Migration API tar.gz) |

Config: `conf/config.example.yaml` → `runwatch` / `notify` sections.

## AI Assistant (eino)

Built on [CloudWeGo eino](https://github.com/cloudwego/eino). **Disabled by default**.

Enable in **System → AI Assistant**: pick a preset (OpenAI / DashScope / DeepSeek / Ollama / vLLM / custom) or set **API Base URL**, choose **model** (or fetch list), set **API Key**, **Save** (hot reload, no restart).

29 tools covering queries, health, inventory, RPO, integrity, drift, audit, failure diagnosis, memory (`plan_mode` / `remember` / `recall`), and confirmed dangerous ops (`run_task`, connection tests, DR drill).

**Safety:** read-only by default; git credentials and model keys never enter prompts/logs; no destructive tools without UI confirmation.

| Method | Path | Notes |
|--------|------|-------|
| GET | `/api/v1/ai/status` | 501 = disabled |
| POST | `/api/v1/ai/chat` | SSE chat |
| GET | `/api/v1/ai/config` | config (key masked) |
| POST | `/api/v1/ai/config` | save + hot reload |
| POST | `/api/v1/ai/config/test` | probe OpenAI-compatible endpoint |

## Configuration

### Environment

| Variable | Required | Description |
|----------|----------|-------------|
| `ENCRYPTION_KEY` | Yes | AES-256-GCM key for credential storage |
| `GITFERRY_TOKEN` | CLI | API key for `gitferry` CLI (`X-API-Key`) |
| `GIT_SYNC_AI_API_KEY` | AI | AI assistant API key (optional) |
| `GIT_SYNC_TOKEN_<NAME>` | Ops | Injected platform tokens (never in request body) |

```bash
cp .env.example .env
openssl rand -base64 32   # generate ENCRYPTION_KEY
```

### Config file

```yaml
server:
  host: "0.0.0.0"
  port: 8890

database:
  driver: sqlite
  dsn: "data/git_sync.db"
```

## Related repositories

| Repository | Import path | Role |
|------------|-------------|------|
| [git-sync-core](https://github.com/yi-nology/git-sync-core) | `github.com/yi-nology/git-ferry-core` | Sync engine library (no HTTP) |
| **git-ferry** (this repo) | `github.com/yi-nology/git-ferry` | Public shell: hz API + Vue UI + CLI |
| [git-sync-intranet](https://github.com/yi-nology/git-sync-intranet) | `github.com/yi-nology/git-sync-intranet` | Intranet shell (gateway/SSO auth) |

Single-repo build works (`git clone` + `go build`). Local core workspace: `go work init . ../git-sync-core` (do not commit `go.work` or `replace`).

## Development

```bash
make tidy && make test && make lint
make build && make build-cli
make apidoc          # regenerate docs/openapi.json
cd frontend && npm ci && npm run build
```

See [CONTRIBUTING.md](CONTRIBUTING.md) and [ARCHITECTURE.md](ARCHITECTURE.md).

## Security

Please report vulnerabilities privately — see [SECURITY.md](SECURITY.md).

## Contributing

1. Fork and branch (`git checkout -b feature/amazing-feature`)
2. Keep `make test` / `make lint` green
3. Open a PR using the template

## License

MIT — see [LICENSE](LICENSE).

## Project Stats

<!-- STATS_START -->
| Metric | Value |
|--------|-------|
| Test Files | 19 |
| Total Tests | 104 |
<!-- STATS_END -->
