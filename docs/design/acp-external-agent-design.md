# 外部 AI Coding 工具接入（ACP / acpx）设计

> 2026-10-26 · 状态:待评审
> 形态:git-ferry 壳层 `internal/agent` 引入「后端可插拔」;ACP 作为内置 eino 助手之外的第二个执行后端
> 关联:`ARCHITECTURE.md` §7.5(MCP 与 AI 工具注册表)· `internal/mcp/server.go` · `internal/agent/*`
> 目标读者:壳层维护者、运维治理方

---

## 0. 结论摘要

**一句话**:把「谁来执行这一轮对话」从 `internal/agent` 里抽出来做成**可插拔引擎**,新增一个 **ACP 引擎**驱动外部成熟 coding 工具(Claude Code / Codex / MiniMax Code / Gemini CLI …);外部 agent 通过 **git-ferry 自己的 MCP 端点**拿回 41 个领域工具,不必重造。

三条决策:

| # | 决策 | 理由 |
|---|------|------|
| D1 | 接缝在**回合层(turn)**,不在模型层 | ACP 是「编辑器 ↔ coding agent」的 JSON-RPC 协议,agent 是**子进程**,不是 OpenAI 兼容端点。接在 model 层无处可接 |
| D2 | 领域工具**复用现有 `/mcp`**,不重写一套 | 41 个工具已在一个注册表里,`internal/mcp/server.go` 已是对外工具面;外部 agent 挂 MCP client 即可,一份事实源 |
| D3 | 落地顺序:**acpx 子进程通道 → 原生 ACP stdio** | acpx 免实现 client 侧 fs/terminal 能力、免管长驻进程,一天内能验证;原生 ACP 才能拿到 `session/request_permission` 打进确认 UI |

**推荐落地路径**:先做「阶段 0/1(acpx 通道 + 委派工具)」,拿到外部 agent 干活的实证;再做「阶段 2(后端整体切换)」;最后「阶段 3(原生 ACP + 权限确认 + 审计)」。

---

## 1. 现状基线(事实,带出处)

### 1.1 内部 AI 助手链路

```
main.go:64-138            AI 装配(yaml 默认 + data/ai-settings.json 覆盖 → buildRunner → SetAgentRunner)
  └ internal/agent/runner.go:40-55    NewRunner:OpenAI 兼容 ChatModel(eino-ext openai)
      └ internal/agent/runner.go:58-89 NewRunnerWithModel:adk.NewChatModelAgent + decoratedTools
          └ internal/agent/decorator.go:33-80  DecorateTool:包一层产出 tool_start/tool_end/tool_confirm
              └ internal/agent/tools/repo.go   41 个领域工具全部注册于此;危险标注只写在 addDanger 处
internal/agent/runner.go:125-229       Run:并发闸门(syncx.Gate)+ 会话快照 → eino 迭代器 → 事件流
internal/agent/event.go:4-19           Event{start|delta|tool_start|tool_end|tool_confirm|done|error}
internal/agent/confirm.go              危险工具确认令牌(5 分钟过期)
internal/agent/tools/permission.go     Policy + oagentkit/guardx permgate 规则引擎(会话级档位)
```

- 角色靠 `ctx` 传递(`authz.WithRole`),工具层自判角色,绕过路由守卫。
- **41 个工具全部注册在 `internal/agent/tools/repo.go`**,其中 **12 处 `addDanger` 是危险标注的全仓唯一事实源**;
  `internal/agent/tools/danger.go` 只是它的派生视图(不另抄清单),系统提示词清单与 `tools/list` 过滤都从注册表派生。
- 权限档位是**会话级**(`Session.permMode`),切档需 admin 角色 + 确认。

### 1.2 对外工具面(已存在,本次要复用)

```
internal/mcp/server.go:41-102   POST /mcp  JSON-RPC(initialize / tools/list / tools/call / ping)
internal/mcp/server.go:119-163  listTools 按角色裁剪(只读角色看不见危险工具)
internal/mcp/server.go:85-93    危险工具在 MCP 路径上做角色闸门(只读角色直接拒)
internal/mcp/server.go:47-53    MCP 路径不登记待确认令牌(确认由客户端负责)
```

**关键事实**:MCP 端点一旦通过角色闸门,危险工具**直接执行、不弹确认**。这决定了外部 agent 挂什么 key 给它(见 §8)。

### 1.3 配置与契约面

```
internal/agent/config.go:18-32     Config(ai yaml 段)· :102-118 Validate(enabled 时要求 base_url+model+apiKey)
internal/agent/settings.go:16-29   Settings(前端可改,存 data/ai-settings.json,0600)
internal/agent/settings.go:174-203 MergeSettings(yaml 默认 + 设置文件覆盖)
biz/handler/git_sync/ai_config.go  GET/POST /api/v1/ai/config · POST /api/v1/ai/config/test
biz/handler/git_sync/ai_service.go:55-134  POST /api/v1/ai/chat(SSE)
idl/ai.thrift:3-12                AIConfigRequest
```

---

## 2. 目标与非目标

### 目标

1. **不使用内部 AI**:AI 助手的执行后端可整体换成外部成熟 coding 工具,内置 eino 只是**其中一个可选后端**。
2. **新增「调用外部 AI code」能力**:内置助手可以把一段编码任务**委派**给外部 agent(阶段 1),两条路都在。
3. 外部 agent **能操作 git-ferry 领域**(同步任务、镜像、运维…),且**不绕过**既有的角色闸门与审计。
4. 前端**尽量零改动**复用现有 SSE 事件与确认卡。

### 非目标

- 不做 git-ferry 自己的 coding agent(不写 patch 生成、不做 IDE 集成)。
- 不替代 `/mcp` 对外工具面(那是给 Claude Code / Cursor 用的,本次是**反向**:让外部 agent 用上 git-ferry 的工具)。
- 不在第一阶段就做原生 ACP 的 fs/terminal 客户端(那是 acpx 之后的事)。
- 不动 `git-ferry-core`(本设计全在壳层 `internal/agent` + `biz` + `idl` + `frontend`)。

---

## 3. 核心判断:接缝在回合层,不在模型层

**为什么不能接在模型层**:内置助手的 `model.BaseModel` 是「OpenAI 兼容 chat/completions」形态。ACP 侧根本没有 chat/completions —— 它是:

```
client ──initialize──▶ agent
client ──session/new──▶ agent      {cwd, mcpServers:[...]}
client ──session/prompt──▶ agent   {sessionId, prompt:[ContentBlock]}
agent ──session/update──▶ client   (notification, 流式:文本块 / 思考块 / 工具调用 / 计划)
agent ──session/request_permission──▶ client   (agent 反向要授权)
client ──session/cancel──▶ agent
```

会话由 **agent 侧进程**持有,工具由 **agent 侧**自己执行(通过 MCP client 调 git-ferry)。把它硬塞进 `model.BaseModel` 会得到一堆假动作。

