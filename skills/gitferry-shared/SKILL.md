---
name: gitferry-shared
version: 1.0.0
description: "GitFerry CLI 共享基础：认证（API Key / 环境变量）、全局参数、Envelope 输出、安全规则。当用户首次使用 gitferry、遇到 401/403、或需要理解输出格式时触发。"
metadata:
  requires:
    bins: ["gitferry"]
  cliHelp: "gitferry --help"
---

# gitferry 共享规则

本技能指导你如何通过 `gitferry` CLI 操作 GitFerry（Git 多平台同步 / 镜像 / 备份服务）。

**开始任何领域操作前，先读完本文件。**

## 认证

GitFerry API 使用 **X-API-Key** 头鉴权（OIDC Bearer 由服务端配置，CLI 不处理登录流）。

### 配置方式（优先级从高到低）

1. 环境变量（CI / Agent 推荐）：

```bash
export GITFERRY_BASE_URL="http://127.0.0.1:8890"   # 默认值，可省略
export GITFERRY_TOKEN="your-api-key"                # 或 GITFERRY_API_KEY
```

2. 配置文件 `~/.config/gitferry/config.yaml`：

```yaml
base_url: http://127.0.0.1:8890
api_key: your-api-key
format: json
```

写入配置：

```bash
gitferry auth login --token "$GITFERRY_TOKEN"
gitferry auth login --base-url http://127.0.0.1:8890
```

### 状态检查

```bash
gitferry auth status
```

### 认证错误处理

| 状态 | 含义 | 动作 |
|------|------|------|
| 401 | API Key 缺失或错误 | 引导设置 `GITFERRY_TOKEN` 或 `gitferry auth login` |
| 403 | 角色权限不足（readonly/operator/admin） | 确认 key 的 `GIT_SYNC_API_KEY_ROLE`，只读 key 不能做写操作 |
| 连接失败 | 服务未启动或 base_url 错误 | 检查 `GITFERRY_BASE_URL`，确认服务 `GET /health` 可达 |

## 全局参数

| 参数 | 说明 |
|------|------|
| `--base-url` | 服务基址（默认读 `GITFERRY_BASE_URL`） |
| `--format` | `json` / `table`（**Agent 场景始终用 `json`**） |
| `--debug` | 打印请求/响应调试信息（**不要在含密钥的场景开启后回传给用户**） |
| `--yes` | 危险操作跳过交互确认（仅脚本/自动化使用） |

## 输出格式（Envelope）

所有命令输出统一为：

```json
{
  "ok": true,
  "data": { },
  "meta": { "page": 1, "limit": 20, "total_count": 100 }
}
```

错误：

```json
{
  "ok": false,
  "error": {
    "code": 404,
    "message": "task not found",
    "suggestion": "用 gitferry task +list 查看可用任务 key"
  }
}
```

**解析约定：**
- 先看 `ok`；`ok=false` 时读 `error.suggestion` 决定下一步
- 列表类结果在 `data.items`（或 `data.<plural>`），分页信息在 `meta`
- AI 场景建议：`gitferry <cmd> --format json | jq .`

## 三层命令体系

| 层级 | 格式 | 示例 | 适用 |
|------|------|------|------|
| Shortcuts | `gitferry <domain> +<verb>` | `gitferry task +list` | 高频操作，优先使用 |
| Raw API | `gitferry api <METHOD> <PATH>` | `gitferry api GET /api/v1/system/status` | Shortcuts 未覆盖的端点 |

完整 REST 面见服务端 `docs/openapi.json`。

## 安全规则（必须遵守）

1. **禁止**把 API Key / token 打印到日志、回复、PR、截图
2. **写入 / 删除 / 触发执行前必须确认用户意图**；未明确要求时不要加 `--yes`
3. 危险命令清单：`task +run`、`task +delete`、`history +retry`、`ops +drill`、`ops +rebuild`、`platform +delete`、`repo +delete`
4. 遇到 `confirm_required` 类响应时，**把确认卡片原样展示给用户**，拿到确认后再执行
5. 不要猜测任务/仓库 key：先 `gitferry task +list` / `repo +list` 取真实 key

## 常见排查顺序

1. `gitferry auth status` — 认证与连通
2. `gitferry ops +overview --format json` — 服务是否健康
3. 按领域进入对应 Skill（repo / task / history / ops）

## References

- [flags 与易踩坑](references/flags.md)
- [troubleshooting 排障对照](references/troubleshooting.md)
- [gitferry-repo](../gitferry-repo/SKILL.md)
- [gitferry-task](../gitferry-task/SKILL.md)
- [gitferry-history](../gitferry-history/SKILL.md)
- [gitferry-ops](../gitferry-ops/SKILL.md)
- [gitferry-workflow](../gitferry-workflow/SKILL.md)
