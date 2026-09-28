# GitFerry UI 说明

> 历史静态设计稿（`index.html` / `history.html` / `create-task.html`）已由生产前端取代。
> **现行 UI 与截图以 [`docs/screenshots/`](../docs/screenshots/) 与 [`frontend/`](../frontend/) 为准。**

## 现行界面

| 页面 | 截图 | 说明 |
|------|------|------|
| 登录 | `docs/screenshots/login.png` | GitFerry 品牌、API Key 登录 |
| 仪表盘 | `docs/screenshots/dashboard.png` | 指标条 + 失败关注 + 最近任务/仓库 |
| 同步任务 | `docs/screenshots/sync-tasks.png` | 列表 / 筛选 / 批量 / CRUD |
| 执行记录 | `docs/screenshots/sync-records.png` | 历史、状态、详情抽屉 |
| 仓库管理 | `docs/screenshots/repos.png` | 平台卡片、连接测试 |
| AI 助手 | `docs/screenshots/ai-assistant.png` | 浮动球对话面板 |
| AI 配置 | `docs/screenshots/ai-settings.png` | 模型 / Base URL / API Key |
| Webhook 规则 | `docs/screenshots/webhook-rules.png` | 触发规则管理 |
| 运维中心 | `docs/screenshots/ops.png` | 概览 / 健康 / 盘点 / 模板 |

## 设计原则

1. **中性优先**：画布浅灰、表面白卡描边，主色只用于可点击
2. **失败优先**：仪表盘「需要关注」先于快捷入口
3. **统一骨架**：PageHeader → MetricStrip → 筛选 → 内容卡表格
4. **侧栏分组**：管理 / 自动化 / 运维 / 系统

完整 token 与组件说明见 [`frontend/README.md`](../frontend/README.md)。

## 本目录遗留文件

| 文件 | 状态 |
|------|------|
| `styles.css` | 历史设计稿共享样式，生产前端不引用 |
| `index.html` / `history.html` / `create-task.html` | 早期任务列表/历史/向导草稿 |
| `webhook-flow.html` | Webhook 流程示意 |
| `research/` | 调研笔记 |