**所以接缝定义为**:一个引擎能把「一句话 + 一个会话上下文」变成「一串与后端无关的事件」。

```
                    ┌──────────────── Runner(门面,对外 API 不变)────────────────┐
                    │  Run(ctx, sess, msg) → (<-chan Event, cancel, error)       │
                    │  ExecuteConfirmed(ctx, sess, tool, token)                  │
                    │  Sessions() / ModelName()                                 │
                    └────────┬───────────────────────────────┬──────────────────┘
                             │                               │
              ┌──────────────▼───────────────┐   ┌───────────▼──────────────────┐
              │  engine.Internal(eino)       │   │  engine.ACP(外部 agent 子进程) │
              │  41 工具进程内直调 core      │   │  ACP stdio / acpx CLI        │
              │  工具执行在 git-ferry 进程内 │   │  工具执行在外部 agent 进程内   │
              └──────────────────────────────┘   └───────────┬──────────────────┘
                                                                 │ MCP client
                                                                 ▼
                                                   git-ferry 自己的 /mcp(41 工具)
```

**关键收益**:外部 agent 不需要知道 git-ferry 的领域模型 —— 它只要会说 MCP。而 git-ferry 这边**一个工具都不用重写**。

---

## 4. 引擎抽象(Go 接口设计)

### 4.1 包布局(落地版)

```
internal/agent/engine/            中立层:Engine 接口 + Event/Usage + Session 最小接口
internal/agent/engine/delegate/   叶子包:委派契约(tools 与 engine/acp 都只依赖它)
internal/agent/engine/acp/        外部后端:acpx transport + 原生 stdio transport
internal/agent/engine_eino.go     内置后端(eino):**留在 agent 包**,不搬
internal/agent/                   门面(Runner)+ 会话 + 配置
```

依赖方向必须单向,否则成环:

```
internal/agent ──▶ internal/agent/engine ──▶ internal/agent/tools
internal/agent/engine/acp ──▶ internal/agent/engine, engine/delegate
internal/agent/engine/acp ──▶ internal/agent/tools(只为 SessionScope 契约)
```

> ⚠️ 环检测:`agent` **不得** import `engine/acp`;两者由 `main.go` 显式装配。
> 这与本仓「壳不渗入 / 显式装配」的既有纪律一致。

> **与初版设计的偏差(有意为之)**:初版计划把 eino 实现整体搬到
> `engine/internal`。落地时**没有搬** —— eino 实现与 `agent` 包的
> prompt / 装饰器 / 落盘 / 会话深度耦合,搬动会产生大量无谓改动与回归面。
> 抽象只落在**契约层**(engine 包),eino 实现原地保留,对外接口完全一致。

### 4.2 接口

```go
// internal/agent/engine/engine.go
package engine

// Event 中立事件(现有 agent.Event 的家)。字段与现 Event 完全一致,
// 前端 SSE 契约零变化。
type Event struct {
    Type      string // start|delta|thinking|tool_start|tool_end|tool_confirm|permission|done|error
    Content   string
    Tool      string
    Args      string
    Result    string
    Token     string
    SessionID string
    Usage     *Usage
}

type Usage struct{ InputTokens, OutputTokens int }

// Session 引擎最小可见的会话面(engine 不需要知道 agent.Session 的实现细节)。
type Session interface {
    ID() string
    // History 返回可用于拼装 prompt 的历史(user/assistant 文本)。
    History() []Turn
}

type Turn struct{ Role, Content string }

// Engine 一个能跑一轮对话的后端。
type Engine interface {
    // ID 后端标识:internal | acp。
    ID() string
    // Label 展示名(如 "MiniMax Code (ACP)"、"内置助手 (eino)")。
    Label() string
    // ModelName 展示用模型/agent 名。
    ModelName() string
    // Run 执行一轮对话,产出事件流。
    // 契约:ctx 取消 ⇒ 事件流必须终止;返回的 cancel 幂等且可重复调用;
    //       引擎内部错误以 error 事件呈现,不以 Go error 呈现(除启动期致命错误)。
    Run(ctx context.Context, sess Session, in Input) (<-chan Event, context.CancelFunc, error)
    // Close 释放后端资源(关子进程/清会话)。可返回 nil。
    Close() error
}

type Input struct {
    Message string
    // Workspace 外部 agent 的工作目录(沙箱边界)。
    Workspace string
}
```

### 4.3 现有代码的迁移映射

| 现状 | 去向 |
|------|------|
| `internal/agent/event.go` 的 `Event`/`Usage` | 移到 `engine` 包;`agent` 包留 `type Event = engine.Event` **别名**(现有引用一行不改) |
| `internal/agent/runner.go` 的 eino 装配 + 迭代器 | 搬到 `internal/agent/engine/internal/engine.go` |
| `internal/agent/decorator.go` 的 `DecorateTool` | 留在 `agent` 包(engine/internal 引用它),或一并下移到 `engine` 包 |
| `internal/agent/runner.go` 的 `Runner` | 保留在 `agent` 包,改为持有 `engine.Engine`;对外方法签名不变 |
| `internal/agent/runner_test.go` 等 | 随 eino 引擎一起搬到 `engine/internal` 包 |

**兼容性**:对外 HTTP 契约(`/api/v1/ai/chat` 的 SSE 事件结构)、`Runner` 的公开方法、`SetAgentRunner` 注入点**全部不变** —— 这是本设计最重要的约束。

---

## 5. 外部 agent 的两种接法

### 5.1 接法 A:acpx 子进程(推荐先做)

`acpx`(npm,pre-1.0 alpha)是 **ACP 的无头 CLI 客户端**:一个命令面对 20+ 个 coding agent,免写 JSON-RPC、免实现 client 侧 fs/terminal 能力、免管长驻进程。

```
git-ferry ──exec──▶ acpx --format json <agent> "<prompt>" ──stdio──▶ ACP agent
             ◀──NDJSON 事件流──
```

可用的内置 agent 名(acpx 注册表,节选):`codex` / `claude` / `gemini` / `opencode` / `qwen` / `cursor` / `copilot` / `kimi` / `iflow` / `openclaw` / `pi` / **`mcode`(MiniMax Code)** / `droid` / `trae` / `zeroclaw` …

**本机现成可用**(已实测 `--help`):

| 命令 | 形态 | 备注 |
|------|------|------|
| `mcode acp` | ACP server over stdio | MiniMax Code 自家,`--help` 明确写了 "Run MiniMax Code as an Agent Client Protocol server over stdio" |
| `openclaw acp` | ACP bridge(连 Gateway) | 需 `--url/--token` 或本地 Gateway |
| `npx pi-acp` | ACP adapter | 本机 `pi` 有插件 patch 报错,需走 npx 包 |

⇒ **第一个验证目标不需要装 Claude/Codex**,`mcode acp` 就能跑通。

关键能力(来自 acpx 文档,落地时以实测为准):

