# GitFerry 架构设计文档

> 本文件描述**当前**架构：分层边界、模块职责、数据模型、接口面。
> 历史设计过程与实施记录不在此维护 —— 那些是一次性的过程文档，不是现行架构。

## 1. 项目概述

### 1.1 服务命名

**git-ferry** — 独立的 Git 仓库同步 / 镜像 / 备份中枢。中文名「摆渡」。

### 1.2 三仓架构

| 仓库 | Import path | 角色 | 开源策略 |
|------|-------------|------|----------|
| `git-ferry-core` | `github.com/yi-nology/git-ferry-core` | 同步引擎（无 HTTP） | 公网开源 |
| `git-ferry`（本仓） | `github.com/yi-nology/git-ferry` | 公网壳：hz API + Vue + CLI | 公网开源 |
| `git-ferry-intranet` | `github.com/yi-nology/git-ferry-intranet` | 内网壳：网关身份头 / SSO 钩子 | 私有仓 |

本地开发将三仓放在同一父目录；**服务以 module 版本依赖 core**（`require github.com/yi-nology/git-ferry-core v0.30.0`，无 `replace`）。联调未发布 core 时用本地 `go.work`，勿提交 replace。

```
模式 A: 公网独立服务                 模式 B: 作为库              模式 C: 内网壳
┌────────────────────────┐      ┌──────────────────────┐   ┌────────────────────────┐
│ git-ferry              │      │ 外部项目              │   │ git-ferry-intranet      │
│  main / biz / frontend │      │  ┌────────────────┐  │   │  auth(proxy/SSO hook)  │
│         │              │      │  │  自有 handler  │  │   │  main                  │
│         ▼              │      │  └───────┬────────┘  │   │    ▼                   │
│  git-ferry-core         │◄─────┤          ▼           │   │  httpapi(复用路由)      │
│  model/service/…       │      │  git-ferry-core       │   │    ▼                   │
│         ▼              │      └──────────────────────┘   │  git-ferry-core         │
│  go-git-platform       │                                 └────────────────────────┘
└────────────────────────┘
```

内网差异只放在 `git-ferry-intranet/auth`（`SetAuthMiddlewareProvider`）与配置默认值，不复制业务逻辑。

### 1.3 分层原则

| 层 | 负责 | 不负责 |
|---|---|---|
| `go-git-platform` | 跨仓 HTTP / git 原语：分页、凭证、七平台 REST、gitbackend | 同步编排、业务规则 |
| `git-ferry-core` | 同步编排、DAO/锁、冷备、**一切业务规则与业务数据** | HTTP、鉴权、通知通道、AI |
| `git-ferry` 壳 | HTTP 绑定、DTO 转换、鉴权、AI/CLI、通知通道、只读 gitserve | 业务规则、DB、平台 REST |

**判据一句话：「换一个壳（内网 / 其他前端）还要不要这段逻辑？」要 → core；不要 → 壳。**

### 1.4 设计原则

| 原则 | 说明 |
|------|------|
| **完全独立** | core 不依赖壳，自包含数据模型 |
| **单一依赖** | core 只依赖 go-git-platform 做 Git 平台操作 |
| **可复用库** | 独立仓 `git-ferry-core`，外部 `require github.com/yi-nology/git-ferry-core` |
| **壳不渗入** | hz handler / response / converter / version 留在本仓 |
| **hz 标准** | HTTP 层遵循 hz IDL 代码生成规范 |
| **拒绝静默失败** | 参数不合法时让真实错误暴露，不做静默兜底（详见 §7） |

### 1.5 核心功能

1. **同步任务管理** — CRUD、启停、批量、预览
2. **同步执行** — 手动触发、定时任务、Webhook 触发、失败自动补偿
3. **Webhook 规则** — 分支模式匹配、防抖、平台侧注册
4. **同步历史** — 运行记录、步骤链、失败诊断
5. **镜像中心** — 开源发布 / 仓库备份通道，身份改写 + 版本矩阵 + 发布门禁
6. **历史同步** — 双仓内容差异发布：计划 → 冲突裁决 → 移植 → 推送 + 审计链
7. **运维中心** — 健康评分、资产盘点、策略模板、漂移检测、灾备演练、备份与元数据回灌
8. **AI 助手** — 对话式运维，**执行后端可插拔**：内置 eino（OpenAI 兼容端点）
   或外部 AI coding 工具（ACP 驱动，工具在 agent 侧经本服务 `/mcp` 执行）；
   模型/端点/后端均可界面配置，热生效

