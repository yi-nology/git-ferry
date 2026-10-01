---
name: gitferry-repo
version: 1.0.0
description: "GitFerry 仓库管理：列出、查看、分支查询、连通性测试。当用户要查同步仓库、看仓库状态/平台、列分支时触发。"
metadata:
  requires:
    bins: ["gitferry"]
  cliHelp: "gitferry repo --help"
---

# gitferry-repo（仓库操作）

**前置：** 先读 [`../gitferry-shared/SKILL.md`](../gitferry-shared/SKILL.md)（认证、Envelope、安全规则）。
**写操作 / 连通性测试前必须确认用户意图。**

## Shortcuts

| Shortcut | 说明 | 危险 |
|----------|------|------|
| `repo +list` | 仓库列表，支持关键字过滤、分页 | 否 |
| `repo +info` | 单个仓库详情（状态、平台、分支数） | 否 |
| `repo +branches` | 仓库在源平台的分支列表 | 否 |
| `repo +test` | 测试仓库连通性 | **是**（需确认） |
| `repo +create` | 创建仓库记录 | 是（写） |
| `repo +delete` | 删除仓库记录 | **是**（写） |

## 使用示例

```bash
# 列出仓库（Agent 用 json）
gitferry repo +list --format json

# 关键字过滤 + 分页
gitferry repo +list --keyword demo --page 1 --limit 20 --format json

# 仓库详情
gitferry repo +info --key github.com/acme/demo --format json

# 列分支
gitferry repo +branches --key github.com/acme/demo --format json

# 测试连通性（危险：先确认）
gitferry repo +test --key github.com/acme/demo --yes
```

## 参数

### repo +list

| 参数 | 必填 | 说明 |
|------|------|------|
| `--keyword` | 否 | 名称/地址模糊过滤 |
| `--platform` | 否 | 按平台 key 过滤 |
| `--page` | 否 | 页码，默认 1 |
| `--limit` | 否 | 每页条数，默认 20 |
| `--format` | 否 | `json` / `table` |

### repo +info / +branches / +test / +delete

| 参数 | 必填 | 说明 |
|------|------|------|
| `--key` | 是 | 仓库 key（先用 `repo +list` 获取） |

## API 映射

| Shortcut | HTTP |
|----------|------|
| `repo +list` | `GET /api/v1/repos?keyword=&page=&per_page=` |
| `repo +info` | `GET /api/v1/repo?key=` |
| `repo +branches` | `GET /api/v1/repo/branches?key=` |
| `repo +test` | `POST /api/v1/repo/test` |
| `repo +create` | `POST /api/v1/repo/create` |
| `repo +delete` | `DELETE /api/v1/repo/delete` |

Raw API 兜底：

```bash
gitferry api GET '/api/v1/repos?page=1&per_page=5' --format json
```

## 返回关键字段

| 字段 | 说明 |
|------|------|
| `key` | 仓库唯一键（后续操作主键） |
| `name` | 名称 |
| `platform` | 所属平台 key |
| `status` | 状态（active 等） |
| `clone_url` | 克隆地址 |
| `has_task` | 是否已有同步任务覆盖（配合 ops 用） |

## 注意事项

- **不要猜 `--key`**：key 形如 `github.com/acme/demo`，必须从 `repo +list` 取
- 仓库「删除」通常只是解除纳管记录，不删源站代码；仍属写操作，需确认
- 分支列表来自源平台实时查询，网络异常会返回错误 envelope

## References

- [gitferry-shared](../gitferry-shared/SKILL.md)
- [gitferry-task](../gitferry-task/SKILL.md)
