# repo +list

> **前置：** 先读 [`../../gitferry-shared/SKILL.md`](../../gitferry-shared/SKILL.md)。

列出纳管的同步仓库，支持关键字过滤与分页。

## 命令

```bash
gitferry repo +list --format json
gitferry repo +list --keyword demo --platform github --page 1 --limit 20 --format json
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--keyword` | 否 | 名称 / 地址模糊匹配 |
| `--platform` | 否 | 平台 key 过滤 |
| `--page` | 否 | 页码，默认 1 |
| `--limit` | 否 | 每页条数，默认 20 |
| `--format` | 否 | `json` / `table`（Agent 用 `json`） |

## API

```
GET /api/v1/repos?keyword={keyword}&page={page}&per_page={limit}
```

## 返回字段

| 字段 | 说明 |
|------|------|
| `items[].key` | 仓库主键 |
| `items[].name` | 名称 |
| `items[].platform` | 平台 |
| `items[].status` | 状态 |
| `items[].clone_url` | 克隆地址 |
| `meta.total_count` | 总数 |

## References

- [gitferry-repo](../SKILL.md)
- [gitferry-shared](../../gitferry-shared/SKILL.md)