### 1.6 Web 控制台

`frontend/` 为 Vue 3 + Ant Design Vue 控制台（GitFerry UI）。

- **设计系统**：`styles/variables.scss` tokens + `styles/theme.ts` AntD 主题；
  中性优先、语义色克制（GitHub/Linear 式运维台）。
- **页面骨架**：`PageHeader` → `MetricStrip` → 筛选栏 → `content-card` 表格。
- **侧栏 IA**：仪表盘 / 管理（同步、仓库、镜像）/ 自动化（Webhook）/ 运维 / 系统（AI、平台）。
- **截图与说明**：`docs/screenshots/`、`frontend/README.md`。

## 2. 目录结构

```
my_project/
├── git-ferry-core/                   # 同步引擎库（独立仓）
│   ├── go.mod                       # github.com/yi-nology/git-ferry-core
│   ├── sync.go                      # 库入口 (package sync)
│   ├── model/                       # GORM 模型 + AutoMigrate 注册
│   ├── service/                     # 业务服务层
│   ├── executor/                    # 同步/备份/DR 执行引擎
│   ├── mirror/                      # 镜像通道 + history/ 双仓内容同步
│   ├── dao/                         # GORM 数据访问层
│   ├── health/  lock/  tpl/  pkg/   # 健康聚合 / 锁 / 模板 / 公共件
│
├── git-ferry/                       # 本仓：公网壳
│   ├── go.mod                       # require git-ferry-core v0.30.0（无 replace）
│   ├── main.go
│   ├── router.go / router_gen.go
│   ├── biz/
│   │   ├── handler/git_sync/        # hz handler + 鉴权 + RBAC
│   │   ├── model/                   # thrift 生成请求/响应
│   │   ├── router/                  # 路由 + 可替换鉴权 Provider
│   │   └── serve/                   # HTTP 启动（内网壳亦复用）
│   ├── internal/
│   │   ├── corebridge/              # 引擎桥梁（壳唯一入口）
│   │   ├── cli/                     # gitferry CLI（cobra）
│   │   ├── agent/                   # AI 助手门面 + 工具注册表（后端可插拔）
│   │   │   ├── engine/             # 执行后端契约 + ACP 外部 agent（acpx / stdio）
│   │   │   └── engine_eino.go      # 内置 eino 后端
│   │   ├── mcp/                     # MCP Streamable HTTP 端点
│   │   ├── gitserve/                # Git Smart HTTP 只读
│   │   ├── notify/  metrics/  runwatch/  converter/  version/
│   ├── frontend/
│   ├── idl/                         # thrift IDL
│   ├── skills/                      # Agent Skills
│   └── conf/config.yaml
│
└── git-ferry-intranet/              # 内网壳（独立仓）
    ├── go.mod                       # require core
    ├── main.go
    ├── auth/                        # proxy 头 / API Key / SSO 钩子
    ├── httpapi/                     # 最小 HTTP 层
    └── conf/config.yaml
```

### 2.1 桥接面

全仓只有 `main.go` 调 `corebridge.NewService`，handler 一律经 `init.go` 的 `GetSyncService()`。**壳内不得手拼平台 SDK 配置** —— provider 必须走 `Service.ProviderForPlatform`（Manager 缓存 + App token + repo token 覆盖）。

## 3. 核心库 API

### 3.1 入口

```go
// git-ferry-core/sync.go
package sync

type Config = model.Config
type Option = service.Option

func NewService(cfg *Config, opts ...Option) (*Service, error)
```

`Option` 用于装配跨领域能力（force-push 审批人、模板存储注入等），避免全局 setter。

### 3.2 服务域

