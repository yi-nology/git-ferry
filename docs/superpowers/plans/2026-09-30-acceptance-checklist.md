# 阶段验收清单（对照 gitlink-cli design.md §12）

> 适用于 CLI / Skills / 开源工程化改造。每阶段打勾后再进入下一阶段。

## Phase A — CLI 能力（已达成）

| 项 | 验收命令 | 期望 |
|----|----------|------|
| Schema 自省 | `gitferry schema list` / `show sync` | 列出域；`show diagnose` 能命中 |
| 分页拉全 | `gitferry task +list --all --format json` | 一次拿全量列表 |
| 批量 dry-run | `gitferry task +batch-run --task-keys t1,t2 --dry-run` | 返回 `planned`，不触发 |
| 批量执行门 | `gitferry task +batch-run --task-keys t1` | 无 `--yes` 时 409 |
| 配置管理 | `gitferry config init/set/get/list` | 读写 `~/.config/gitferry/config.yaml` |
| 多格式 | `--format json\|table\|yaml` | 三种格式均可解析 |

## Phase B — 工程化

| 项 | 验收 | 期望 |
|----|------|------|
| 单测 | `go test ./internal/cli/...` | 全绿 |
| 危险门 | 未 `--yes` 的 run/delete/drill | 一律 `ok=false` + suggestion |
| Keychain | `gitferry auth login --token x`（macOS） | 优先进 Keychain，回退文件 0600 |
| 多平台构建 | `goreleaser build --snapshot --clean`（可选） | 6 平台产物 |
| npm 自测 | `make pack-npm` | 产出 tgz，含 skills |

## Phase C — Skills

| 项 | 验收 | 期望 |
|----|------|------|
| 安装 | `make install-skills` | skills 进入 Agent |
| 共享规则 | 阅读 `gitferry-shared` | 含 Envelope / 危险操作约定 |
| 排障 | `gitferry-shared/references/troubleshooting.md` | 错误码 → 处置 |
| 工作流 | `gitferry-workflow` | 失败排查 / 接入 / 灾备 / 周报 |

## Phase E — 业务逻辑深化（Scorecards / Renovate）

| 项 | 验收 | 期望 |
|----|------|------|
| 维度评分 | `go test ./internal/health/` | 五维权重合计 100；失败场景压低 reliability |
| 健康 API | `GET /api/v1/ops/health-score` | 含 `dimensions`/`actions`/`attention` |
| 模板继承 | `go test ./internal/tpl/` | 链式合并、环检测、缺父停止 |
| apply 合并 | `POST /ops/templates/apply` | 返回 `effective_spec` + `extends_chain` |
| AI 工具 | `get_sync_health` | 维度 + legend 说明 |

## Phase D — 端到端（Agent 实测）

用外部 Agent（Claude Code / MiMo）仅靠 skills + CLI 完成：

1. `gitferry auth status` — 连通
2. `gitferry task +list --format json` — 找到失败任务
3. `gitferry history +diagnose --run-id <ID>` — 给出原因
4. 用户确认后 `history +retry --yes` — 验证转 success
5. 全程未打印 API Key；危险步骤均先确认

## 回归命令（发版前）

```bash
go test ./... -count=1
go vet ./...
cd frontend && ./node_modules/.bin/vue-tsc -b && npm run build
make build && make build-cli && make pack-npm
```
