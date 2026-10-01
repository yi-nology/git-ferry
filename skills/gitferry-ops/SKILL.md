---
name: gitferry-ops
version: 1.0.0
description: "GitFerry 运维中心：系统概览、健康评分、资产盘点、RPO、备份完整性、漂移检测、审计链、灾备演练、重建。当用户要巡检、合规检查、灾备或处理孤儿仓库时触发。"
metadata:
  requires:
    bins: ["gitferry"]
  cliHelp: "gitferry ops --help"
---

# gitferry-ops（运维中心）

**前置：** 先读 [`../gitferry-shared/SKILL.md`](../gitferry-shared/SKILL.md)。
**`ops +drill` / `ops +rebuild` / `ops +retry-batch` 为危险操作，必须先确认用户意图。**

## Shortcuts

| Shortcut | 说明 | 危险 |
|----------|------|------|
| `ops +overview` | 系统概览（仓库/任务/健康） | 否 |
| `ops +todo` | 统一待办队列（健康/孤儿/RPO） | 否 |
| `ops +push-backup` | 推送到 github/gitlab 备份远端 | **是** |
| `ops +repo-files` | 浏览任务 workdir 文件（救援） | 否 |
| `ops +org-map` | org 映射目标预览 | 否 |
| `ops +health` | 同步健康评分 + 问题列表 | 否 |
| `ops +inventory` | 资产盘点（孤儿仓库） | 否 |
| `ops +rpo` | RPO/RTO 观测 | 否 |
| `ops +integrity` | 冷备完整性（Merkle 比对） | 否 |
| `ops +drift` | 漂移检测（本地 vs 远端 refs） | 否 |
| `ops +audit` | 审计哈希链校验 | 否 |
| `ops +drill` | 灾备演练（恢复+fsck+refs） | **是** |
| `ops +rebuild` | 清 workdir 全量重建 | **是** |
| `ops +retry-batch` | 批量重试失败任务 | **是** |
| `ops +metadata-restore` | 元数据回灌（默认 dry-run） | **是**（`--execute`） |

## 使用示例

```bash
# 日常巡检入口（推荐从 todo 开始）
gitferry ops +todo --format json
gitferry ops +overview --format json
gitferry ops +health --format json

# 需要更严的 safety 评估时（网络开销）：折入漂移检测
gitferry ops +health --with-drift --format json

# 有没有仓库没被任务覆盖
gitferry ops +inventory --format json

# 备份时效
gitferry ops +rpo --max-seconds 86400 --format json

# 漂移 / 完整性 / 审计（只读三件套）
gitferry ops +drift --format json
gitferry ops +integrity --format json
gitferry ops +audit --format json

# 危险：灾备演练（会从冷备恢复并校验）
gitferry ops +drill --name nightly.bundle --yes

# 危险：任务重建
gitferry ops +rebuild --task daily-mirror --yes

# 元数据回灌：先 dry-run 看计划，再 --execute --yes 真正写入
gitferry ops +metadata-restore --key my-repo --format json
gitferry ops +metadata-restore --key my-repo --kinds labels,milestones --execute --yes
```

## 参数

| Shortcut | 参数 | 说明 |
|----------|------|------|
| `ops +health` | `--limit` | 列出问题条数 |
| `ops +rpo` | `--max-seconds` | RPO 阈值（秒），超出标记超标 |
| `ops +drift` | `--task` | 可选，限定任务 |
| `ops +drill` | `--name` | bundle 名称 |
| `ops +rebuild` | `--task` | 任务 key |
| `ops +retry-batch` | `--task-keys` | 逗号分隔任务 key |
| `ops +metadata-restore` | `--key` | 仓库 key（必填） |
| `ops +metadata-restore` | `--name` | snapshot_dir（空=最新） |
| `ops +metadata-restore` | `--kinds` | labels,milestones,issues,prs,releases |
| `ops +metadata-restore` | `--execute` | 真正写入（缺省 dry-run） |
| `ops +metadata-restore` | `--overwrite` | 同名 label 覆盖 |

## API 映射

| Shortcut | HTTP |
|----------|------|
| `ops +overview` | `GET /api/v1/ops/overview` |
| `ops +todo` | `GET /api/v1/ops/todo` |
| `ops +health` | `GET /api/v1/ops/health-score` |
| `ops +inventory` | `GET /api/v1/ops/inventory` |
| `ops +rpo` | `GET /api/v1/ops/rpo` |
| `ops +integrity` | `POST /api/v1/ops/backup-manifest/verify` |
| `ops +drift` | `GET /api/v1/ops/drift` |
| `ops +audit` | `GET /api/v1/ops/audit-chain/verify` |
| `ops +drill` | `POST /api/v1/ops/dr-drill` |
| `ops +rebuild` | `POST /api/v1/ops/rebuild` |
| `ops +retry-batch` | `POST /api/v1/ops/retry-batch` |
| `ops +metadata-restore` | `POST /api/v1/ops/metadata-restore` |

## 返回关键字段

| 字段 | 说明 |
|------|------|
| `items[].score` / `level` | 总分 0-100 / gold·silver·bronze·basic |
| `items[].dimensions[]` | 分项：`reliability` / `freshness` / `schedule` / `safety` / `completeness` |
| `items[].dimensions[].reason` | 人类可读原因（含成功率、最近执行时长等） |
| `items[].dimensions[].action` | 建议 CLI 动作（低分维度才有） |
| `items[].actions[]` | 该任务的汇总建议 |
| `attention` | score&lt;60 的任务列表（「需要关注」） |
| `summary.top_actions` | 跨任务去重的建议动作排行 |
| `orphans[]` | 无任务覆盖的仓库（inventory） |
| `items[].rpo_seconds` / `breach` | 备份时效与是否超标（rpo） |

### 维度权重（Scorecards 风格）

| 维度 | 权重 | 看什么 |
|------|------|--------|
| reliability | 35 | 近 N 次成功率、连续失败、最近是否成功 |
| freshness | 20 | 最近执行距今时长 |
| schedule | 15 | 是否有 cron、是否启用 |
| safety | 15 | force_push_policy / keep_divergent / git_force；`--with-drift` 时折入实测漂移 |
| completeness | 15 | 名称、历史、冷备 bundle、wiki |

**解读顺序：** 先看 `attention` → 对每个任务看低分维度的 `reason`/`action` → 用 `history +diagnose` 深挖 reliability 问题。

## 注意事项

- 巡检类命令全部只读，可放心先跑 `+overview` / `+health`
- **`+drill` 会写入恢复目录并做 fsck**；`+rebuild` 会清任务工作目录 —— 都要用户明确点头
- 审计链校验失败时，优先 `history +detail` 对照时间线，不要直接 rebuild

## References

- [gitferry-history](../gitferry-history/SKILL.md)
- [gitferry-workflow](../gitferry-workflow/SKILL.md)
- [gitferry-shared](../gitferry-shared/SKILL.md)