| 域 | 典型方法 | 职责 |
|----|---------|------|
| 任务 | `ListTasks` / `GetTask` / `CreateTask` / `UpdateTask` / `DeleteTask` | 同步任务 CRUD |
| 执行 | `RunTask` / `RunTaskWithTrigger` / `PreviewSync` | 触发与预览 |
| 历史 | `ListHistory` / `GetRunDetail` / `RecentRuns` | 执行记录与步骤链 |
| 仓库 | `ListRepos` / `GetRepo` / `CreateRepo` / `TestConnection` / `ListBranches` | 仓库元数据 |
| 平台 | `ListPlatforms` / `TestPlatform` | 凭据与连通性 |
| Webhook | `ListRules` / `ReceiveWebhook` / `ListEvents` / `RetryEvent` | 规则与事件 |
| 备份 | `MetadataBackup` / `MetadataRestore` / `PushBackup` / `BuildBackupManifest` | 冷备与元数据 |
| 灾备 | `RunDRDrill` / `VerifyDrillChain` / `RPOReport` | 演练与 RPO |
| 镜像 | 镜像通道 CRUD / `Preview` / `Verify` / 版本矩阵 | 开源发布通道 |
| 历史同步 | 通道 CRUD / `Plan` / `Resolve` / `Preview` / `Apply` / `Push` / `Rollback` | 双仓内容回合 |
| 治理 | 审计哈希链、RBAC、force-push 审批、模板 | 合规与策略 |

core 导出 `(*Service)` 方法约 135 个，**零 HTTP 渗入**（`net/http` 仅出站用途）。

### 3.3 包布局（core）

| 包 | 职责 |
|---|---|
| `service` | 业务服务层入口与各领域服务 |
| `executor` | 同步/备份/DR 演练执行引擎、pipeline、bundle 与远端备份 |
| `mirror` | 镜像通道门禁、身份改写、语义校验、跨实例互斥；`mirror/history` 为双仓内容同步 |
| `dao` | GORM 数据访问层 |
| `model` | GORM 模型与 `AutoMigrate` 注册 |
| `health` | 健康检查聚合 |
| `lock` | 进程内互斥与 redis 跨实例锁（抖动时降级为进程内互斥） |
| `tpl` | 模板存储（文件实现，可由 `service.TemplateStore` 注入替换） |
| `pkg` | 公共件：`proclog`（进程级日志/退出）、`strutil`、`deploykey` |

## 4. IDL 定义 (Thrift + hz 规范)

IDL 位于 `idl/`，按域拆分：`git_sync.thrift`（聚合）、`sync_task.thrift`、`repo.thrift`、`platform.thrift`、`webhook.thrift`、`ops.thrift`、`mirror.thrift`、`ai.thrift`、`operation_log.thrift`、`system.thrift`、`base.thrift`。

```thrift
// idl/sync_task.thrift
namespace go sync_task

service SyncTaskService {
    sync_task.ListTasksResp ListTasks(1: sync_task.ListTasksReq req) (api.get="/api/v1/sync/tasks")
    sync_task.GetTaskResp GetTask(1: sync_task.GetTaskReq req) (api.get="/api/v1/sync/task")
    sync_task.CreateTaskResp CreateTask(1: sync_task.CreateTaskReq req) (api.post="/api/v1/sync/task/create")
    sync_task.UpdateTaskResp UpdateTask(1: sync_task.UpdateTaskReq req) (api.post="/api/v1/sync/task/update")
    sync_task.DeleteTaskResp DeleteTask(1: sync_task.DeleteTaskReq req) (api.post="/api/v1/sync/task/delete")
    sync_task.RunTaskResp RunTask(1: sync_task.RunTaskReq req) (api.post="/api/v1/sync/task/run")
    sync_task.PreviewSyncResp PreviewSync(1: sync_task.PreviewSyncReq req) (api.post="/api/v1/sync/preview")
}
```

### 4.1 代码生成

```bash
make generate     # hz update，生成 biz/model + router
make apidoc       # 重新生成 docs/openapi.json + internal/pkg/swagger/openapi.json
```

**契约一致性是硬门禁**：IDL → hz 生成 → OpenAPI 三者必须同步。改动 API 后跑 `make apidoc`，CI 会校验 OpenAPI 与路由表一致。

### 4.2 契约纪律