- `--format json` → NDJSON 事件流;`--format quiet` → 只要最终文本;`--format text` → 人读流
- 权限档:`--approve-all` / `--approve-reads` / `--deny-all` / `--non-interactive-permissions fail`
- 会话:`sessions new|ensure|list|show|history|close`,作用域 = (agent, cwd, 可选名),状态存 `~/.acpx/sessions/`
- 逃生口:`--agent '<任意 ACP 服务器命令>'`
- 其它:`--cwd` / `--timeout` / `--ttl` / `--file` / `--no-wait` / `--json-strict` / `--verbose`

> ⚠️ **NDJSON 字段名必须实测钉住,不能照抄 README**。acpx 文档给出的是事件**信封**形状
> `{eventVersion, sessionId, requestId, seq, stream, type}`,`type` 取值只明确提到 `tool_call`;
> 文本/思考/结束事件的类型名与 payload 形状需**先写一个探针脚本**(`acpx --format json <agent> "say hi"`)
> 把真实输出贴进测试里再写解析器。**照抄文档写解析器 = 拿假设当协议**(本仓既有教训:验证手段本身的盲区会让失败信号被读成「目标不存在」)。

**acpx 接法的能力边界(要如实告知用户)**:

| 能力 | acpx 通道 | 原生 ACP |
|------|-----------|-----------|
| 免实现 client fs/terminal | ✅ | ❌ 要自己实现 |
| 免管长驻子进程 | ✅(每次 prompt 一次 CLI 调用) | ❌ 要自己做会话进程池 |
| 20+ agent 免适配 | ✅ | ❌ 每个 agent 一个 adapter |
| **拿到 `session/request_permission`** | ❌(被 acpx 的权限档吃掉) | ✅ |
| **把 git-ferry 的 `/mcp` 挂给 agent** | ⚠️ 依赖各 adapter 自己的 MCP 配置(claude/codex/gemini 各不同) | ✅ 原生 `session/new.mcpServers` 一处配好 |
| 结构化事件保真度 | 投影(thinking/tool/done) | 完整 |

### 5.2 接法 B:原生 ACP stdio(阶段 3)

直接对 ACP adapter 说 JSON-RPC:

```
initialize → session/new{cwd, mcpServers} → session/prompt → (session/update 流)
                                        ↘ session/request_permission(要授权)
```

- **client 侧必须实现的能力**(ACP 规定 agent 会反向调我们):
  `fs/read_text_file`、`fs/write_text_file`、`terminal/create|output|release|wait_for_exit|kill`。
  这部分工作量实打实,但**只在阶段 3 做**。
- **好处**:唯一能拿到 permission 请求 ⇒ 唯一能把外部 agent 的写操作接进 git-ferry 确认 UI 的路径;
  也是唯一能干净挂 `mcpServers` 的路径。
- Go SDK 备选:`github.com/coder/acp-go-sdk`(官方感更强,含 claude/gemini 示例)或
  `github.com/ironpark/acp-go`(非官方,`acp1`=稳定 schema/v1,`acp2`=draft,建议只依赖 `acp1`)。
  > 本仓偏好**用现成工具而非自研**(「仓里不留自研的代码生成逻辑」同源偏好)⇒
  > 阶段 3 先评估 SDK 直接可用性,只在 SDK 缺口处自研,且缺口给上游提 issue。

### 5.3 一个必须先做的取舍:**挂 MCP 还是不走 MCP**

外部 agent 要用 git-ferry 领域能力,有两条路:

| 方案 | 做法 | 评价 |
|------|------|------|
| **M1(推荐)** | `session/new` 挂 git-ferry 自己的 `/mcp`(http + 专用 key) | 一份工具事实源;agent 侧标准做法;阶段 3 落地 |
| M2 | 给外部 agent 一份 git-ferry **CLI/SDK 说明书** | 要自己写适配,工具语义会漂 ⇒ 否决 |

M1 的实现细节(阶段 3):

```json
{
  "mcpServers": [
    {
      "type": "http",
      "name": "gitferry",
      "url": "http://127.0.0.1:8890/mcp",
      "headers": [
        { "name": "X-API-Key", "value": "<专用只读 key>" }
      ]
    }
  ]
}
```

> ⚠️ **headers 是数组 `[{name,value}]`,不是对象** —— 这是 ACP 规范原文的形状
> (官方 session-setup 文档)。写成对象 agent 侧解析不出来,而这种错**不会报错**,
> 只会表现为「MCP 工具列表为空」。

**必须用只读 key**(见 §8):MCP 路径上危险工具**不弹确认**
(`internal/mcp/server.go:47-53`),只有角色闸门(`:85-93`)。用 admin key 挂给
外部 agent = 把「无需确认的写操作」交出去。

> **落地时发现并修掉的缺口**:本仓原先是**单一全局 API Key + 单一全局角色**
> (`biz/handler/git_sync/apikey.go`),拿不到「只读 key」。
> 已补**按 key 分角色**(`SetScopedAPIKey` / `ResolveAPIKeyRole`),
> 外部 agent 的 MCP key 固定为 readonly(`SetAIMCPAPIKey`),旧 key 立即失效。

---

## 6. 事件映射(前端尽量零改动)

现有事件(`internal/agent/event.go`):`start | delta | tool_start | tool_end | tool_confirm | done | error`。

| ACP / acpx 事件 | git-ferry Event | 备注 |
|-------------------|-----------------|------|
| 会话建立 | `start`(+session_id) | 与内部后端同形 |
| `agent_message_chunk` / acpx 文本事件 | `delta` | 直接复用 |
| `agent_thought_chunk` / acpx thinking | `thinking`(**新增类型**) | 前端可选渲染;不渲染也不破 |
| `tool_call`(开始) | `tool_start`(tool=工具名, args=入参) | 复用现有前端渲染 |
| `tool_call_update`(完成/失败) | `tool_end`(result=输出摘要) | 复用现有前端渲染 |
| `session/request_permission` | `tool_confirm`(token=permission request id) | **需扩展确认语义**(见 §7) |
| `plan` | `plan`(**新增类型,可选**) | 前端可选 |
| 回合结束 / stopReason | `done` | 复用 |
| 进程错误 / RPC 错误 | `error` | 复用 |

**新增事件类型的兼容策略**:前端 `AssistantPanel.vue` 只认识已知类型就渲染、未知就忽略 ⇒
新增 `thinking`/`plan` 不需要前端同步改就能上线(先不显示思考过程)。

---

## 7. 权限模型

### 7.1 三个权限面,别混

| 面 | 现状 | ACP 场景 |
|----|------|----------|
| **git-ferry 角色**(`admin`/`operator`/`readonly`) | 工具层自判,经 ctx 传递 | 外部 agent 的 MCP key 决定它能碰哪些工具 |
| **git-ferry 工具权限档**(`default`/`readonly`/`audit`,permgate) | 会话级,切档需 admin + 确认 | 内置助手链路不变 |
| **外部 agent 自身权限**(ACP `session/request_permission` / acpx 权限档) | 不存在 | **新增面**,阶段 3 接入 |

