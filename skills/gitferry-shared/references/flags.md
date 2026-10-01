# 常用参数与易踩坑（gitferry）

> 和 [`troubleshooting.md`](troubleshooting.md) 搭配：这里讲参数语义，那里讲出错怎么修。

## 全局

| 参数 | 说明 | 坑 |
|------|------|----|
| `--format json\|table\|yaml` | 输出格式 | Agent 一律 `json`；`table` 列是启发式，宽表请用 json |
| `--yes` | 跳过危险确认 | 只在用户明确授权后使用；不要默认加上 |
| `--base-url` | 覆盖服务地址 | 优先级：flag > `GITFERRY_BASE_URL` > 配置文件 |
| `--all` | 列表拉全分页 | 大实例上会慢；先 `--limit 20` 试探 |

## 资源 key

- 仓库：`platform/name`，如 `github/acme-api` —— **必须来自 `repo +list`**
- 任务：短 slug，如 `docs-mirror` —— 来自 `task +list`
- run_id：整数，来自 `history +list` 的 `id`，不是页面序号

## 任务字段

| 参数 | 坑 |
|------|----|
| `--source-branch` / `--target-branch` | 默认 `main`；文档仓可能是 `docs` |
| `--sync-mode all\|single` | `all` 推所有分支；`single` 只推两分支 |
| `--git-force` | **覆盖目标历史**，必须用户点头 |
| `--cron` | 空 = 仅手动；服务端另有 `runwatch` 自动重跑 |

## 批量

```bash
# 先看计划
gitferry task +batch-run --task-keys a,b --dry-run
# 用户同意后再执行
gitferry task +batch-run --task-keys a,b --yes
```

`--dry-run` **不会** 跳过危险语义检查，但也不会发请求；去掉 dry-run 后仍要 `--yes`。

## 自省

```bash
gitferry schema list
gitferry schema show ops
gitferry schema show diagnose
gitferry api GET /api/v1/ops/health-score --format json   # 路径不确定时先 schema
```

## 密钥注入（部署方）

| 变量 | 说明 |
|------|------|
| `GIT_SYNC_TOKEN_<KEY>` | 直接注入平台 token（KEY=平台 key 大写、`-`→`_`） |
| `GIT_SYNC_TOKEN_CMD_<KEY>` | 执行命令取 token（`op read` / `vault kv get` 等），stdout 首行，5s 超时 |
| `GITFERRY_TOKEN` | CLI 访问本服务的 API Key |

优先级：`TOKEN_CMD` > `TOKEN` > 配置/DB 中的值。

## 审计导出

```bash
gitferry ops +audit-report --csv --limit 200
gitferry task +batch-run --task-keys a,b --csv
```

## 配置优先级

```
flag > 环境变量（GITFERRY_*） > OS Keychain(api_key) > ~/.config/gitferry/config.yaml
```

`gitferry config list` 可看最终生效值（api_key 脱敏）。
