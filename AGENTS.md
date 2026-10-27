# AGENTS.md —— 面向 AI Agent 的规范与硬规则

> 本文件是**给 AI Agent 看的**仓库规范:人读架构看 [`ARCHITECTURE.md`](ARCHITECTURE.md)，
> 人读贡献流程看 [`CONTRIBUTING.md`](CONTRIBUTING.md)。
> 本仓是 [oekit](https://github.com/oekit) 家族的使用方 —— 下面第 1 节是 oekit 的
> 硬规则,本仓**逐条落地**并用机械闸门守住(第 2 节)。

## 0. 先读这几份

| 想知道 | 读 |
|---|---|
| 当前架构 | `ARCHITECTURE.md` |
| 外部 AI coding 工具(ACP)的设计与实测 | `docs/design/acp-external-agent-design.md` |
| 领域工具(MCP)的清单与语义 | `skills/`、`internal/mcp/` |
| 发版纪律与门禁 | `internal/release/`、本文第 4 节 |

## 1. oekit 硬规则(逐条落地)

oekit 的规则见 <https://oekit.github.io/llms.txt>。本仓的落地口径:

### 1.1 错误处理必须用 `pkg/errors`

- 用 `github.com/oekit/pkg/errors`,**不要**用 stdlib `errors`
  (两者**同名**:stdlib 的 `Is`/`As` 与 oekit 的判定跨体系时,哨兵匹配会失效 ——
  oekit 的红线叫「类型同一性」,一个 import 即可)。
- 包装错误用 `errors.Wrap(err, "上下文")` —— 它显式 nil-safe,还能挂 kv 上下文
  (`errors.Wrap(err, "启动 agent 失败", "command", cmd)`)。
  **不要**用 `fmt.Errorf("...: %w", err)`。
- 纯格式化(不包装任何错误)用 `fmt.Errorf` 即可,不必引 errors 包。
- **业务错误自带状态码**:用 `errors.BizError`
  (本仓封装为 `response.NewBizError(状态码, 消息)`),
  由 `response.Biz` / `response.FromError` 统一渲染 ——
  「这个错误该落成 403 还是 400」由**产生错误的地方**定,不由渲染方猜。

### 1.2 日志用 logx 的 kv 风格,包级 logger 用 `Delegating`

- 每个会打日志的包声明一次:`var logger = logx.Delegating("组件名")`。
- **必须用 `Delegating`,不能用 `logx.Default()`**:包级 var 在 init 时求值,
  直接捕获 `Default()` 会永远绑在 init 那一刻的后端上,消费方随后的
  `SetDefault` 对它无效(日志进黑洞)。`Delegating` 是**调用时**解析。
- 打日志用 `logger.Info("消息", "k", v)` 的 kv 风格。
  **不要**直连 `slog.Info(...)` —— 那绕过了统一门面(级别/脱敏/trace_id/审计全失效)。
- 后端接线在 `biz/serve.SetupLogger`:logx 接到**与默认 slog 同一个 handler**上,
  格式与级别与既有日志完全一致。

### 1.3 禁止裸 `go func()`

- 一律用 `github.com/oekit/concurrency/async`:`async.GoSafe` / `async.GoSafeLabeled` /
  `async.GoSafeWithErr`。
- 理由不是洁癖:裸 goroutine 里的 **panic 没人接** ⇒ 直接带走整个进程。
  对本仓这种长驻进程(MCP + AI 助手同在一个进程)尤其致命。

### 1.4 能用 oekit 原语就别手搓

优先用现成原语,别自己写一份:

| 需要 | 用 |
|---|---|
| 错误包装/业务码 | `pkg/errors` |
| 日志门面 | `logx` |
| panic 收编 | `concurrency/async` |
| 路径清洗/traversal 拒绝 | `pkg/pathx`(`SanitizeRelStrict` 严格档) |
| 分页参数 | `pkg/pagination` |
| UUID / 雪花 ID | `kitx/uidgen` |
| 指针取值 | `pkg/ptrx` |
| 字符串工具 | `pkg/stringx` |
| 并发容器 | `pkg/container` |
| 限流 / 防爆破锁定 | `concurrency/ratelimit` |
| 中间件栈 | `middleware`(`mw.Recovery` 等) |
| HTTP 客户端(重试/熔断) | `httpx` |
| 服务骨架/优雅关停 | `bootstrap` |

**但不是无脑替换**:oekit 没有的能力就自己写,并**注释说明为什么不用 oekit**
(例:oekit 的 `syncx.Gate` 是排队语义,而「满额即拒」要的是 try-acquire)。

## 2. 机械闸门(规则不靠人记)

`internal/convention/` 把上面四条落成**可执行的检查**(CI 跑):

| 门禁 | 拦住什么 |
|---|---|
| `TestConvention_禁止裸goroutine` | 裸 `go func(` |
| `TestConvention_错误处理必须用oekit的errors` | import stdlib `"errors"` |
| `TestConvention_禁止fmtErrorf包w` | `fmt.Errorf(...%w...)` |
| `TestConvention_日志必须走logx门面` | 直连 `slog.Info/Error/Warn/Debug(` |
| `TestConvention_包级logger必须用Delegating` | `= logx.Default()` |

`internal/release/` 是**发版纪律**门禁(破坏性变更必须 major、公开契约删除必须 major)。

## 3. 本仓自己的硬规矩

1. **拒绝静默失败 / 静默降级**:配置校验放在**保存时**而不是只在启动时;
   外部 agent 的错误如实透传给调用方;起不来的后端要**明确报出来**,
   不能悄悄退回内置助手。
2. **单一入口**:同一个概念只允许**一套解析**。本仓吃过亏 ——
   「旧的单 profile 字段」与「新的 profile 列表」两套写法并存,
   校验查旧字段、装配用新字段 ⇒ 界面显示已保存、启动直接失败。
   现在解析只有 `agent.Config.EffectiveExternal()` / `ResolveProfile()` 一处。
3. **公开契约只增不减**:删 path / 删请求参数 ⇒ 必须 major 版本
   (`internal/release` 有机械门禁)。
4. **密钥不进 yaml**:只进环境变量或 `data/*.json`(0600)。
   外部 agent 的 MCP 专用 key 固定**只读角色**。
5. **子进程环境净化**:外部 agent 进程摘掉 `ENCRYPTION_KEY` 与
   `GIT_SYNC_*` / `GITFERRY_*` 前缀(agent 自己的 provider 鉴权要留)。
6. **IDL → hz → OpenAPI 三者同步**:改 `idl/*.thrift` 后跑 `make generate` 与
   `make apidoc`。
7. **提交用 `scripts/release-commit.sh`**:多 agent 会话共享同一个 `.git/index`,
   普通 `git commit` 会把别人 staged 的内容一起带走。

## 4. 依赖与发版

- **依赖地板追最新**:`github.com/oekit/*`、`github.com/oagentkit/*` 一律用**真实发布
  tag**(禁 `replace`;`replace` 会让依赖图里的版本号变成假的)。
  查最新版本:`go list -m -versions <mod>`。
- oekit 模块矩阵的**权威索引**(不要凭记忆猜 API):
  - 模块索引 + AI Agent 硬规则:<https://oekit.github.io/llms.txt>
  - 机器可读版本矩阵:<https://oekit.github.io/versions.json>
- 不确定 API 时**以源码为准**:`go doc github.com/oekit/<mod>[/<pkg>]` 可查,
  每个模块都带测试。
- 发版:`CHANGELOG.md` 写版本段 → `scripts/release-commit.sh` 提交 → 打 `v*` tag
  ⇒ `.github/workflows/release.yml` 出二进制与 CLI 包。
  破坏性措辞(破坏性/不再/移除/字段名/退出码…)会触发 `internal/release` 门禁。

## 5. 提交前自检

```bash
gofmt -l .          # 独立于 lint 的一道闸门,别只信 lint
go vet ./...
go test -race ./...
golangci-lint run
cd frontend && npm run build:check
```