### 7.2 外部 agent 权限档设计

| 档 | 语义 | 落地 |
|----|------|------|
| `ask`(默认) | agent 每请求一次授权就弹确认卡 | 阶段 3:`session/request_permission` → `tool_confirm` → 前端回传 → 回 `selected` option |
| `approve_reads` | 只读类操作自动放行,写类弹确认 | 阶段 3(按 ACP option kind 分类) |
| `approve_all` | 全放行 | **只有 admin 能切**,且切换本身留审计 |
| `deny` | 全拒 | 应急档 |

**默认必须是 `ask`**。理由:外部 agent 在 git-ferry 主机上跑,拥有该用户全部文件与网络权限;
`approve_all` 等于把主机交出去。

### 7.3 确认语义的扩展(不要复用现有令牌)

现有危险工具确认走 `internal/agent/confirm.go` 的**待确认令牌**(5 分钟过期,`ConsumePending`)。
ACP permission 是**另一套**:`request id → option id`,由 agent 侧持有状态。

⇒ **新增 IDL 字段,不改旧的**:

```thrift
struct AIPermissionCall {
    1: string requestId (api.json="request_id")
    2: string optionId  (api.json="option_id")
    3: string sessionId  (api.json="session_id")
}
struct AIChatRequest {
    ...
    4: optional AIPermissionCall permissionCall (api.json="permission_call")
}
```

引擎侧维护 `pendingPermission map[requestID]pendingPerm`,会话结束即清理(超时按 ACP 约定回 `cancelled`)。

---

## 8. 安全模型(本设计的最大风险面)

外部 agent 是**跑在 git-ferry 主机上的子进程**,权限等于服务运行用户。必须写进设计并在代码里落地:

| # | 风险 | 措施 |
|---|------|------|
| S1 | agent 读写服务目录/密钥文件 | `cwd` 固定到专用工作区(默认 `var/ai-workspace`);**不**指向服务目录;代码层校验 cwd 必须在允许根下 |
| S2 | agent 拿到 admin 权限的 MCP key | 专用 key 从环境变量取(`GITFERRY_AI_MCP_API_KEY`),**角色=readonly**;不复用 admin key |
| S2b | **agent 子进程继承服务密钥** | Go 的 `exec.Cmd.Env` 为 nil ⇒ 继承父进程环境 ⇒ agent 能读到 `ENCRYPTION_KEY`(凭据加密主密钥!)与 admin 主 key ⇒ **绕开 MCP 的只读边界**直调写接口。修法:给 agent 进程**净化环境**(精确摘掉 `ENCRYPTION_KEY` 与 `GIT_SYNC_*`/`GITFERRY_*` 前缀;agent 自己的 provider 鉴权要留) |
| S3 | 外部 agent 绕过 git-ferry 确认链路 | 默认权限档 `ask`;`approve_all` 仅 admin 可切且留审计 |
| S4 | 改后端/改权限档是治理动作 | `ai.backend` 与 `ai.external.permission` 的修改走 admin 闸门(复用 `AdminGuard` 语义),并写操作日志 |
| S5 | 外部 agent 的行为不进审计 | `tool_call` / permission 决策落 `operation_logs`(走既有审计链),字段含 agent 名、cwd、权限档 |
| S6 | 密钥进配置文件 | 沿用本仓约定:密钥只进 `data/*.json`(0600)或环境变量,**不进 yaml** |
| S7 | 静默降级 | adapter 缺失/启动失败 ⇒ 明确报错(带安装指引),**不自动回落内置助手**(拒绝静默失败) |
| S8 | 外部 agent 网络出口 | 本仓生产部署在无公网出口的内网 ⇒ 外部 agent 的模型调用天然受限;这是**已知约束**,要在界面上说清,不能让人以为配了就能用 |

> S2 的一个补充:MCP 端点当前对危险工具是「角色闸门放行即执行」。
> 若后续要给外部 agent 写权限,正解是**在 MCP 端点加一条独立的授权策略**(按 key/会话维度),
> 而不是复用「客户端已确认」那套 —— 后者的语义是「调用方自己问过用户」,对外部 agent 不成立。

---

## 9. 配置与契约面

### 9.1 yaml(`conf/config.yaml` 的 `ai:` 段)

```yaml
ai:
  enabled: true
  backend: internal          # internal | acpx | acp
  # —— internal 后端沿用现有字段 ——
  base_url: ""
  model: ""
  # —— 外部 agent(backend=acpx|acp 时生效)——
  external:
    agent: mcode             # acpx 内置 agent 名
    command: ""              # 自定义命令(优先于 agent 名,对应 acpx --agent)
    args: []                 # 追加参数
    cwd: var/ai-workspace    # 沙箱工作目录
    timeout_seconds: 600
    permission: ask          # ask | approve_reads | approve_all | deny
    mcp_enabled: true        # 是否把 git-ferry 自己的 /mcp 挂给外部 agent
    mcp_api_key_env: GITFERRY_AI_MCP_API_KEY
```

### 9.2 设置文件(`data/ai-settings.json`)

`Settings` 增加 `backend` + `external` 两段(与 yaml 同构),前端可改、热生效。
`SettingsView` 里 **不回传** `mcp_api_key_env` 指向的密钥(只回 `has_mcp_key` + 掩码)。

### 9.3 校验按后端分支(不是加兜底默认值)

`Config.Validate` 现在无条件要求 `base_url` + `model` + apiKey。改造后:

```go
switch c.Backend {
case "internal":
    if c.BaseURL == "" { return errors.New("ai.backend=internal 需要 ai.base_url") }
    if c.Model == ""   { return errors.New("ai.backend=internal 需要 ai.model") }
    if apiKey == ""    { return fmt.Errorf("ai.backend=internal 需要环境变量 %s", APIKeyEnvVar) }
case "acpx", "acp":
    if c.External.Agent == "" && c.External.Command == "" {
        return errors.New("ai.backend=acpx 需要 ai.external.agent 或 ai.external.command")
    }
    // command 不在 PATH ⇒ 明确报错(带安装指引),不静默回落
default:
    return fmt.Errorf("未知 ai.backend: %s(可选 internal|acpx|acp)", c.Backend)
}
```

> 这是**按后端分支的校验**,不是「给调用方兜底默认值」—— 前端选哪个后端,服务端就按那个后端的要求校验。

### 9.4 API 面(IDL 变更)

```thrift
struct AIConfigRequest {
    ... 现有 8 字段 ...
    9: optional string backend (api.json="backend")
    10: optional AIExternalConfig external (api.json="external")
}
struct AIExternalConfig {
    1: string agent (api.json="agent")
    2: string command (api.json="command")
    3: list<string> args (api.json="args")
    4: string cwd (api.json="cwd")
    5: i32 timeoutSeconds (api.json="timeout_seconds")
    6: string permission (api.json="permission")
    7: bool mcpEnabled (api.json="mcp_enabled")
}
```

