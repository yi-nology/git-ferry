# Changelog

GitFerry — 自托管 Git 同步/镜像/备份中枢。本文件记录壳层发版变化。

## [v1.20.1] - 2026-10-01

> 依赖 git-ferry-core v0.7.1（ForcePushApprover 注入）。

### Added

- **core v0.7.0 接入**：分支过滤 `include_branches`/`exclude_ref_patterns`
  （converter/create/update + 任务向导表单）。
- **备份远端推送** `POST /ops/push-backup` + CLI `ops +push-backup`（github/gitlab git remote）。
- **workdir 浏览** `GET /ops/repo-files` + CLI `ops +repo-files`（防路径穿越）。
- 首启向导平台链接修正；goreleaser brews/scoops；OpenAPI 85 paths。


### Added

- **force-push 审批闭环**：executor 注入 `ForcePushApprover`；`block` 策略分歧时
  自动登记 pending，Admin 放行后放行覆盖（core `SetForcePushApprover` + 壳层适配器）。
- **备份远端 push**（P2.2）：`sync.backup_remotes[]` 同步成功后向 GitHub/GitLab 等
  push 镜像副本（`{owner}/{repo}` 占位）；与开源发布区分，不改写 module 身份。
- **同步趋势** `GET /ops/trends?days=N` + CLI `ops +trends`；前端可接小图。
- **metrics 按任务标签** `sync_runs_total{status,task}`。
- **DR 演练验元数据**：`dr-drill?with_metadata=true` 抽样比对快照清单/issues 分片。
- **AI 工具** `get_metadata_snapshots`（只读；回灌仍走 CLI）。
- **CLI** `ops +git-url` 打印 Git Smart HTTP clone URL。
- **配置启动校验**：`backup_format` 枚举 / `backup_keep` 范围（对齐 schema 子集）。
- **首启向导**：仪表盘无仓库无任务时展示三步引导。

## [v1.19.4] - 2026-10-01

### Changed

- **README 全面重写**（中英双语对齐）：能力总览、快速开始、运维与灾备矩阵、
  Agent 三接入面（CLI/MCP/Skills）、元数据恢复闭环、Git Smart HTTP 示例、
  配置样例与架构图；链接指向规划/调研/示例模板。

### Fixed

- 再次移除 `org-mirror` 路径中对未发布 core 字段 `IncludeBranches` 的引用，修复 CI。

## [v1.19.3] - 2026-10-01

### Added

- **org 映射预览** `POST /ops/resolve-org-target` + CLI `ops +org-map`
  （preserve/single/flat/mixed，`internal/orgmap` 纯逻辑+单测）。
- **公共组织导入** `POST /ops/import-public-org`（可选建任务）；
  与已有 `org-mirror` / `import-starred` 路由对齐。
- **force-push 审批流** `ops/force-push-approvals`（申请/列表/Admin 放行）。
- **post-exec 钩子** `sync.post_exec_script`（GITFERRY_TASK/RESULT/RUN_ID/TRIGGER）。
- **冷备 zip 形态** `sync.backup_format: bundle|zip`（keep 轮转两者皆支持）。
- **配置 JSON Schema** `conf/config.schema.json`。
- **分发与 CI 模板** `examples/packaging/{homebrew,scoop,nix}`、
  `examples/ci/github-actions-mirror.yml`。
- OpenAPI 收录 org 相关端点（81 paths）。



> 对标开源（gickup / ghorg / gitea-mirror / github-backup-rust）缺口批次。
> 规划：`docs/superpowers/plans/2026-10-01-competitor-gap-roadmap.md`。

### Added

- **元数据 Restore（P0）**：`POST /api/v1/ops/metadata-restore`（Admin，**默认 dry-run**）
  - labels → milestones → issues（含评论/关闭）→ PRs（issue 形态标注）→ releases
  - CLI `ops +metadata-restore --key <repo> [--kinds …] [--execute] [--overwrite]`
  - 前端冷备页「回灌」入口；历史写 `backup_dir/metadata-restore/<repo>/`
