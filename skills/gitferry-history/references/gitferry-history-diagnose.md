# history +diagnose

> **前置：** 先读 [`../../gitferry-shared/SKILL.md`](../../gitferry-shared/SKILL.md)。

对一次失败（或任意）执行做规则化诊断：错误分类 → 可能原因 → 建议动作。只读。

## 命令

```bash
gitferry history +diagnose --run-id 42 --format json
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--run-id` | 是 | 执行记录 ID（来自 `history +list`） |
| `--format` | 否 | `json` / `table` |

## API

```
GET /api/v1/ops/diagnose?run_id={run_id}
```

## 返回字段

| 字段 | 说明 |
|------|------|
| `run_id` | 执行 ID |
| `task_key` | 所属任务 |
| `error_class` / `error_msg` | 错误分类与信息 |
| `cause` | 可能原因 |
| `actions[]` | 建议动作（可直接对应 Shortcuts） |
| `related[]` | 同任务相邻执行，便于看是否连续失败 |

## 用法建议

1. 诊断结果里的 `actions` 优先于自己猜测
2. 若指向 token/权限 → 让用户更新平台凭据后 `platform +test`（危险）
3. 若指向 workdir 漂移 → 用户同意后 `ops +rebuild`（危险）
4. 修复后 `history +retry --run-id <ID>`（危险）并用 `+list` 验证

## References

- [gitferry-history](../SKILL.md)
- [gitferry-workflow](../../gitferry-workflow/SKILL.md)