`GET /api/v1/ai/status` 增加:`backend` / `agent_label` / `external_ready`(二进制是否就位)。

`POST /api/v1/ai/config/test` 按后端分支:
- internal → 现有 OpenAI 兼容探测(不动)
- acpx/acp → 探测命令是否存在 + 能否 `initialize`/跑一次极短 prompt

> 注意:仓库纪律「IDL → hz 生成 → OpenAPI 三者必须同步」⇒ 改 IDL 后必须跑 `make generate` + `make apidoc`。

### 9.5 前端

- `frontend/src/views/settings/AISettings.vue`:后端选择器(内置助手 / 外部 coding agent)+ 外部参数表单 + 「测试连接」。
- `frontend/src/components/ai/AssistantPanel.vue`:`thinking`/`plan` 事件可选渲染(不认识就忽略)。

---

## 10. 会话、并发与生命周期

| 维度 | internal 后端 | acpx 后端 | acp(原生)后端 |
|------|---------------|-----------|----------------|
| 会话状态 | git-ferry 内存 `SessionStore`(30 分钟 TTL) | 同左 | git-ferry 侧要存 agent 的 `sessionId` |
| 外部进程 | 无 | **无长驻进程**(每次 prompt 一次 CLI 调用) | **每会话一个长驻子进程** ⇒ 要做进程池 + TTL 回收 + 崩溃重启 |
| 并发闸门 | `syncx.Gate`(MaxConcurrentChats) | 同左 | 同左 + 进程数上限(两者取小) |
| 历史 | git-ferry 侧存,每次带进模型 | git-ferry 侧存 → **拼成 prompt 文本**带给外部 agent | git-ferry 侧存;agent 侧也有自己的上下文 ⇒ **双份上下文**,需明确谁为准 |
| 取消 | ctx cancel → 模型流停 | ctx cancel → kill CLI 进程 | ctx cancel → `session/cancel` + 超时 kill |

**必须明确的两个语义**(否则会出现「界面显示的历史」与「agent 实际记得的」不一致):

1. **acpx 后端**:git-ferry 把会话历史压成一段文本随 prompt 发送 ⇒ agent 侧不是真多轮会话,
   真多轮靠 acpx 自己的 `sessions`(可按 git-ferry session id 映射命名)。
2. **acp 后端**:agent 侧有真会话 ⇒ **agent 的记忆是权威**,git-ferry 侧历史只作展示。
   建议:acp 后端下 git-ferry 侧不再回灌历史,只记消息用于审计与前端展示。

---

## 11. 分阶段落地

### 阶段 0:引擎抽象(纯搬迁,不改行为)

- 建 `internal/agent/engine` 包(Event/Usage/Engine/Session 接口)。
- eino 实现搬到 `internal/agent/engine/internal`;`agent.Runner` 改为持有 `engine.Engine`。
- `type Event = engine.Event` 别名保持现有引用不变。
- **验收**:现有 `internal/agent` 与 `internal/mcp` 的全部单测通过;`/api/v1/ai/chat` 行为逐字不变。

### 阶段 1:acpx 通道 + 委派工具(可见价值)

- `internal/agent/engine/acp`:acpx 子进程执行器(NDJSON → `engine.Event`)。
- 新工具 `ai_code_delegate`:内置助手可把编码任务派给外部 agent(参数:`agent`/`prompt`/`cwd`/`timeout`)。
  - **该工具标为危险**(`addDanger`)⇒ 走既有确认链路。
- 配置:`ai.backend=acpx` 时整段会话由外部 agent 承接;`internal` 时该工具可单独使用。
- **验收**:
  1. 本机 `mcode acp` 跑通一次真实回合(产出 delta + done)。
  2. 探针脚本把 acpx NDJSON 真实输出固化成测试夹具(**先实测再写解析器**)。
  3. 外部 agent 缺失时:明确报错 + 安装指引,**不回落内置助手**。

### 阶段 2:后端整体切换 + 配置面

- `ai.backend` / `external` 配置段 + 设置文件 + IDL + 前端表单 + 「测试连接」。
- `GET /api/v1/ai/status` 回报当前后端。
- **验收**:界面上切后端 → 热生效 → 不用重启。

### 阶段 3:原生 ACP + 权限确认 + 领域工具打通

- `internal/agent/engine/acp` 的 stdio transport:`initialize`/`session/new`/`session/prompt`/`session/update`/`session/cancel`。
- client 侧能力:`fs/*` + `terminal/*`。
- `session/new.mcpServers` 挂 git-ferry 自己的 `/mcp`(专用只读 key)。
- `session/request_permission` → `tool_confirm` → 前端确认 → 回传 → `IDL` 加 `permissionCall`。
- 审计:tool_call / permission 决策落 `operation_logs`。
- **验收**:外部 agent 通过 MCP 读到 git-ferry 的仓库/任务数据;危险写操作在界面上先弹确认。

---

## 12. 代码改动清单

```
internal/agent/engine/engine.go              新增  Engine 接口 / Event / Usage / Session
internal/agent/engine/internal/engine.go     新增  eino 引擎(自 runner.go 搬)
internal/agent/engine/internal/*_test.go     迁移  runner_test 等
internal/agent/engine/acp/acpx.go            新增  acpx 子进程执行器
internal/agent/engine/acp/acpx_events.go     新增  NDJSON → Event 映射
internal/agent/engine/acp/acpx_test.go       新增  (夹具 = 实测输出)
internal/agent/engine/acp/acp.go             新增(阶段 3)原生 stdio transport
internal/agent/engine/acp/acp_permission.go  新增(阶段 3)permission 请求挂起/应答
internal/agent/event.go                      改动  Event/Usage 改为 engine 别名
internal/agent/runner.go                     改动  Runner 持有 engine.Engine;对外 API 不变
internal/agent/config.go                     改动  Config 加 Backend / External;Validate 按后端分支
internal/agent/settings.go                   改动  Settings 加 Backend / External;View 脱敏
internal/agent/tools/delegate.go             新增  ai_code_delegate 工具(危险)
internal/agent/tools/repo.go                 改动  注册 delegate 工具
biz/handler/git_sync/ai_config.go            改动  保存/校验/测试连通性按后端分支
biz/handler/git_sync/ai_service.go           改动  status 回报后端;确认路径支持 permissionCall
idl/ai.thrift                                改动  backend / external / AIPermissionCall
frontend/.../AISettings.vue                  改动  后端选择 + 外部参数 + 测试连接
frontend/.../AssistantPanel.vue               改动  thinking/plan 可选渲染
docs/design/acp-external-agent-design.md     新增  本文档
```

---

## 13. 风险与取舍

