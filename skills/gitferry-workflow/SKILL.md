---
name: gitferry-workflow
version: 1.0.0
description: "GitFerry 复合运维工作流：同步失败根因排查、新平台接入、灾备演练、日常巡检。当用户目标跨多个步骤（诊断+修复+验证）时触发。"
metadata:
  requires:
    bins: ["gitferry"]
  cliHelp: "gitferry --help"
---

# gitferry-workflow（复合工作流）

**前置：** 先读 [`../gitferry-shared/SKILL.md`](../gitferry-shared/SKILL.md)。
**每个工作流中标记「危险」的步骤，必须先向用户确认再执行。**

以下 recipes 把多个 Shortcuts 串成可复用流程。执行时**逐步走、逐步汇报**，不要一口气跑完危险步骤。

---

## 1. 同步失败根因排查

**目标：** 任务反复失败，定位原因并给出修复建议。

```bash
# 1) 现状：健康与最近失败（含维度）
gitferry ops +health --format json
gitferry history +list --task <TASK_KEY> --limit 5 --format json

# 2) 看一次失败的步骤链
gitferry history +detail --task <TASK_KEY> --run-id <RUN_ID> --format json

# 3) 诊断（错误分类 → 原因 → 建议）
gitferry history +diagnose --run-id <RUN_ID> --format json

# 4) 交叉验证：配置是否漂移、备份是否超时
gitferry ops +drift --task <TASK_KEY> --format json
gitferry ops +rpo --format json
```

若已启用内置 AI 助手，也可用 `deep_analyze`：一次返回 failures / weak_dims / playbook / next_action。
低分维度解读：reliability→诊断与重试；freshness→查 cron 停摆；safety→查 force/漂移；schedule→补调度。

**决策表（常见 error_class）：**

| 现象 | 可能原因 | 下一步 |
|------|----------|--------|
| auth / 401 / 403 | 平台 token 过期或权限不足 | 让用户更新平台 token，再 `platform +test`（危险） |
| network / timeout | 网络或代理 | 检查平台 `proxy_url`；必要时重试 |
| non-fast-forward / rejected | 目标有独有提交（双向写入） | `ops +drift` 确认；与用户确认是否强推/合并策略 |
| repo not found | key 拼错或源站删除 | `repo +list` 核对 key |
| 连续失败且 drift 非空 | 本地 workdir 脏 | 用户同意后 `ops +rebuild`（危险） |

**修复后验证：** `history +retry`（危险，需确认）→ `history +list` 看是否转 success。

---

## 2. 新平台 / 新仓库接入

**目标：** 把一个仓库纳入同步。

```bash
# 1) 创建平台并测试连通（test 为危险）
gitferry platform +list --format json
gitferry platform +create --key gh-acme --type github --instance-url https://github.com ...
gitferry platform +test --key gh-acme --yes

# 2) 纳管仓库
gitferry repo +create --platform gh-acme --name acme/demo --clone-url https://github.com/acme/demo.git --yes
gitferry repo +list --keyword demo --format json   # 取真实 key

# 3) 建任务
gitferry task +create --key demo-mirror --name "demo 镜像" \
  --source-repo <SRC_KEY> --target-repo <DST_KEY> --cron "0 2 * * *" --yes

# 4) 首次手动跑通
gitferry task +run --key demo-mirror --yes
gitferry history +list --task demo-mirror --limit 3 --format json
```

---

## 3. 灾备演练

**目标：** 证明冷备可恢复。

```bash
# 只读预检
gitferry ops +integrity --format json
gitferry ops +rpo --format json

# 危险：真实恢复演练（必须用户确认）
gitferry ops +drill --name <BUNDLE_NAME> --yes

# 演练后核对
gitferry ops +audit --format json
```

**原则：** 演练默认写在隔离恢复目录；若用户环境不允许写盘，只做 `+integrity` / `+rpo` 并说明局限。

---

## 4. 日常巡检（只读，可定时）

```bash
gitferry ops +overview --format json
gitferry ops +health --format json
gitferry ops +inventory --format json
gitferry ops +rpo --max-seconds 86400 --format json
```

产出建议：按「失败任务 → 孤儿仓库 → RPO 超标 → 漂移」排序汇报，只报异常项。

---

## 5. 平台同步导入（拉取远端仓库清单）

```bash
gitferry platform +sync-repos --key gh-acme --yes
gitferry repo +list --format json
```

对新拉到的仓库批量建任务前，先 `ops +inventory` 看覆盖缺口。

---

## 6. 周巡检报告（只读，可出文档）

**目标：** 产出一份可粘贴到周报的异常摘要。

```bash
gitferry ops +overview --format json
gitferry ops +health --format json
gitferry ops +inventory --format json
gitferry ops +rpo --max-seconds 86400 --format json
gitferry ops +drift --format json
```

**报告骨架：**
1. 总览：任务成功/失败/运行中计数
2. 需关注：score < 80 或 level ≤ bronze 的任务，附 issues
3. 孤儿仓库：`inventory` 中 `no_task` 条目
4. 备份：RPO 超标项 + `integrity` 是否通过
5. 建议动作：3 条以内，每条对应一个 CLI 命令

只报异常，不复述正常项。数字给绝对值，避免“大幅下降”。

---

## 7. 备份恢复验收（灾备闭环）

```bash
# 1) 预检（只读）
gitferry ops +integrity --format json
gitferry ops +rpo --format json

# 2) 演练（危险，需确认）
gitferry ops +drill --name <BUNDLE> --yes

# 3) 验收点
gitferry ops +audit --format json
```

**验收标准：** drill 报告中 restore / fsck / refs 比对全部通过；否则按
[`gitferry-shared/references/troubleshooting.md`](../gitferry-shared/references/troubleshooting.md)
第 4 节处置，**不要**直接 rebuild。

---

## 8. 批量迁移 / 批量重试

```bash
# 先计划
gitferry task +batch-run --task-keys t1,t2,t3 --dry-run
gitferry ops +retry-batch --limit 10 --dry-run

# 用户点头后
gitferry task +batch-run --task-keys t1,t2,t3 --yes
```

批量场景必须：先 dry-run 汇报条数 → 用户确认 → 再 `--yes`。中途失败要列出
`results[]` 中 status=failed 的项，不要只报成功数。

---

## 通用原则

1. **先只读、后写入**：overview/health/detail/diagnose/schema 都可以随便跑
2. **危险步骤单独确认**：不要把 `--yes` 和多条危险命令拼在一条里跑
3. **用真实 key**：`<TASK_KEY>` / `<RUN_ID>` 必须来自 list/detail 输出
4. **失败要汇报**：任何一步 envelope `ok=false`，停下来把 `error` 原样告诉用户
5. **批量先 dry-run**：见工作流 8
6. **出错先查表**：[`gitferry-shared/references/troubleshooting.md`](../gitferry-shared/references/troubleshooting.md)

## References

- [gitferry-shared](../gitferry-shared/SKILL.md) · [flags](../gitferry-shared/references/flags.md) · [troubleshooting](../gitferry-shared/references/troubleshooting.md)
- [gitferry-history](../gitferry-history/SKILL.md)
- [gitferry-ops](../gitferry-ops/SKILL.md)
- [gitferry-task](../gitferry-task/SKILL.md)
- [gitferry-repo](../gitferry-repo/SKILL.md)