- **`optional` 必须被解析器正确处理**：thrift 的 `optional` 关键字若被当类型解析，所有可选字段会退化成 `"type":"string"` ⇒ 按契约生成的客户端会把布尔字段发成字符串。apidoc 生成器先剥 `optional` 再取类型，并有单测钉住。
- **破坏性变更直接改**：字段改名 / 出参形态变更不必兼容旧形状。旧调用方一起改。
- **OpenAPI 与路由表必须一致**：二者由不同来源生成，任一漂移都会让客户端拿到说谎的契约。

## 5. 数据模型

12 张表由 `model.InitDB` 的 `AutoMigrate` 自动创建（sqlite / mysql 通用）。

| 表 | 模型 | 说明 |
|---|---|---|
| `platforms` | `Platform` | 平台凭据（token 加密落库） |
| `repos` | `Repo` | 仓库元数据 |
| `sync_tasks` | `SyncTask` | 同步任务 |
| `sync_runs` | `SyncRun` | 执行记录 |
| `sync_run_steps` | `SyncRunStep` | 执行步骤链 |
| `webhook_rules` | `WebhookRule` | 触发规则 |
| `webhook_rule_tasks` | `WebhookRuleTask` | 规则 ↔ 任务关联 |
| `webhook_events` | `WebhookEvent` | 接收事件流 |
| `operation_logs` | `OperationLog` | 操作审计 |
| `mirror_channels` / `mirror_targets` / `mirror_runs` | `MirrorChannel` 等 | 镜像发布通道 |
| `history_channels` / `history_targets` / `history_runs` / `history_overrides` | `HistoryChannel` 等 | 双仓内容同步 |
| `templates` | `tpl.Template` | 策略模板 |
| `force_push_approvals` | `ForcePushApproval` | 强推审批 |

### 5.1 关键模型

```go
type SyncTask struct {
    ID            uint       `json:"id" gorm:"primaryKey"`
    Key           string     `json:"key" gorm:"uniqueIndex"`
    Name          string     `json:"name"`
    SourceRepoKey string     `json:"sourceRepoKey"`
    SourceBranch  string     `json:"sourceBranch"`
    TargetRepoKey string     `json:"targetRepoKey"`
    TargetBranch  string     `json:"targetBranch"`
    SyncMode      string     `json:"syncMode"`
    Cron          string     `json:"cron"`
    Enabled       bool       `json:"enabled"`
    GitTags       bool       `json:"gitTags"`
    GitForce      bool       `json:"gitForce"`
    LastRunAt     *time.Time `json:"lastRunAt"`
    LastStatus    string     `json:"lastStatus"`
    CreatedAt     time.Time  `json:"createdAt"`
    UpdatedAt     time.Time  `json:"updatedAt"`
}

type Repo struct {
    ID            uint   `json:"id" gorm:"primaryKey"`
    Key           string `json:"key" gorm:"uniqueIndex"`
    Name          string `json:"name"`
    Platform      string `json:"platform"`
    PlatformOwner string `json:"platformOwner"`
    PlatformRepo  string `json:"platformRepo"`
    CloneURL      string `json:"cloneUrl"`
    SSHURL        string `json:"sshUrl"`
    DefaultBranch string `json:"defaultBranch"`
    AccessToken   string `json:"-"`     // 加密存储，永不出库
    WebhookSecret string `json:"-"`     // 加密存储，永不出库
    Status        string `json:"status"`
}
```

### 5.2 表名与字段约定

- 表名常量集中在 `model/constants.go`，模型用 `TableName()` 引用，**不在代码里散写字符串表名**。
- 敏感字段（`access_token` / `webhook_secret`）带 `json:"-"`，序列化即丢弃。

### 5.3 软删与唯一索引

带 `gorm.DeletedAt` 的表配唯一索引时，**软删行仍占着业务键**，而默认作用域查不到软删行 ⇒ 「查不到 ⇒ 新建」会撞唯一索引。重建路径必须先 `Unscoped` 查软删行并复活（清 `deleted_at` + 重置状态），不能直接 INSERT。