| 风险 | 影响 | 缓解 |
|------|------|------|
| acpx 是 pre-1.0 alpha,CLI/运行时接口可能变 | 阶段 1 的解析层被打断 | 解析层只依赖 **NDJSON 事件**;acpx 只当**启动器**;真正稳态走原生 ACP |
| ACP 协议仍在演进 | 阶段 3 的类型层要改 | 只依赖 **stable schema(v1)**;draft 特性隔离;扩展方法用 `_` 前缀命名 |
| 外部 agent 与 git-ferry 的权限模型割裂 | 治理缺口 | 明确三条权限面(§7);外部 agent 默认只读 + `ask` 档;一切进审计 |
| 外部 agent 在控制面主机上有全权 | **最大风险** | cwd 沙箱 + 只读 MCP key + 默认 `ask`;建议部署侧再套容器隔离(代码层不解决) |
| 上下文双份(agent 侧 vs git-ferry 侧) | 用户看到的与 agent 记得的不一致 | 阶段 2 明确「acpx 下历史随 prompt 发送;acp 下 agent 侧为准」 |
| `make generate`/`make apidoc` 漏跑 | 契约漂移,CI 门禁挂 | IDL 变更与生成同一次提交 |

---

## 14. 明确不做的事

- 不写自己的 ACP 协议实现(除非 SDK 有真缺口,且缺口要提上游 issue)。
- 不在仓里做「acpx 的封装生成器」—— 用现成 CLI。
- 不把外部 agent 的会话状态存进 `git-ferry-core`(业务规则与数据在 core,AI 在壳层)。
- 不让外部 agent 直接连 core 的数据库。
- 不为 acpx/acp 写「适配每个 agent 的 MCP 配置模板」—— 原生 ACP 的 `mcpServers` 一处配好(阶段 3)。

---

## 15. 参考

**协议与工具**

- Agent Client Protocol(规范):https://agentclientprotocol.com
- ACP Go SDK(Coder):https://github.com/coder/acp-go-sdk · https://pkg.go.dev/github.com/coder/acp-go-sdk
- acp-go(非官方,`acp1`/`acp2` 分层):https://github.com/ironpark/acp-go
- acpx(ACP 无头 CLI 客户端):https://github.com/openclaw/acpx · https://acpx.sh
- MiniMax Code 作为 ACP server:`mcode acp`(本机实测 `--help` 确认)

**本仓相关**

- `internal/agent/runner.go` · `internal/agent/event.go` · `internal/agent/decorator.go` · `internal/agent/confirm.go`
- `internal/agent/engine/engine.go` · `internal/agent/engine_eino.go` · `internal/agent/engine/acp/*`
- `internal/agent/tools/permission.go` · `internal/agent/tools/repo.go` · `internal/agent/tools/delegate.go`
- `internal/mcp/server.go` · `biz/handler/git_sync/ai_service.go` · `biz/handler/git_sync/ai_config.go`
- `biz/handler/git_sync/apikey.go`(按 key 分角色)
- `ARCHITECTURE.md` §7.5

---

## 16. 落地实况(2026-10-26 实施后补记)

### 16.1 协议形状全部来自实测,不是照抄文档

用探针脚本对 `mcode acp` 与 `acpx 0.21.0` 实测,固化在
`internal/agent/engine/acp/testdata/`(两个夹具)并由单测钉住。

| 事实 | 实测结论 |
|------|---------|
| `initialize` 响应 | `{protocolVersion, agentCapabilities:{loadSession, mcpCapabilities:{http,sse}, promptCapabilities, sessionCapabilities}, agentInfo:{name,title,version}, _meta}` |
| `session/new` 响应 | `{sessionId, modes:{currentModeId, availableModes}}`(+ agent 扩展 `configOptions`) |
| `session/prompt` 响应 | `{stopReason:"end_turn"}` |
| `session/update` 的 `agent_message_chunk` | `{sessionUpdate, messageId, content:{type:"text",text}}` |
| `session/update` 的 `tool_call` | `{sessionUpdate, toolCallId, title, name, kind}` |
| `session/update` 的 `tool_call_update` | `{sessionUpdate, toolCallId, status, rawInput, rawOutput, locations}` —— **可能不带 name/title** ⇒ 工具名要在 `tool_call` 时记住 |
| `rawOutput` | `{content:[{type:"text",text}], details:{...}}` |
| mcode 的扩展更新类型 | `session_info_update` / `usage_update`(非 ACP 基线,忽略即可) |
| **acpx `--format json` 的输出** | **就是原始 ACP JSON-RPC 流**(不是投影)⇒ 两种 transport 共用同一个解析器 |
| acpx 的 stdout | 会夹**非 JSON 横幅行**(`[acpx] session … · agent …`)⇒ 解析器必须跳过并计数 |
| acpx 的会话模型 | 提示前要先有会话;`sessions new --name <n>` 建,`-s <n>` 复用;作用域 = (agent, cwd, 名字) |
| `mcode acp` 的文件读取 | **不使用** client 的 fs 能力(它自己读);但 claude-agent-acp / codex-acp 一类会要求 ⇒ fs/terminal 仍要实现 |

### 16.2 与初版设计的偏差

| # | 偏差 | 原因 |
|---|------|------|
| D1 | eino 实现**不搬**到 `engine/internal` | 与 agent 包的 prompt/装饰器/落盘/会话深度耦合,搬动收益低于回归风险;抽象只落在契约层 |
| D2 | 新增**按 key 分角色**的鉴权 | 原设计要求给外部 agent 一个只读 key,但本仓原先只有单一全局 key + 单一全局角色 ⇒ 必须补 |
| D3 | acpx transport 与 stdio transport **共用** JSON-RPC 解析 | 实测 acpx 输出即原始 ACP 流,没必要写两套 |
| D4 | 委派工具命名为 `ai_code_delegate` | 「委派」与「后端切换」是两条路,名字要能一眼区分 |

### 16.3 已验证的事实

- 两条 transport 都对真实 agent(`mcode`)跑通完整回合(acpx 20.7s / stdio 7.6s,均产出 `delta` + `done`)。
- 外部 agent 挂上 MCP 后**真的调通了工具**(见 `TestE2E_Stdio_MCPMounted`)。
- 工具数 41 → **42**(新增 `ai_code_delegate`,危险标注在注册处)。

---

## 17. 补完轮(2026-10-26 二次实施)

初版实现有几个洞,这一轮补上。**其中三个是真 bug,是测试逼出来的** ——
写测试时才发现「看起来对」的代码其实是错的。

### 17.1 补上的东西

