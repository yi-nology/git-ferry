# Changelog

GitFerry — 自托管 Git 同步/镜像/备份中枢。本文件记录壳层发版变化。

## [Unreleased]

### Changed

- **代码内聚重构**:
  - `internal/githubapi`: GitHub 附件/gists 客户端独立包,与 handler 解耦。
  - `internal/pkg/textutil`: 路径安全化 + CSV 转义统一实现,消除重复。
  - `ops_service` 拆为 `retry_service` / `health_service` / `audit_report`。
  - `newIssueProvider` 抽到 `provider_access`,issues/metadata 共用。

### Added

- **任务策略变更预览**: 编辑时改 `force_push_policy` 显示 原值→新值 diff、风险说明与放宽警告。
- **DR 演练报告导出**: `GET /ops/dr-drill/export?format=json|csv`,前端一键下载。
- **GitHub App 全链路联调测试**: JWT 签发 → installation token → 缓存 → provider 配置。
- **GitHub App 配置贯通**: 平台表单/IDL/接口支持 `github_app_id/installation_id/private_key`,
  与 PAT 二选一;PlatformInfo 回传 `has_github_app`。
- **任务编辑 `force_push_policy`**: 创建/编辑统一三策略,兼容 `keep_divergent`。
- **AI 工具扩展**: `get_rpo_report` / `get_backup_integrity` / `get_drift_report` /
  `get_audit_chain` / `run_dr_drill`(确认门),共 29 个工具。
- **生命周期后台**: `sync.auto_discover_interval_minutes` 周期自动发现 +
  冷备 retention 自动清理(legal_hold 时跳过)。
- **OpenAPI/ARCHITECTURE 对齐** Ops 接口(77 paths);冷备页元数据快照列表。

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