- **平台 API 限流退避（P0.2）**：403/429 + Retry-After / X-RateLimit 指数退避
  （1s→60s，最多 5 次）；指标 `gitferry_api_ratelimit_total`
- **分支过滤 + 忽略 PR refs（P1.3）**：任务字段 `include_branches` / `exclude_ref_patterns`
  （默认排除 `refs/pull/*`、`refs/merge-requests/*`）；CLI 对应 flags
- **MCP Server（P1.1）**：`POST /mcp` Streamable HTTP，与 eino 工具同表
  （initialize / tools/list / tools/call）；`GET /mcp` 探测
- **Git Smart HTTP 只读（P2.1）**：`git_serve` 配置段，局域网/灾备 `git clone`
- **元数据增量（P3）**：`metadata-backup?since=RFC3339`

## [v1.19.2] - 2026-10-01

### Fixed

- 去掉对未发布 `git-ferry-core` 字段（`IncludeBranches`/`ExcludeRefPatterns`）的引用，
  恢复基于 `git-ferry-core v0.6.1` 的 CI 构建。

## [v1.19.1] - 2026-10-01

### Added

- **只读 Git Smart HTTP**（`internal/gitserve`）：内网/灾备场景 `git clone` 冷备或 workdir 的 bare 仓。
- **MCP 端点**（`internal/mcp`）：把 agent 工具表暴露给 Claude Code / Cursor 等外部客户端。
- **GitHub API 节流**（`internal/githubapi/throttle`）：速率控制 + metrics。
- Playwright 截图回归脚本 `scripts/e2e-screenshots.js`（`make e2e-shots`）+ 关键页截图
  （login/dashboard/ops/devhub/health/templates）。
- 测试补齐：`OpsTodo`/`HealthScore` 响应结构与空态、模板 `extends` 链、
  CLI client 分页、config 读写与 env 覆盖、批量 CSV 转义。

### Fixed

- 批量结果 CSV 对含逗号/引号的 error 字段做转义（原先可能破坏 CSV 结构）。
- `prBody` 改用 `fmt.Fprintf`，清掉 staticcheck QF1012。
- 前端统一走 `notify*` toast（静态 `message` 在 AntD 4 不可靠）：仪表盘复制、DeveloperHub。
- HealthScorePanel 样式对齐设计 token；待办/建议动作支持复制命令、危险标记。
- 健康 attention 按分升序；模板 `extends` 成环错误信息带上完整链路。
- DeveloperHub Release 安装命令改为变量版本号 + Releases 链接，不再写死旧版本。

### Changed

- `goreleaser` 明确为可选路径（默认发版仍走 `release.yml`）；Makefile 增加
  `e2e-shots` / `goreleaser-snapshot`。

## [v1.19.0] - 2026-09-30

> 对照 gitlink-cli 的 Agent-Native / 开源工程化批次 + Scorecards/Renovate 业务深化。
> 依赖 `git-ferry-core v0.6.1`。新增 `github.com/spf13/cobra v1.10.2`（CLI）。

### Added

- **`gitferry` CLI**（`cmd/gitferry` + `internal/cli`）：
  - Shortcuts：`repo/task/history/ops/platform/webhook` 域（`+list/+info/+run/+batch-run/...`）
  - Raw API `gitferry api METHOD PATH`、Schema 自省 `schema list/show`（内嵌 OpenAPI）
  - 统一 Envelope `{ok,data,error,meta}`，`--format json|table|yaml`
  - `--all` 全量分页、`--dry-run` 批量预览、`--with-drift` 健康折入漂移、`--csv` 导出
  - 危险操作 409 确认门（脚本 `--yes`）；`config init/get/set/list/path`
  - macOS Keychain 存 API Key（文件 0600 回退）
- **Agent Skills**（`skills/gitferry-*`）：shared/repo/task/history/ops/workflow，
  含 troubleshooting、flags 参考；`make install-skills` / npm `gitferry-install-skills`