| # | 缺口 | 处置 |
|---|------|------|
| F1 | **AI 配置端点没有任何 RBAC 闸门** | 只读角色也能改 AI 配置;换后端/改授权档 = 治理动作,仅 admin;留审计(`ai.backend.change`) |
| F2 | 「测试连接」只查文件在不在 | stdio 后端改为**真握手**(起进程 → initialize → 读响应);acpx 查版本 + agent 可执行性 |
| F3 | 状态接口不报外部 agent 是否就位 | `/api/v1/ai/status` 增 `ready` / `ready_msg`(**静态就位性,快**;握手留给测试连接) |
| F4 | 回收进程会打断在飞的回合 | `sessionProc` 加在飞计数,回收**跳过有回合在飞**的进程 |
| F5 | 取消回合没通知 agent | 补 `session/cancel` 通知(用 `finished` 区分「正常结束」与「被取消」,否则回合结束的 defer 会误发一次 cancel 把下一轮也取消) |
| F6 | 文档不可达 | README(中英)补「换成外部 AI coding 工具(ACP)」一节 |
| F7 | MCP 挂载**只支持一个源**(布尔开关 + URL) | 改为**列表**:`external.mcp_servers` 可挂多个第三方 MCP;最终清单 = 本服务 /mcp(若启用)+ 列表 |
| F8 | **换后端热重建时旧后端不关** | 外部后端每会话一个**长驻 agent 子进程**,不关旧的就攒孤儿进程 ⇒ 补 `Runner.Close()`(同时停掉会话回收 goroutine),重建与进程退出路径都调 |
| F9 | **yaml 的 `mcp_enabled: true` 永远不生效** | 布尔的「未设置 vs 显式 false」没法区分 ⇒ `MergeSettings` 漏合并它 ⇒ 设置文件没写过之前,yaml 的布尔值说了算 |
| F10 | **acpx 通道的 `ask` 档名不副实** | acpx 非交互(它的 stdin/stdout 被我们占着)⇒ 弹不出授权卡,`nonInteractivePermissions` 默认 deny ⇒ 「问」实际等于「拒绝」⇒ 校验阶段直接拒绝该组合,并在**保存时**就拒(不存「起不来」的配置);参数映射改成显式给 `--approve-reads`,不吃默认值 |
| F11 | **外部 agent 的会话 ID 没透出** | 外部 agent 与本服务可能**共用同一个会话库**(MiniMax Code 的 CLI 与桌面版)⇒ 新增 `agent_session` 事件带出 `agent_session_id`,用户才能在那个界面上找到这次会话 |
| F12 | **fs 沙箱不解析符号链接** | 只做 `filepath.Clean` 时,工作区里一个指向 `~/.ssh/id_rsa` 的软链路径看着仍在根下 ⇒ 沙箱形同虚设。改法:`EvalSymlinks` 解析后再判包含;根目录本身也要解析(macOS 的 `/var` → `/private/var` 会造成误判) |
| F13 | **terminal 无上限** | 终端只在 agent 调 `terminal/release` 时才释放 ⇒ agent 不释放就一直涨(内存泄漏)。改法:加存活上限(16),超了明确报错让 agent 自己放 |
| F14 | **acpx 会话泄漏到磁盘** | acpx 的会话是**磁盘上的持久记录**(`~/.acpx/sessions/`),git-ferry 会话 30 分钟过期但 acpx 会话不会自己消失 ⇒ 长期运行就把磁盘喂饱。改法:引擎关时关掉本进程建过的会话 + 空闲回收 |
| F15 | **同一会话并发回合互相截胡** | 同一个 git-ferry 会话并发发两轮时,共享的 sink 被覆盖(第一轮事件流到第二轮通道),且 ACP 会话一次只处理一个 prompt。改法:每会话**串行化**回合 |

### 17.2 测试逼出来的三个真 bug

| # | bug | 症状 | 修法 |
|---|-----|------|------|
| B1 | `SettingsStore.Save` 的 **api_key 校验漏在后端分支外** | 切到外部后端也强制要 OpenAI 的模型 Key ⇒ **外部后端根本存不下去** | 密钥校验移进 `BackendInternal` 分支(外部后端不走 OpenAI 端点) |
| B2 | 治理闸门比较后端时**没归一化存量值**(空串 ≠ `internal`) | 任何一次普通保存都被当成「换了后端」⇒ operator 改个模型也要 admin | 两边都归一化(空 → `internal` / `ask`) |
| B3 | 回收进程的 `go oldest.close()` **panic 会崩掉整个服务** | 后台 goroutine 的 panic 无人接 ⇒ 直接带走进程 | 回收放进 `async.GoSafe`(可恢复);`close()` 补 nil 保护 |

> **B2 的教训**:治理闸门的「有没有变更」判定,必须**两边用同一套归一化**再比。
> 否则「没填」会被当成「改了值」,把正常操作卡死 —— 而这恰恰是最容易被忽略的一类错,
> 因为它「看起来是安全方向的失败」(宁可多拒),不会有人第一时间怀疑它。

### 17.3 已知行为:委派会占住确认请求

`ai_code_delegate` 走危险工具确认链路,而确认请求是**同步执行**的 ⇒
外部 agent 跑多久,那条 SSE 连接就开多久(服务端 `server.Default` 不设写超时,
长 SSE 不受影响)。前端表现为:确认卡消失 → 工具行停在「调用中…」→ 结果到达时更新。

这是刻意的:委派结果要**回到助手的上下文里**,异步化就得另建一套「取回结果」通道。
代价是**确认请求会挂很久**,已写进 `skills/gitferry-histsync/SKILL.md` 的治理清单。

### 17.4 与 MiniMax Code Desktop 的对接(实测结论)

对接时查清的三件事(都不是文档里写的,是实测出来的):

| # | 事实 | 依据 |
|---|------|------|
| D1 | **桌面版本身不能当 ACP agent** | `/Applications/MiniMax Code.app` 的 asar 里**没有任何 acp 入口**(36077 个条目里 acp 只匹配到两个编解码器);它是 `@mmx-agent/electron` 纯 GUI,无 bin |
| D2 | ACP server 在 **CLI**:`mcode acp`(同产品的终端 agent) | `mcode --help`:`acp — Run MiniMax Code as an Agent Client Protocol server over stdio` |
| D3 | **CLI 与桌面版共用同一个 v2 会话库** ⇒ git-ferry 驱动的外部会话**会出现在桌面版的会话列表里** | ACP 建的会话落 `~/.minimax/v2/sessions/YYYY/MM/DD/HH-MM-SS-mmm-session_<base64(sessionId)>/`,与桌面版同格式(`source: local-runtime`, `layout: v2-final-dated-session`) |

**完整回路实测通过**(git-ferry → 外部 agent → 经 MCP 调回 git-ferry 自己的工具):

```
POST /api/v1/ai/chat  (backend=stdio, command="mcode acp", MCP 挂本服务 /mcp, 专用只读 key)
  → 事件流:start → thinking* → delta* → tool_start(mcp__gitferry__list_repos) → tool_end
            → thinking* → delta*(答案) → done
  → agent 拿到真实数据(total=311,分页 20/页)并给出观察(平台全是 gitea、默认分支不统一)
  → 该会话在桌面版会话库里可见,messages.jsonl 里有完整的 user/assistant/toolResult/assistant 四条
```

**因此「对接 desktop」的真实落点**是:桌面版作为**会话与人工接管的界面**,agent 运行时由 CLI 提供。
要在桌面版继续这次会话,靠的是同一份会话库,不是另开一条协议。

