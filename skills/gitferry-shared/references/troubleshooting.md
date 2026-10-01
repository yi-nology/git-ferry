# 排障对照表（gitferry）

> 按 HTTP / Envelope 错误码 → 可能原因 → 处置动作。Agent 遇错先查本表。

## 1. 认证与连通

| 现象 | 可能原因 | 处置 |
|------|----------|------|
| `ok=false` code 401 | 未设置 / 错误的 API Key | `export GITFERRY_TOKEN=...` 或 `gitferry auth login` |
| code 403 | Key 角色只读 / 权限不足 | 确认 `GIT_SYNC_API_KEY_ROLE`（admin/operator/readonly） |
| `reachable: false` / 连接失败 | 服务未启动、base_url 错、代理 | `curl $GITFERRY_BASE_URL/health`；查端口 8890 |
| 改了配置仍 401 | 环境变量覆盖了配置文件 | `env \| grep GITFERRY`；`gitferry config list` |

## 2. 资源定位

| 现象 | 可能原因 | 处置 |
|------|----------|------|
| code 404 `task not found` | key 拼错 / 任务已删 | `gitferry task +list --format json` 取真实 key |
| 仓库 key 格式不对 | 不是 `platform/name` 形式 | `gitferry repo +list` 看 `key` 字段 |
| `run_id` 无效 | 用了页面序号而非历史 ID | `gitferry history +list --task <K>` 看 `id` |

## 3. 同步失败（error_class 启发式）

| error_class / 关键词 | 原因 | 处置 |
|----------------------|------|------|
| `auth` / 401 / 403 / permission | 平台 token 过期或权限不足 | 用户更新平台 token → `platform +test`（危险） |
| `network` / timeout / dial | 网络/代理/防火墙 | 查 `proxy_url`；稍后 `history +retry` |
| `non-fast-forward` / rejected | 目标有独有提交 | `ops +drift`；与用户确认合并/强推策略 |
| `repo not found` | 源/目标被删或 key 错 | `repo +info --key ...` |
| `lfs` | LFS 对象缺失 | 开 `--git-lfs` 或清 LFS 缓存 |
| 连续失败 + drift 非空 | workdir 脏/半截状态 | 用户同意后 `ops +rebuild --task <K> --yes` |

## 4. 运维与备份

| 现象 | 原因 | 处置 |
|------|------|------|
| RPO 超标 | cron 停 / 失败堆积 | `ops +health` → `history +list` → 修任务 |
| `integrity` 失败 | 冷备损坏或清单不一致 | 不要 rebuild；先 `ops +drill` 隔离验证 |
| 孤儿仓库（inventory） | 有仓库无任务 | `task +create` 覆盖或标记归档 |
| 审计链 verify 失败 | 日志被改/截断 | 对照 `logs/operations` 时间线，升级前备份 db |

## 5. CLI 本身

| 现象 | 处置 |
|------|------|
| `gitferry: command not found` | `make build-cli && cp output/gitferry /usr/local/bin/` 或 `npm i -g gitferry-cli` |
| npm 装了但缺二进制 | 见 postinstall 日志；或从 Releases 手动下载 |
| `--format table` 列空白 | 数据字段不在 preferred 列；换 `--format json` |
| `schema show` 无结果 | `schema list` 看真实域名；关键字用路径片段如 `diagnose` |

## 6. 危险操作门

未加 `--yes` 时返回：

```json
{"ok": false, "error": {"code": 409, "message": "需要确认: ...", "suggestion": "危险操作需确认；脚本场景可加 --yes"}}
```

处置：**先向用户复述将要发生的副作用**，拿到明确同意再加 `--yes`。禁止为了“跑通”自动加 `--yes`。

## 7. 安全红线

- 不要把 `GITFERRY_TOKEN` / `X-API-Key` 打进日志、回复、截图
- `--debug` 输出可能含 URL 参数，转发前先脱敏
- 配置文件权限保持 `0600`；macOS 优先 Keychain
