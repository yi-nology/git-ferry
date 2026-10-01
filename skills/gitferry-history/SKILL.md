---
name: gitferry-history
version: 1.0.0
description: "GitFerry 执行历史：同步记录列表、单次执行详情、失败诊断、重试。当用户要查同步是否成功、排查失败原因、重跑任务时触发。"
metadata:
  requires:
    bins: ["gitferry"]
  cliHelp: "gitferry history --help"
---

# gitferry-history（执行历史与诊断）

**前置：** 先读 [`../gitferry-shared/SKILL.md`](../gitferry-shared/SKILL.md)。
**`history +retry` 会重新触发同步，必须先确认用户意图。**

## Shortcuts

| Shortcut | 说明 | 危险 |
|----------|------|------|
| `history +list` | 执行历史列表（可按任务过滤） | 否 |
| `history +detail` | 单次执行详情（步骤链、错误链） | 否 |
| `history +diagnose` | 失败诊断：错误分类 → 原因 → 建议 | 否 |
| `history +retry` | 重试一次失败执行 | **是** |

## 使用示例

```bash
# 某任务最近执行
gitferry history +list --task daily-mirror --limit 10 --format json

# 一次执行的完整步骤（按 run-id 过滤）
gitferry history +detail --task daily-mirror --run-id 42 --format json

# 失败诊断（推荐排查第一步）
gitferry history +diagnose --run-id 42 --format json

# 重试（危险：先确认）
gitferry history +retry --run-id 42
```

## 参数

### history +list

| 参数 | 必填 | 说明 |
|------|------|------|
| `--task` | 否 | 任务 key；不填查全部 |
| `--limit` | 否 | 条数，默认 20 |

### history +detail

| 参数 | 必填 | 说明 |
|------|------|------|
| `--run-id` | 是 | 执行记录 ID（来自 `+list`） |
| `--task` | 否 | 任务 key，进一步收窄 |

### history +diagnose / +retry

| 参数 | 必填 | 说明 |
|------|------|------|
| `--run-id` | 是 | 执行记录 ID |

## API 映射

| Shortcut | HTTP |
|----------|------|
| `history +list` | `GET /api/v1/sync/history?task_key=&limit=` |
| `history +detail` | `GET /api/v1/sync/history?task_key=&run_id=` |
| `history +diagnose` | `GET /api/v1/ops/diagnose?run_id=` |
| `history +retry` | `POST /api/v1/ops/retry` body `{"run_id": N}` |

## 返回关键字段

| 字段 | 说明 |
|------|------|
| `id` / `run_id` | 执行记录 ID |
| `task_key` | 任务 |
| `status` | success / failed / running |
| `started_at` / `finished_at` | 起止时间 |
| `steps[]` | 步骤链（fetch/push/...） |
| `error` / `error_class` | 错误信息与分类 |
| `diagnosis`（diagnose） | 原因 + 建议动作列表 |

## 排查建议顺序

1. `history +list --task <key> --status failed` — 确认是否连续失败
2. `history +detail` — 看卡在哪一步
3. `history +diagnose` — 拿错误分类与建议
4. 按建议决定：改配置 → `task +update`，或用户同意后 `history +retry`
5. 反复失败：转 [`gitferry-workflow`](../gitferry-workflow/SKILL.md) 的「失败根因排查」

## 注意事项

- **`+retry` 会真实推送**，未确认不要执行
- `run_id` 必须来自真实历史，不要用页面上猜测的数字
- 诊断只读，可随时执行

## References

- [gitferry-task](../gitferry-task/SKILL.md)
- [gitferry-ops](../gitferry-ops/SKILL.md)
- [gitferry-shared](../gitferry-shared/SKILL.md)
