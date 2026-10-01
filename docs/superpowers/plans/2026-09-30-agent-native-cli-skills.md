# 对照 gitlink-cli 的学习与改造方案

> 参考项目：[ccfos/gitlink-cli](https://github.com/ccfos/gitlink-cli)（GitLink 官方 CLI，Agent-Native 设计）
> 本仓：GitFerry（git-ferry）— Git 多平台同步 / 镜像 / 备份服务
> 日期：2026-09-30

## 1. 参考项目在做什么

gitlink-cli 是给 GitLink 平台用的命令行工具，核心不是「又一个 CLI」，而是**人和 AI Agent 双受众的运维入口**：

1. **三层命令体系**
   - Shortcuts：`gitlink-cli repo +list`，高频语义封装，声明式定义（`Shortcut{Flags, Run}`）
   - Raw API：`gitlink-cli api GET /path`，覆盖全部端点的逃生舱
   - Config：`~/.config/gitlink-cli/config.yaml`
2. **统一输出 Envelope**：`{ok, data, error, meta}`，`--format json|table|yaml`，错误带 `suggestion`（给 Agent 的下一步提示）
3. **Skills 外置包**：`skills/gitlink-*/SKILL.md + references/`，Claude Code 等外部 Agent 零配置即可调用
4. **认证可移植**：OS Keychain + `GITLINK_TOKEN` 环境变量（CI/非交互）
5. **上下文自动解析**：git remote → owner/repo，少打参数

## 2. 本项目现状对照

| 维度 | gitlink-cli | GitFerry 现状 | 差距 |
|------|-------------|---------------|------|
| 形态 | 纯 CLI | Web 服务 + Vue 控制台 | 无脚本化入口 |
| Agent 接入 | 11 个 Skills 包 | eino 内嵌 29 工具（`internal/agent/tools/`） | 能力锁在进程内，外部 Agent 用不了 |
| 输出契约 | Envelope | hz `response` 包，各 handler 风格不一 | Agent 解析成本高 |
| 认证 | Keychain / env token | Web 登录、API Key、OIDC | CLI 需要可脚本化的 token 路径 |
| 分发 | npm + install-skills | Docker / 源码 | 缺一键装 Skills |
| 命令面 | 40+ shortcuts | 70+ REST 端点（无快捷层） | API 全但难记 |

本项目已有可复用资产：`docs/openapi.json`（完整 API 面）、`internal/agent/tools/`（能力语义与危险操作分级）、`internal/pkg/response`、鉴权中间件。

## 3. 改造目标

把 GitFerry 从「只有 Web 控制台的同步服务」补成 **Agent-Native 运维入口**：

1. **Skills 外置包** — 外部 Agent（Claude Code / MiMo / Cursor）可直接运维 GitFerry
2. **CLI（gitferry）** — 脚本、CI、无浏览器场景；Shortcuts + Raw API 双层
3. **统一 Envelope** — CLI/API 对 Agent 输出契约一致

非目标（本期不做）：npm 分发、OS Keychain（先用 env + 配置文件）、改写 eino 内嵌工具。

## 4. 分步计划

### P0 — 方案文档（本文档）
产出：对照结论、命令面规划、Skills 目录规划、验收标准。

### P1 — Skills 外置包 `skills/`

```
skills/
├── README.md                 # 安装与使用（npx skills add / 手动复制）
├── gitferry-shared/          # 认证、基址、Envelope、安全规则
│   └── SKILL.md
├── gitferry-repo/            # 仓库查询 / 分支
│   ├── SKILL.md
│   └── references/
├── gitferry-task/            # 同步任务 CRUD / 手动触发
│   ├── SKILL.md
│   └── references/
├── gitferry-history/         # 执行历史 / 失败诊断
│   └── SKILL.md
├── gitferry-ops/             # 运维中心：健康/盘点/RPO/漂移/审计
│   └── SKILL.md
└── gitferry-workflow/        # 复合工作流：失败排查、灾备演练、平台接入
    └── SKILL.md
```

要点（学 gitlink-cli）：
- frontmatter：`name` / `description`（触发条件写清）/ `metadata.requires.bins`
- 默认 `--format json`，危险操作必须先确认
- `references/` 放参数明细，SKILL.md 保持短决策树
- 与 `internal/agent/tools/` 一一对应，避免两套语义漂移

### P2 — CLI `gitferry`（cmd/gitferry）

```
cmd/gitferry/
├── main.go
└── ...
internal/cli/
├── client/       # HTTP + Envelope 解包
├── config/       # ~/.config/gitferry/config.yaml + GITFERRY_TOKEN
├── output/       # Envelope + json/table
└── auth/         # token 读取（env 优先）
```

命令面（对齐 REST，高频走 Shortcuts）：

| 域 | Shortcuts | 对应 API |
|----|-----------|----------|
| auth | `+login` `+status` `+logout` | —（本地 token） |
| repo | `+list` `+info` `+branches` `+test` `+create` `+delete` | `/api/v1/repos` `/repo*` |
| task | `+list` `+info` `+create` `+update` `+run` `+delete` `+preview` | `/api/v1/sync/task*` `/sync/preview` |
| history | `+list` `+detail` `+diagnose` `+retry` | `/api/v1/sync/history` `/ops/diagnose` `/ops/retry` |
| platform | `+list` `+test` `+sync-repos` | `/api/v1/platform*` |
| ops | `+overview` `+health` `+inventory` `+rpo` `+drift` `+integrity` `+audit` `+retry-batch` `+drill` `+rebuild` | `/api/v1/ops/*` |
| webhook | `+rules` `+events` | `/api/v1/webhook/*` |
| api | Raw `GET/POST/PUT/DELETE` | 任意路径 |

约定：
- 输出一律 Envelope；默认 `table`，`--format json` 给 Agent
- 错误带 `suggestion`（如「先 gitferry auth login」）
- `GITFERRY_BASE_URL`（默认 `http://127.0.0.1:8890`）+ `GITFERRY_TOKEN`
- 危险 shortcut（`task +run`、`ops +drill` 等）默认返回 `ok=false` + 409，脚本用 `--yes`
- 服务端写接口多为 `POST`（含 delete/run，query 传 `key`）；CLI 已按 IDL 对齐

### P3 — 契约与文档收口
- README 增加「CLI / Agent Skills」章节
- `make build-cli` / `make install-skills`
- Skills 与 CLI 命令表同步维护（单一来源：本文档 §4）

## 5. 验收标准

1. 外部 Agent 仅靠 `skills/` + `gitferry --format json` 能完成：查任务 → 看失败 → 诊断 → 重试
2. 危险操作默认不执行，需 `--yes` 或确认
3. Token 不进日志、不进 Envelope
4. `go test ./...` 通过；CLI 无服务端改动也能独立构建

## 6. 风险与取舍

| 风险 | 缓解 |
|------|------|
| Skills 与 eino 工具语义漂移 | Skills 描述直接引用工具名；改工具时同步改 Skills |
| API 面大，Shortcuts 追不全 | Raw `api` 层兜底；Shortcuts 只做高频 |
| 双端维护成本 | CLI 只做薄封装，不写业务逻辑 |
| 认证模型差异（Web 登录 vs token） | CLI 仅支持 API Key / `GITFERRY_TOKEN`，不复刻登录流 |

---

## 7. 开源工程化（对照 gitlink-cli 的发布与社区能力）

> 第二轮学习重点：gitlink-cli 如何把项目**开源出去**——不只代码可读，而是别人能装、能贡献、能二次分发。

### 7.1 gitlink-cli 的开源工程清单

| 能力 | gitlink-cli 做法 | 作用 |
|------|------------------|------|
| 双语文档 | `README.md`（英）+ `README.zh-CN.md`（中），互链 | 国际可读 |
| 分层 README | Why → Features → Install（人/Agent 双路径）→ Commands → Contributing | 3 分钟上手 |
| 社区健康 | LICENSE 清晰、贡献入口明确 | 降低 PR 门槛 |
| npm 分发 | `@gitlink-ai/cli`：postinstall 下载二进制 + `install-skills` | 一条命令安装 |
| 多平台 Release | 6 平台 tar.gz/zip + changelog 自动生成 | 无 Go 环境也能用 |
| Skills 随包 | npm `files` 含 `skills/`，`gitlink-cli-install-skills` | Agent 开箱即用 |
| 脚本工具链 | `scripts/build-npm.sh` / `pack-local.sh` | 本地打包自测 |
| 设计文档 | `doc/design.md` | 贡献者理解架构 |

### 7.2 本仓缺口与补齐

| 缺口 | 补齐动作 | 状态 |
|------|----------|------|
| 无英文 README | `README.md`（英）+ `README.zh-CN.md`（中） | 本迭代 |
| 无 CONTRIBUTING / SECURITY | 新增 | 本迭代 |
| 无 Issue / PR 模板 | `.github/ISSUE_TEMPLATE/` + `PULL_REQUEST_TEMPLATE.md` | 本迭代 |
| CLI 无 npm 分发 | `npm/` 包：wrapper + install-skills + postinstall 下载 | 本迭代 |
| Release 只发服务端 | 同时打 `gitferry` CLI 6 平台压缩包 | 本迭代 |
| 无本地打包脚本 | `scripts/build-npm.sh` / `pack-local.sh` | 本迭代 |

### 7.3 开源后维护约定

1. **双语同步**：改 README 时两份一起改；CI 不强制，靠 PR 模板勾选
2. **Release 资产命名**：`git-ferry-{os}-{arch}`（服务端）+ `gitferry_{ver}_{os}_{arch}.{tar.gz|zip}`（CLI）
3. **npm 版本 = git tag**：发版走 `git tag v*` → Release workflow → npm publish
4. **安全披露**：漏洞走 SECURITY.md，不公开 issue
5. **许可证**：MIT（与 gitlink-cli 的 MulanPSL-2.0 不同，保持本仓 MIT 不变）

---

## 8. 后续学习清单（已落地 / 待业务迭代）

### 已落地（CLI/Agent/开源工程）

| 能力 | 位置 |
|------|------|
| Schema 自省 | `gitferry schema list/show`（内嵌 OpenAPI） |
| 分页拉全 | `--all` + `client.PaginateAll` |
| 批量 dry-run | `task +batch-run` / `ops +retry-batch --dry-run` |
| config 子命令 | `gitferry config init/get/set/list/path` |
| 多格式输出 | `--format json\|table\|yaml` |
| macOS Keychain | `config.SaveAPIKey`（`security` CLI，文件回退） |
| goreleaser | `.goreleaser.yaml`（可选，与手写 release.yml 二选一） |
| 排障/踩坑文档 | `skills/gitferry-shared/references/{troubleshooting,flags}.md` |
| 工作流扩展 | 周巡检 / 备份验收 / 批量 dry-run |
| 验收清单 | `docs/superpowers/plans/2026-09-30-acceptance-checklist.md` |
| 相邻工具借鉴 | `designs/research/ops-ux-patterns.md` |

### 待业务迭代（见 ops-ux-patterns.md §4）

1. 健康分维度化（Scorecards 模式）
2. 运维待办 Dashboard（Renovate 模式）
3. 策略模板继承（base + override）
4. `token_cmd` / secrets manager
5. 批量操作审计 CSV