另有两个静默失败的坑：`Updates(map)` 跳过 nil 值（清 `deleted_at` 要用 `gorm.Expr("NULL")`）；复用同一 `tx` 做多条 UPDATE 会让 WHERE 累加（每条语句从 `db()` 重新起一条）。

## 6. 与 go-git-platform 集成

| SDK 包 | 用途 | 我们实现什么 |
|--------|------|-------------|
| `provider` | 平台 REST、凭证、App token | 规则匹配、任务触发、平台管理 |
| `gitbackend` | git fetch/push/clone | 同步执行引擎、镜像改写与移植 |
| `credential` | 凭证构建与加密 | 凭据管理 |

### 6.1 支持平台

`github` / `gitlab` / `gitee` / `gitea` / `forgejo` / `gitcode` / `tencent_code`，以及自建实例（`api_url` 自配）。

### 6.2 分支过滤

```go
import branchfilter "github.com/yi-nology/go-git-platform/branchfilter"

func matchBranch(pattern, branch string) bool {
    if pattern == "" {
        return true
    }
    bf := branchfilter.New(pattern)
    return bf.Match(branch)
}
```

同步任务另支持 `include_branches` 白名单与 `exclude_ref_patterns`（默认忽略 `refs/pull/*` 等 PR refs）。

### 6.3 SDK 层的既有约束

- **跨 host 重定向不得带走平台令牌**（SDK v0.99.4+）：平台 302 到 CDN 时令牌会发给第三方主机。
- **日志与错误构造必须脱敏**：URL query 里的凭据会随 `*url.Error` 的 `Error()` 进入日志；`Request.Path` 可自带 query。秘密类 query 参数替换成 `REDACTED`，路径整体掩掉，保留 host 以便排障。

## 7. 接口面

### 7.1 HTTP 路由分组

| 分组 | 前缀 | 说明 |
|---|---|---|
| 同步任务 | `/api/v1/sync/*` | 任务 CRUD、预览、执行历史 |
| 仓库 | `/api/v1/repo(s)` | 仓库 CRUD、分支、连通性、批量 |
| 平台 | `/api/v1/platform(s)` | 平台 CRUD、连通性、仓库清单同步 |
| Webhook | `/api/v1/webhook/*` | 规则、事件、平台侧注册 |
| 运维 | `/api/v1/ops/*` | 健康、盘点、待办、趋势、模板、备份、灾备、审计、审批、漂移、Org 映射 |
| 镜像 | `/api/v1/mirror/*` | 镜像通道、目标、版本矩阵、校验 |
| 历史同步 | `/api/v1/histsync/*` | 通道、目标、计划、裁决、预检、移植、推送、回滚、审计 |
| AI | `/api/v1/ai/*` | 状态、对话、配置、模型列表 |
| 日志 | `/api/v1/logs/operations` | 操作日志 |
| 系统 | `/api/v1/system/status` | 系统状态 |
| MCP | `/mcp` | Streamable HTTP |
| Git Smart HTTP | `/git/*` | 只读冷备取件（`git_serve.enabled` 时开启） |
| Webhook 入站 | `/api/webhook/receive/*repoKey` | 平台回调（`*` 捕获含斜杠的 `owner/repo`） |

完整路由表以 `docs/openapi.json` 为准（`make apidoc` 生成）。

### 7.2 鉴权与 RBAC

| 机制 | 说明 |
|---|---|
| `X-API-Key` | 主鉴权头，角色由 `server.api_key_role` 决定 |
| 角色 | `admin` / `operator` / `readonly` 三档 |
| 写操作守卫 | `WriteGuard()`（需写角色）/ `AdminGuard()`（需 admin） |
| OIDC JWT | `oidc_auth.go`，外部 IdP 换取会话 |
| 审计 | 操作日志 + 哈希链（`/ops/audit-chain/verify`） |

### 7.3 失败语义纪律

**hertz 会静默忽略未知 query 参数** ⇒ 契约漂移的表现是「命令存在、参数也传了，但永远不生效」。因此：

- handler 绑定失败即 400，**不得把绑定错误当成「没传过滤条件」**（那会让筛选静默退化成全量扫描）。
- CLI 的请求形状必须与服务端契约逐字段对齐。
- `PaginateAll` 必须真的拉全量并给出真实总数。

