# GitFerry 前端

Vue 3 + Vite + TypeScript + Ant Design Vue 的自托管运维控制台。

## 视觉方向

冷静运维台（GitHub Enterprise / Linear）：

- **中性优先**：画布 `#f6f8fa`，表面白色描边分层，几乎不用阴影
- **语义克制**：主色 `#2563eb` 只用于可交互/主操作；成功/警告/危险用低饱和绿/黄/红
- **信息密度**：指标条（MetricStrip）替代彩色大图标统计卡；失败任务优先上浮
- **统一页面骨架**：`PageHeader`（标题 + 副标题 + 操作）+ 筛选栏 + 内容卡

设计 token 见 `src/styles/variables.scss`，Ant Design 主题见 `src/styles/theme.ts`。

## 技术栈

| 层 | 选型 |
|----|------|
| 框架 | Vue 3.4 (`<script setup>` + TS) |
| 构建 | Vite 5 |
| UI | Ant Design Vue 4 + `@ant-design/icons-vue` |
| 路由 | vue-router 4 |
| 状态 | Pinia（鉴权/仓库/任务/Webhook） |
| 请求 | axios（`{code,data}` 自动解包）+ vue-query |
| 样式 | SCSS tokens + 全局工具类 |

## 开发

```bash
cd frontend
npm install
npm run dev          # http://localhost:5174，代理 /api → :8890
npm run build        # 产出 dist/
npm run build:check  # vue-tsc + build
```

## 目录结构

```
frontend/src/
├── api/            # 接口封装（http、repo、sync、ops、ai、aiConfig、mirror…）
├── components/
│   ├── ai/         # AI 助手面板（AssistantPanel）
│   ├── common/     # PageHeader / MetricStrip / StatusBadge
│   └── layout/     # AppLayout / AppSider / AppHeader
├── composables/    # useAIChat / useMirror / useRepos / usePlatformSync
├── constants/      # 状态字典、平台预设、webhook
├── router/         # 路由与鉴权守卫
├── stores/         # auth / repo / syncTask / webhook
├── styles/         # variables.scss / theme.ts / global.scss
├── types/          # API 与领域类型
├── utils/          # notify、筛选器、平台展示
└── views/
    ├── dashboard/  # 仪表盘
    ├── sync/       # 任务列表 / 执行记录 / 新建任务
    ├── repos/      # 仓库列表 / 详情 / 统一配置
    ├── mirror/     # 镜像通道
    ├── webhook/    # 规则 / 事件
    ├── ops/        # 运维中心（概览/健康/盘点/模板/密钥/冷备）
    ├── logs/       # 操作日志
    ├── settings/   # AI 助手配置 / 平台管理
    └── login/
```

## 侧栏信息架构

```
仪表盘
管理     同步任务 · 执行记录 · 仓库管理 · 镜像中心
自动化   Webhook 规则 · Webhook 事件
运维     运维中心 · 操作日志
系统     AI 助手 · 平台管理
```

## 页面模式

列表页统一为：

1. `PageHeader` — 标题 / 副标题 / 主次操作（刷新、创建）
2. `MetricStrip` — 3～4 个关键指标（失败用 danger 色）
3. 筛选栏 — 搜索 + 下拉 + 重置
4. `content-card` 包表格 — 表头浅灰、行分隔细线

仪表盘额外有「需要关注」失败任务条；登录页品牌统一为 GitFerry。

## AI 助手

- **入口**：右下角浮动球 + 顶栏机器人（仅 `enabled` 时显示）
- **配置**：`/settings/ai` — 服务预设、Base URL、模型、API Key、温度/超时等；保存热生效
- **对话**：SSE 流式；工具调用显示为 chip；危险操作弹确认卡

相关 API：`GET/POST /api/v1/ai/config`、`POST /api/v1/ai/config/test`、`GET /api/v1/ai/status`、`POST /api/v1/ai/chat`。

## 截图

见仓库 [`docs/screenshots/`](../docs/screenshots/)：登录、仪表盘、同步任务、执行记录、仓库、AI 面板、AI 配置、Webhook、运维中心。