- **Web「CLI / Agent 入口」**（`/settings/dev`）：安装/认证/命令速查/实时连通
- **健康评分维度化（Scorecards）**：reliability/freshness/schedule/safety/completeness
  五维 + reason + 可复制动作（`health.RouteActions`）；`/ops/health-score` 返回
  `dimensions/action_items/attention/top_actions`；`with_drift` 折入实测漂移
- **统一待办**：`GET /api/v1/ops/todo` + CLI `ops +todo` + AI `get_ops_todo`；
  仪表盘运维待办卡片（P1/P2 + 动作一键复制）
- **策略模板继承**：`Template.Extends` 链式合并（子覆盖父）、环检测，
  apply 返回 `effective_spec`/`extends_chain`；Templates UI 可选继承
- **token_cmd 密钥注入**：`GIT_SYNC_TOKEN_CMD_<KEY>`（secrets manager）→ env → 配置
- **审计 CSV**：`ops +audit-report --csv`、`task +batch-run --csv`
- **开源工程化**：双语 README、CONTRIBUTING/SECURITY/Issue&PR 模板、
  npm 分发包（`gitferry-cli`）、`.goreleaser.yaml`、Release 产出 CLI 六平台压缩包、
  验收清单与相邻工具调研（`designs/research/ops-ux-patterns.md`）
- **AI 工具**：`get_ops_todo`；`deep_analyze` 输出 `weak_dims` 与维度驱动 next_action
  （工具总数 30）

### Changed

- 健康/历史采集：成功率、连续失败、步骤级失败、重试次数、主导错误类型
- 健康计算并行拉历史（消除 N+1）；仪表盘露出薄弱维度
- OpenAPI 收录 `/ops/todo` 等（78 paths）；lint/tidy/gofmt 收口

## [v1.18.0] - 2026-09-29

> 依赖 `git-ferry-core v0.6.0`。对标 gickup / gitea-mirror / ghorg 的 P0–P5 演进批次。

### Added

- **P0 灾备闭环**:
  - DR 演练 `POST /ops/dr-drill`(单个/批量):恢复到临时目录 → `git fsck` → refs 比对 → RTO 观测;
    历史 JSONL 哈希链(`GET /ops/dr-drill/history`、`/dr-drill/chain/verify`)。
  - 备份完整性证明:SHA256 + Merkle Root 清单(`POST /ops/backup-manifest`、`GET /ops/backup-manifest/verify`)。
  - RPO/RTO 观测 `GET /ops/rpo`:按任务暴露最近备份时长、超标标记、估算恢复时间。
- **P1 元数据资产**: `POST /ops/metadata-backup` 快照 issues(+评论)/PR/labels/milestones/releases,
  可选下载各 tag 的 source archive;`GET /ops/metadata-backups` 列表。
- **P2 多目的地扇出**: `sync.backup_destinations`(s3/webdav/azure/local),与 `backup_s3` 并存;
  同步成功后自动加密(可选)并逐目的地上传,失败互不阻断。
- **P3 生命周期**:
  - `POST /ops/auto-discover` 平台新仓库发现/导入。
  - `POST /ops/drift` 本地与目标远端分支漂移检测。
  - 任务字段 `force_push_policy`:`allow | block | backup_on_demand`(覆盖前自动打回滚快照)。
  - `POST /ops/backup-cleanup` 按 `backup_retention_days` 清理;`legal_hold` 时拒绝。
- **P4 认证治理**:
  - RBAC `admin|operator|readonly`;写操作 `WriteGuard`、治理操作 `AdminGuard`。
  - OIDC/JWT Bearer(HS256)可选鉴权 `auth.oidc`。
  - 审计哈希链 `prev_hash/entry_hash` + `GET /ops/audit-chain/verify`。
  - 平台表新增 GitHub App 字段(app_id / installation_id / private_key)。
- **P5 存储效率**: 冷备 AES-256-GCM(`sync.backup_encrypt_key`),`.bundle.enc` 恢复自动解密;
  retention + legal_hold;前端运维中心新增「灾备演练」页签。
- **GitHub App 签发**: RS256 JWT + installation access token(缓存 55min),配置了
  `github_app_id/installation_id/private_key` 时优先于 PAT;私钥加密入库。
