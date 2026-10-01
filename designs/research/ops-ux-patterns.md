# 相邻工具借鉴（Renovate / Scorecards / 交互模式）

> 补充 `git-backup-mirror-findings.md`：备份执行侧已调研（gickup/ghorg/GitLab Mirror…），
> 本文聚焦**运维交互与健康评估**可直接搬进 GitFerry 的设计。

## 1. Renovate（自动化/策略交互）

来源：docs.renovatebot.com / 自托管 Renovate 的成熟 UX

| 模式 | 做法 | GitFerry 可对齐 |
|------|------|----------------|
| **配置继承** | `config:base` / shareable presets，仓库级只写差异 | 策略模板 `ops/templates` 支持「基础预设 + 覆盖」 |
| **dry-run / 本地验证** | `renovate --dry-run=lookup` 只解析不写 | 已有：`templates/apply` dry-run；CLI `--dry-run` |
| **分组与批处理** | `packageRules` group、`prConcurrentLimit` | 批量任务按平台/标签分组限流 |
| **自动合并门槛** | tests passed + 稳定窗口（`stabilityDays`） | 同步成功 N 次后才允许 auto-force / 自动发布 |
| **Dashboard / Dependency Dashboard** | 单 Issue 汇总待办，可勾选 | 「运维待办」单页：失败任务 + 孤儿仓 + 超期备份，可勾选批量处理 |
| **Token 安全** | `git-author` / `platform-token` 分权，最小权限 | 平台 token 按读/写分 token；`GIT_SYNC_TOKEN_<NAME>` 已有 |
| **可观测** | 日志 level 分层、internal metrics | `runwatch` 日志 + `/metrics` 已有，可补 `reason` 标签 |

**可落地优先级：**
1. 策略模板继承（base + override）— 现在 templates 是平铺
2. 「运维待办 Dashboard」勾选批量 — 对接 `ops +retry-batch` / `task +batch-run`
3. 自动合并门槛：连续成功 N 次才允许放宽 `force_push_policy`

## 2. OpenSSF Scorecards / Security Scorecards

来源：github.com/ossf/scorecard

| 维度 | 评估什么 | GitFerry 健康分可加的维度 |
|------|----------|-------------------------|
| **Binary-Artifacts** | 仓库是否含二进制 | 备份是否含 bundle/LFS 对象 |
| **Branch-Protection** | 保护分支策略 | 目标分支是否受保护、force 策略 |
| **CI-Tests** | 测试是否跑 | 任务是否有校验步骤（fsck / refs 比对） |
| **Maintained** | 近 90 天提交 | 源仓库活跃度（影响是否仍需备份） |
| **Token-Permissions** | workflow 最小权限 | 平台 token 权限是否过大 |
| **Vulnerabilities** | 已知 CVE | （可选）依赖扫描 |
| **Pinned-Dependencies** | 依赖钉版本 | 同步镜像是否钉 commit/tag 而非飘 branch |

**打分输出模式：**
- 每项：score 0–10 + `reason`（人类可读）+ `details[]`
- 聚合：gold/silver/bronze/basic（GitFerry 已有等级，可把维度拆细）
- 报告：JSON 机器可读 + 简短 Markdown

**可落地优先级：**
1. 把现有 `health-score` 的 `issues[]` 升级为「维度 + 分 + reason」
2. 新增维度：`backup_freshness` / `drift` / `branch_protection` / `token_scope`
3. 报告可 `gitferry ops +health --format yaml` 直接贴周报

## 3. ghorg / gickup 的交互细节（补充结论）

已在 `git-backup-mirror-findings.md`，这里只记「交互层」：

| 点 | 来源 | 建议 |
|----|------|------|
| `--dry-run` 先报条数 | ghorg filter hooks | CLI 批量一律 dry-run 优先（已做） |
| `stats` CSV 审计 | ghorg stats | 每次批量操作写 operation_log + 可导出 CSV |
| `token_cmd` 取密钥 | ghorg + 1Password | 支持 `GIT_SYNC_TOKEN_CMD` 从 secrets manager 注入 |
| `post_exec_script` | ghorg | `notify.webhook` 已有；可补 per-task hook |
| 结构化本地目录 `host/user/repo` | ghorg/gickup | `workdir` 布局文档化，便于救援时手找 |
| keep: N 轮换 | gickup zip | `backup_keep` 已有，对齐文档 |

## 4. 建议迭代顺序（业务能力，非 CLI 皮）

1. **健康分维度化**（Scorecards 模式）→ 先改 `internal/agent/tools/ops.go` 输出结构
2. **运维待办 Dashboard**（Renovate Dependency Dashboard 模式）→ 前端运维中心新 Tab
3. **策略模板继承**（base + override）→ `/ops/templates` schema
4. **token_cmd / secrets 集成** → `corebridge.tokenenv` 扩展
5. **批量操作审计 CSV** → `ops/audit-report` 与 CLI 打通

## 5. 不建议跟风

| 模式 | 原因 |
|------|------|
| Renovate 整包引入 | 它是依赖更新工具；我们只要它的策略/待办交互 |
| Scorecard 完整扫描器 | 太重；只要「分维度 + reason」的报告形状 |
| P2P peer 同步（hesokuri） | 与「中枢摆渡」定位冲突 |

---

相关：
- `designs/research/git-backup-mirror-findings.md` — 执行面调研
- `docs/superpowers/plans/2026-09-30-agent-native-cli-skills.md` — Agent-Native 改造
- `docs/superpowers/plans/2026-09-30-acceptance-checklist.md` — 验收清单