⇒ 落地时必须**把 agent 侧的会话 ID 透出来**(F11):否则用户只能去对方的会话列表里大海捞针,
这个能力等于没有入口。事件流新增 `agent_session` 事件(带 `agent_session_id` + `agent_label`),
前端在助手面板头部显示成 chip。

### 17.5 一个容易被忽略的通道差异

| 能力 | stdio(原生 ACP) | acpx |
|------|-----------------|------|
| 逐次弹授权卡 | ✅(`session/request_permission` → 界面授权卡) | ❌ 非交互 ⇒ 写操作按拒绝处理 |
| 挂本服务 `/mcp` | ✅(`session/new` 的 mcpServers) | ⚠️ 取决于各 adapter 自己的 MCP 配置 |
| 拿得到 agent 会话 ID | ✅(透出 `agent_session` 事件) | ❌(会话在 acpx 自己的存储里) |

⇒ **要「逐次确认」或「挂本服务 MCP」就得用 stdio 后端**;acpx 适合「一个命令面对
20+ 个 agent、免写协议」的轻量场景。这个差异在配置校验阶段就告诉操作者,不留到运行时才发现。

### 17.5.1 一轮「换了眼睛」的自审发现的东西

F12–F15 是**事后**才发现的,共同点:它们都在「代码看起来对」的地方 ——
fs 沙箱的包含判断、terminal 的生命周期、acpx 会话的去向、并发回合的事件路由。
**没有一条是靠读代码能看出来的**:要么要构造越界的软链,要么要造并发,
要么要长跑才会显现。

⇒ 结论与之前那条一致,但更具体了:**边界与生命周期的 bug,只能用「构造异常输入」和
「制造并发」去找,不能靠 review**。这四条现在都有对应的回归用例。

### 17.6 一次「只读边界」的实测,以及它被捅穿的路径

做「外部 agent 能不能写 git-ferry」的实测时,发现了**两件事**:

**① 只读边界生效,但靠的不是我以为的机制。**
让 agent 调 `create_sync_task` → 工具**根本不在它的列表里**:
MCP 端点对只读角色做 `tools/list` 过滤 ⇒ 危险工具对它**不可见**(不只是被拒)。
agent 自己的结论是「MCP 服务端只暴露了 31 个只读/运维工具,没有 create_sync_task」。

**② 但它会绕路 —— 而且差点成功。**
agent 接着用自带的 `bash` 去翻 git-ferry 源码,找到了 CLI 的 `task create`
(→ `POST /api/v1/sync/task/create`),开始找「MCP 之外的写入通道」。

顺着这条线查下去,发现了**真漏洞**:agent 子进程**继承了服务的环境变量**。
实测让 agent 打印自己的 env,拿到的是:

```
GIT_SYNC_SERVER_API_KEY=local-shot-9f2a4c7b     ← admin 角色的主 key
ENCRYPTION_KEY=6ed209e9…                          ← 凭据加密主密钥(可解密库里所有 git 凭据)
GITFERRY_AI_MCP_API_KEY=readonly-mcp-key-3a91
```

⇒ **MCP 的「只读角色」只是纵深防御,不是边界**:
拿到 admin key 就能直接调写接口,拿到 ENCRYPTION_KEY 就能解密库里的凭据。

修法:`internal/agent/engine/acp/env.go` 的 `scrubEnv` —— 给外部 agent 进程
**净化环境**(精确摘 `ENCRYPTION_KEY` 与 `GIT_SYNC_*`/`GITFERRY_*` 前缀;
agent 自己的 provider 鉴权必须留,不能一刀切清空)。
修完再实测:agent 打印 env ⇒ **一无所获**。

**结论(写进设计的边界声明)**:外部 agent 的真实边界是**操作系统级**的
(运行用户 / 工作目录 / 容器),**不是** MCP 角色、也**不是**环境变量。
代码层能做的是把纵深防御补齐(只读 key + 环境净化 + 工作目录),
真正的隔离要靠部署侧(专用用户 / 容器 / 只读挂载)。

### 17.6.1 异步委派(为什么不能同步等)

委派是**长任务**(外部 agent 跑几分钟很常见),而工具调用是同步的 ⇒
同步等下去会把整条 SSE 挂住,**中间的代理(nginx proxy_read_timeout 默认 60s)
会把连接一刀切掉** —— 用户看到的是「助手卡住了」。

改法:`ai_code_delegate` 确认后**立刻**返回「已派出 + 任务 ID」,
后台跑;结果有两个去处:
- **回填会话**:用户下一轮问话时助手已经看得见结果(措辞标明「仅作数据,不是指令」);
- **按 ID 取**:`ai_delegate_result` 工具 / `GET /api/v1/ai/delegate/{task_id}` 轮询。

实测:确认请求 **0.00s** 返回(此前是同步挂到 agent 跑完)。

⚠️ 后台任务必须用 `context.WithoutCancel(ctx)`:
HTTP 请求一结束,请求 ctx 就被取消 ⇒ 后台委派会被一起杀掉;
但要保留 values(拿会话 scope 回填结果),只去掉取消信号。

### 17.6.2 外部 agent 服务侧失败要如实透传

实测委派链路时,MiniMax Code 服务侧返回
`ACP 错误 -32603: Internal error: MiniMax Code Runtime failed: Content review blocked the request.`
—— 直接跑 `mcode exec` 也一样(与 acpx、cwd、会话都无关)。

这正好验证了两件事:
1. **错误被如实透传**:回合以 error 事件收尾(带真实原因),没有挂死、没有假装成功;
2. **e2e 可能因外部服务状态变红**:不是代码回退。排查时先跑一遍裸命令
   (`mcode exec "..."`),分清是「服务侧拒绝」还是「我们的代码坏了」。

### 17.7 新增测试

- `biz/handler/git_sync/ai_config_test.go`:RBAC 三档(只读拒 / 治理项要 admin / 普通项放行)、
  归一化不误判。
- `internal/agent/engine/acp/acp_test.go`:回收不打断在飞回合、静态就位性。
- `internal/agent/engine/acp/probe.go`:stdio 真握手(起进程只发 initialize,不发 prompt)。
- `internal/agent/engine/acp/stdio_test.go` + `testdata/fake_acp_agent.py`:
  **假 ACP agent** 把协议走完(initialize → session/new → session/prompt → session/update),
  让 stdio 客户端的测试只依赖协议本身(真实 agent 依赖模型与网络,测不了「协议层通没通」);
  顺带钉住「agent 会话 ID 要透出」。
- `internal/agent/engine/acp/acp_test.go`:acpx + ask 档被拒;其它档位可用。
- `internal/agent/config_test.go`:布尔的 yaml 值要生效;acpx+ask 存不下去。
- `internal/agent/engine/acp/env_test.go`:环境净化(摘本服务密钥、不误伤 agent 自己的 provider key)。
- `internal/agent/delegate_e2e_test.go`:委派链路端到端(模型发起 → 危险工具要确认 → 确认后真的派出去)。