### 7.4 CLI

`internal/cli` 基于 cobra，命令组：`repo` / `task` / `history` / `histsync` / `ops` / `platform` / `webhook`，外加 `auth` / `config` / `schema` / `api` / `version`。

- **输出统一 Envelope**：`{ok, data, error, meta}`；`--format json|table|yaml`。
- **危险操作确认门**：`task +run`、`ops +drill`、`ops +metadata-restore --execute` 等缺 `--yes` 时返回 409。
- **拒绝无用参数**：`histsync` 子命令若收到本命令不消费的 flag，直接报错退出（静默丢弃比报错危险得多 —— 让人以为操作生效了）。
- **凭据不进命令历史**：写凭据用 `--credential-file`。
- **退出码分级**：批量部分失败时退出码非 0。

### 7.5 MCP 与 AI 工具注册表

MCP 端点与 AI 助手**共用同一注册表**（`internal/agent/tools`），MCP 只是协议适配壳。

- 42 个工具，其中 13 个标注为**危险**（需用户确认，且只读角色一律拒绝）。
- 危险标注只写在注册处（`addDanger`），别处不另抄清单 —— 派生视图（系统提示词、权限守卫）从注册表取。
- 危险工具必须有**角色闸门**：不能只靠「MCP 路径不注入 scope」这种偶然挡住写操作。
- 写记忆类工具按危险处理：记忆会进每次会话的 system prompt，是持久化提示注入的通道。

### 7.5.1 AI 执行后端可插拔

「谁来跑这一轮对话」由**后端**决定（`internal/agent/engine` 的 `Engine` 契约）：

| 后端 | 形态 | 工具在哪执行 |
|------|------|-------------|
| `internal` | 内置 eino 助手，进程内调 OpenAI 兼容端点 | 本进程（直调 core service） |
| `acpx` | 外部 coding agent，经 acpx CLI 驱动 | agent 侧（acpx 负责 client 角色） |
| `stdio` | 外部 coding agent，原生 ACP，自己当 client | agent 侧（可挂本服务 `/mcp`） |

- 门面（`internal/agent.Runner`）只做**与后端无关**的三件事：并发闸门、会话记账、事件转发。
- 事件契约（`engine.Event`）与后端无关，SSE 出参零变化；外部后端多出 `thinking` / `plan` / `permission` 三种事件。
- 外部后端经 **ACP**（Agent Client Protocol，JSON-RPC over stdio）驱动；协议形状以实测为准，
  夹具见 `internal/agent/engine/acp/testdata/`。
- 外部 agent 是**跑在本机上的子进程**，工作目录即代码层能给到的沙箱边界。
- 挂本服务 `/mcp` 给外部 agent 时，MCP 专用 key 固定**只读角色**（MCP 路径上危险工具不弹确认）。
- 委派工具 `ai_code_delegate`（危险）让内置助手把一段编码任务派给外部 agent。
- **AI 配置是治理面**：改配置要写角色；**换后端 / 改外部 agent 授权档仅 admin**，并写操作日志。
- 详见 `docs/design/acp-external-agent-design.md`。

### 7.6 Agent Skills

`skills/` 提供可安装的知识库：`gitferry-shared`（认证/Envelope/安全规则）、`gitferry-repo` / `gitferry-task` / `gitferry-history`、`gitferry-ops` / `gitferry-workflow`、`gitferry-histsync`。

约定：Skills 与服务端 AI 工具语义对齐，改一侧同步另一侧。

## 8. 配置文件

```yaml
# conf/config.yaml（完整示例见 conf/config.example.yaml，Schema 见 conf/config.schema.json）

server:
  host: 0.0.0.0
  port: 8890
  api_key: ""              # X-API-Key；生产必填强随机值
  api_key_role: admin      # admin | operator | readonly

database:
  driver: sqlite           # sqlite | mysql
  dsn: data/git_sync.db

redis:
  addr: 127.0.0.1:6379

git:
  backend: gogit           # gogit | native

sync:
  max_concurrent: 2
  backup_dir: var/backup
  backup_keep: 5
  backup_format: bundle    # bundle | zip
  backup_encrypt_key: ""   # AES-256-GCM；空 = 不加密
  backup_retention_days: 90
  legal_hold: false
  post_exec_script: ""     # 注入 GITFERRY_TASK/RESULT/RUN_ID/TRIGGER
  backup_destinations: []  # s3/webdav/azure/local 多目的地扇出

runwatch:
  interval_seconds: 30
  history_limit: 10
  retry:
    max_auto_retries: 2
    cooldown_minutes: 5

log:
  level: info
  format: json
```