- **Release 二进制附件**: `metadata-backup` 可下载 GitHub Release assets(`with_assets`);
  **Gists 备份** `POST /ops/gists-backup` 或快照 `with_gists`。
- **前端**: 任务表单「强制推送保护」三策略;冷备页「元数据快照」;灾备页「漂移检测」。

- **AI 模型发现**: `GET /api/v1/ai/models` + 设置页「获取模型」，从端点拉取真实模型列表。
- **仪表盘运维健康**: 平均分、金银铜/基础分布、待改进项数，直达运维中心。
- Webhook 列表表格式内容卡；清理残留旧页头样式。

## [2026-09-29] - 前端重设计与 AI 配置

### Added

- **前端全面重设计**（GitHub/Linear 式冷静运维台）:设计 tokens + AntD 主题、
  侧栏分组 IA、统一 `PageHeader` / `MetricStrip`、仪表盘失败任务「需要关注」。
- **AI 助手配置界面**（系统 → AI 助手）:服务预设（OpenAI/DashScope/DeepSeek/Ollama/vLLM）、
  Base URL、模型选择、API Key、温度/Max Tokens/超时；保存写 `data/ai-settings.json` 并热重建 Runner。
- **AI 配置 API**: `GET/POST /api/v1/ai/config`、`POST /api/v1/ai/config/test`（密钥脱敏回传）。
- **AI 面板重设计**:空态建议问题、工具 chip、危险操作确认卡、composer 输入区。
- **文档与截图更新**: `docs/screenshots/` 新版 UI 全套;README / frontend README / designs 对齐现状。

### Changed

- 登录页品牌统一为 GitFerry（去掉紫色渐变）。
- `getAIStatus` 兼容 `{code,data}` 与裸 JSON，避免误判「未启用」。
- AI 工具事件文案改为「调用中… → 完成/失败摘要」。

## [2026-09-27] - 运维与备份能力批次

### Added

- **运维中心**(侧栏): 概览 / 健康评分 / 资产盘点 / 策略模板 / 部署密钥。
- **可观测**: `GET /metrics` Prometheus;HTTP 低基数路径指标。
- **通知矩阵**: ntfy / gotify / success-fail heartbeat / 通用 HMAC 签名 webhook。
- **失败补偿**: runwatch 轮询 + 自动重跑;`POST /ops/retry`、`/ops/retry-batch`。
- **健康评分** `GET /ops/health-score` (gold/silver/bronze/basic,可配置规则 DSL)。
- **资产盘点** `GET /ops/inventory`(孤儿仓库)。
- **策略模板** CRUD + preview + apply(dry-run);`data/templates.json`。
- **审计导出** `GET /ops/audit-report?format=csv`。
- **过滤导入** `POST /ops/sync-platform`(archived/fork/star/language/glob)。
- **GitHub Migration 全量归档** `POST /ops/migration`。
- **Issues 导出** `GET /ops/issues-export`。
- **部署密钥** `POST /ops/deploy-key`(Ed25519,私钥仅返回一次) + UI。
- **一键重建** `POST /ops/rebuild`。
- **任务高级选项**: `git_lfs` / `sync_wiki` / `git_bundle` / `submodules` /
  `git_push_prune` / `keep_divergent`。
- **密钥注入** `GIT_SYNC_TOKEN_<NAME>`。
- **配置** `sync.backup_dir/backup_keep/partial_clone/backup_s3`、`runwatch`、`notify`。

### Security

- 确认令牌 crypto/rand 128bit;危险工具放行标记一次性消费。
- 工具输出 json.Marshal,消除 `%s` 拼 JSON。
- `conf/config.yaml` 移出 git;弱密钥黑名单。
- 默认 API Key 鉴权写入 `api-key:<指纹>`,杜绝 X-User 冒充。
- SSH 主机指纹钉扎 UI。

### Fixed

- webhook 按 IP 限流;GetSyncService 判空;GetRepo 未命中 404。
- Dashboard 不再污染共享 store;仓库搜索竞态;静默失败提示。
- PlatformSettings 947 行拆分;any/catch 类型收敛。
