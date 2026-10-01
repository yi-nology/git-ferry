---
name: gitferry-task
version: 1.0.0
description: "GitFerry 同步任务：列表、详情、创建、更新、删除、立即执行、同步预览。当用户要管理或触发同步任务时触发。"
metadata:
  requires:
    bins: ["gitferry"]
  cliHelp: "gitferry task --help"
---

# gitferry-task（同步任务）

**前置：** 先读 [`../gitferry-shared/SKILL.md`](../gitferry-shared/SKILL.md)。
**`+run` / `+delete` / `+create` / `+update` 为写或触发操作，必须先确认用户意图。**

## Shortcuts

| Shortcut | 说明 | 危险 |
|----------|------|------|
| `task +list` | 任务列表 | 否 |
| `task +info` | 任务详情 | 否 |
| `task +create` | 创建任务 | 是（写） |
| `task +update` | 更新任务 | 是（写） |
| `task +delete` | 删除任务 | **是** |
| `task +run` | 立即执行一次同步 | **是**（触发执行） |
| `task +preview` | 同步预览（按源/目标分支，不落盘） | 否（只读预览） |

## 使用示例

```bash
# 任务列表
gitferry task +list --format json

# 任务详情
gitferry task +info --key daily-mirror --format json

# 立即执行（危险：默认会确认）
gitferry task +run --key daily-mirror
# 脚本场景
gitferry task +run --key daily-mirror --yes

# 创建任务（写操作，先确认字段）
gitferry task +create \
  --name "每日镜像" \
  --source-repo github/acme-demo \
  --source-branch main \
  --target-repo gitlab/infra-demo \
  --target-branch main \
  --sync-mode all \
  --cron "0 2 * * *" --yes

# 更新任务
gitferry task +update --key daily-mirror --cron "0 3 * * *" --yes

# 同步预览（按源/目标仓库+分支，不落盘）
gitferry task +preview \
  --source-repo github/acme-demo --source-branch main \
  --target-repo gitlab/infra-demo --target-branch main --format json
```

## 参数

### task +list

| 参数 | 必填 | 说明 |
|------|------|------|
| `--keyword` | 否 | 名称过滤 |
| `--status` | 否 | 状态过滤 |
| `--page` / `--limit` | 否 | 分页 |

### task +info / +run / +delete

| 参数 | 必填 | 说明 |
|------|------|------|
| `--key` | 是 | 任务 key |

### task +create（核心字段）

| 参数 | 必填 | 说明 |
|------|------|------|
| `--name` | 是 | 显示名 |
| `--source-repo` | 是 | 源仓库 key |
| `--target-repo` | 是 | 目标仓库 key |
| `--source-branch` | 否 | 默认 `main` |
| `--target-branch` | 否 | 默认 `main` |
| `--sync-mode` | 否 | `all` / `single` |
| `--cron` | 否 | cron 表达式；不填则仅手动 |
| `--git-tags` / `--git-force` / `--git-prune` / `--git-lfs` / `--git-push-prune` | 否 | 同步开关 |

### task +update

| 参数 | 必填 | 说明 |
|------|------|------|
| `--key` | 是 | 任务 key |
| `--name` / `--source-branch` / `--target-branch` / `--sync-mode` / `--cron` | 否 | 仅改传入字段 |
| `--enabled` | 否 | 启停（默认 true，仅在传入时写入） |

### task +preview

| 参数 | 必填 | 说明 |
|------|------|------|
| `--source-repo` / `--target-repo` | 是 | 两端仓库 key |
| `--source-branch` / `--target-branch` | 否 | 默认 `main` |

## API 映射

| Shortcut | HTTP |
|----------|------|
| `task +list` | `GET /api/v1/sync/tasks` |
| `task +info` | `GET /api/v1/sync/task?key=` |
| `task +create` | `POST /api/v1/sync/task/create` |
| `task +update` | `POST /api/v1/sync/task/update` |
| `task +delete` | `POST /api/v1/sync/task/delete?key=` |
| `task +run` | `POST /api/v1/sync/task/run?key=` |
| `task +preview` | `POST /api/v1/sync/preview` |

## 返回关键字段

| 字段 | 说明 |
|------|------|
| `key` / `name` | 任务标识 |
| `source_repo` / `target_repo` | 两端仓库 key |
| `cron` | 调度表达式 |
| `status` | 启停状态 |
| `last_run` / `last_status` | 最近执行 |

## 注意事项

- **`task +run` 会立刻占用执行槽并推送代码**，未获用户明确同意不要执行，也不要默认加 `--yes`
- 创建/更新前先 `repo +list` 确认 source/target 的 `key` 真实存在
- 执行结果是异步的：`+run` 返回受理后，用 [`gitferry-history`](../gitferry-history/SKILL.md) 查落地情况

## References

- [gitferry-history](../gitferry-history/SKILL.md)
- [gitferry-repo](../gitferry-repo/SKILL.md)
- [gitferry-shared](../gitferry-shared/SKILL.md)