### 8.1 AI 运行时设置

界面保存的配置写入 **`data/ai-settings.json`**（权限 0600，含 API Key，不进 yaml）：

| 字段 | 说明 |
|------|------|
| `enabled` | 是否启用助手 |
| `base_url` | OpenAI 兼容端点 |
| `model` | 模型 ID |
| `temperature` / `max_tokens` / `timeout_seconds` / `max_concurrent_chats` | 生成参数 |
| `api_key` | 密钥明文仅存此文件；GET 接口只回 `has_api_key` / 脱敏值 |

合并策略：yaml `ai` 段为默认值 → `ai-settings.json` 覆盖 → 环境变量 `GIT_SYNC_AI_API_KEY` 作密钥兜底。保存接口热重建 `agent.Runner`，无需重启进程。

**密钥不得进 URL query** —— `GET /api/v1/ai/models?api_key=…` 会让密钥进浏览器历史 / Referer / 反向代理 access log。故该端点为 **POST + body**。

## 9. 部署

### 9.1 Dockerfile

```dockerfile
FROM golang:1.27-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o git-ferry .

FROM alpine:3.18
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/git-ferry .
COPY --from=builder /app/conf ./conf
EXPOSE 8890
ENTRYPOINT ["./git-ferry", "-env", "prod"]
```

### 9.2 Makefile 目标

| 目标 | 作用 |
|---|---|
| `build` / `build-cli` | 构建服务端 / CLI |
| `run` / `restart` | 本地运行 |
| `test` / `lint` / `fmt` / `vet` / `tidy` | 质量门禁 |
| `generate` / `apidoc` | hz 代码生成 / OpenAPI 生成 |
| `install-skills` / `pack-npm` / `build-npm` | Skills 与 npm 分发 |
| `docker-build` | 服务端镜像 |
| `goreleaser-snapshot` | 本地多平台产物快照 |

### 9.3 分发

- **Release 二进制**：`gitferry_<ver>_<os>_<arch>.tar.gz`（服务端 `git-ferry_<ver>_<os>_<arch>.tar.gz`）。
- **npm 包**：`gitferry-cli`（随 CLI 发布，需 `NPM_TOKEN`；未配置时发布步骤静默跳过）。
- **包管理器模板**：`examples/packaging/`（homebrew / scoop / nix）。
- **无服务路径**：`examples/ci/github-actions-mirror.yml` —— 只需 CI 定时镜像时不必部署整套服务。

## 10. 已知约束

1. **core 不可依赖 hertz**：payload 构造器与请求抽取留在壳侧（框架相关），载荷结构本身是 core 的类型。
2. **两壳保留少量刻意重复**：`readAPIKey`（配置归属不同：壳走全量 overlay，内网壳是 auth 侧迷你解析）与 webhook 接收的 hertz 抽取（core 不能依赖 hertz）。
3. **redis 抖动降级**：`lock` 在 redis 不可用时降级为进程内互斥而非失败。
4. **mirror 锁语义**：进程内阻塞互斥（保持串行语义）→ 再抢 redis 跨实例锁（被占即报忙碌，不排队）；TTL 覆盖整次克隆。不用等待式分布式锁，避免锁过期导致两实例同时克隆。
5. **`go.work` 掩盖版本**：本地绿 ≠ CI 绿。收尾必须跑 `GOWORK=off go build ./...` 校验发布态。
6. **gitbackend 子命令白名单**：`NativeGitBackend.RunRaw` 不含 `bundle` / `fsck` / `update-ref` / `lfs`，相关流程不要假设白名单外子命令可用。

---

**文档版本**: 3.0
**维护者**: git-ferry team
