# task +create

> **前置：** 先读 [`../../gitferry-shared/SKILL.md`](../../gitferry-shared/SKILL.md)。
> **写操作：执行前必须确认用户意图。**

创建同步任务。仓库 key 必须先用 `gitferry repo +list` 真实获取。

## 命令

```bash
gitferry task +create \
  --name "每日镜像" \
  --source-repo github/acme-demo \
  --source-branch main \
  --target-repo gitlab/infra-demo \
  --target-branch main \
  --sync-mode all \
  --cron "0 2 * * *" --yes
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--name` | 是 | 任务显示名 |
| `--source-repo` | 是 | 源仓库 key |
| `--target-repo` | 是 | 目标仓库 key |
| `--source-branch` | 否 | 默认 `main` |
| `--target-branch` | 否 | 默认 `main` |
| `--sync-mode` | 否 | `all`（全量）/ `single`（单分支） |
| `--cron` | 否 | cron；不填仅手动触发 |
| `--git-tags` | 否 | 同步 tags |
| `--git-force` | 否 | 允许 force push（危险，需用户明确） |
| `--git-prune` | 否 | prune |
| `--git-lfs` | 否 | 同步 LFS |
| `--git-push-prune` | 否 | push --prune |
| `--yes` | 否 | 跳过确认（仅脚本） |

## API

```
POST /api/v1/sync/task/create
{
  "name": "...",
  "source_repo_key": "...",
  "source_branch": "main",
  "target_repo_key": "...",
  "target_branch": "main",
  "sync_mode": "all",
  "cron": "0 2 * * *",
  "git_tags": false,
  "git_force": false,
  "git_prune": false,
  "git_lfs": false,
  "git_push_prune": false
}
```

## 安全

- `--git-force` 可能覆盖目标分支历史，必须先和用户确认策略
- 创建后先 `task +preview` 或 `task +run`（危险）做一次验证

## References

- [gitferry-task](../SKILL.md)
- [gitferry-repo](../../gitferry-repo/SKILL.md)
