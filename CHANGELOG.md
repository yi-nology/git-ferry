# Changelog

GitFerry — 自托管 Git 同步/镜像/备份中枢。本文件记录壳层发版变化。

## [v2.3.0] - 2026-10-27

用 [oekit](https://github.com/oekit) 家族规范深度重构一轮，并把**两条通道的能力拉齐**：
外部 agent 现在在 acpx 通道上也能调回本服务的领域工具。

### Added

- **oekit 四条硬规则落地**（`llms.txt` 的「AI Agent 要点」，逐条有闸门）：
  - **错误处理用 `pkg/errors`**：63 处 `fmt.Errorf("...: %w", err)` 改为
    `errors.Wrap(err, msg)`（显式 nil-safe + 能挂 kv 上下文）；stdlib `errors`
    导入全部换成 oekit 的（同名同型，`Is`/`As` 一个 import 即可）。
  - **业务错误自带状态码**：新增 `response.NewBizError` + `response.Biz`，
    RBAC 闸门与 AI 治理闸门改走它 ⇒「这个错误该落成 403 还是 400」由**产生错误
    的地方**定，渲染处不猜；错误响应顺带统一成同一信封。
  - **日志用 logx 的 kv 风格、SDK 包用 `Delegating`**：9 个包各声明
    `var logger = logx.Delegating("组件名")`，30 处直连 `slog.*` 改走包级 logger；
    `serve.SetupLogger` 把 logx 接到**与进程默认 slog 同一个 handler** ⇒
    格式 / 级别 / trace_id 完全一致。包级 logger 必须用 `Delegating`：
    `logx.Default()` 在 init 时求值，会永远绑在 init 那一刻的后端上 ⇒ 日志进黑洞。
  - **禁裸 `go func`**：5 处裸 goroutine 改为 `concurrency/async`
    （裸 goroutine 的 panic 没人接 ⇒ 直接带走整个进程；本仓 MCP 与 AI 助手同进程，
    尤其致命）。
- **机械闸门 `internal/convention/`**：把上面四条变成 **CI 跑的可执行检查**
  （裸 goroutine / stdlib `errors` / `fmt.Errorf %w` / 直连 `slog` /
  `logx.Default()` 包级捕获）。纪律变成闸门，而不是文档里的一句话。
- **仓根 `AGENTS.md`**：面向 AI Agent 的规范 —— 四条硬规则的本仓口径、
  本仓自己的硬规矩（拒绝静默失败 / 单一入口 / 公开契约只增不减 / 密钥不进 yaml /
  子进程环境净化 / IDL→hz→OpenAPI 同步 / 提交用 `release-commit.sh`）、
  依赖地板追最新（真实 tag，禁 `replace`）、提交前自检清单。

### Fixed

- **外部 agent 的 terminal 工作目录没被管**（安全）：`terminal/create` 的 cwd
  此前是**直接用 agent 给的值**，而文档写的是「命令的工作目录固定在 root 内」
  —— 实现根本没管 ⇒ agent 报一个根外的 cwd 就能把命令挪到沙箱外跑。
  改法：`fs/*` 与 `terminal/*` **共用一份沙箱**（相对路径先过
  `pkg/pathx.SanitizeRelStrict` 严格档 → 解析符号链接 → 判包含）；
  根外直接拒，工作目录不存在也拒。
- **acpx 通道没挂本服务的 `/mcp`**：acpx 一直支持 `--mcp-config`，
  是装配层**没写这份配置** ⇒ 界面里 `mcp_enabled: true` 存着、日志不报错、
  后端就是没挂 ⇒ 外部 agent 拿不到领域工具，只能**满环境自己找**
  （实测连续 27 次 `bash`/`grep`，最后 300s 撞超时被杀，用户看到「助手卡住了」）。
  改法：新增 acpx MCP 配置写入（形状以 acpx 0.21.0 源码实测为准：
  `mcpServers` 是**数组**、`headers` 是**数组** `[{name,value}]`、`type` 必须显式写；
  acpx 只能从文件读 ⇒ key 必然落盘一次 ⇒ 私有目录 0700 + 文件 0600 + 跑完即删），
  会话创建 / 对话 / 一次性执行三条路径都挂上。
  **实测：同一个 prompt 从 303s 撞超时被杀，变成 16s 直接调 `mcp__gitferry__list_repos`。**
- **委派执行器也挂 MCP**：被派的 agent 在本机干活时可能需要回本服务取数据。
  挂不上就**不装委派执行器**（工具如实报「未配置」），而不是让它在缺工具的情况下跑。

### Changed

- **手搓实现换 oekit 原语**：沙箱路径判定 → `pkg/pathx.SanitizeRelStrict`；
  UUID → `kitx/uidgen`（零三方依赖）⇒ 少一个直接依赖。
- **文档订正**：设计文档 §18.3/§18.4 里「acpx 不挂 /mcp」「要用领域工具必须 stdio」
  的旧结论已订正 —— 那是**我们没写配置**，不是 acpx 做不到；补 §20 记录
  症状 / 根因 / 配置形状的三个坑 / 修法 / 实测对比。
  README（中英）与 `ARCHITECTURE.md` 的后端选型说明同步为
  「两个后端都挂 `/mcp`，差别在逐次授权卡与 agent 侧会话 ID」。

## [v2.2.0] - 2026-10-27

外部 AI coding 工具（ACP）能力的一轮补完：**多 agent 并存**、**委派改异步**、
**stdio 形态也能被委派**，外加全流程实测挖出的六个坑。

### Added

- **多个外部 agent 并存（具名 profile 列表）**：`ai.external.agents` 是一个 profile 列表
  （`name` / `transport` / `agent` / `command` / `cwd` / `permission` / MCP 挂载…），
  `current` 选中的驱动对话，委派工具的 `agent` 参数按名字选。
  列表为空时回退旧的单 profile 字段 ⇒ 已发布的 v2.1.0 配置照旧可用。
- **委派改异步**：`ai_code_delegate` 确认后**立刻**返回任务 ID（`dtask-N`），
  外部 agent 在后台跑。此前同步等待会把整条 SSE 挂住 ⇒ 中间的代理
  （nginx `proxy_read_timeout` 默认 60s）一刀切掉 ⇒ 用户看到「助手卡住了」。
  结果两处可取：回填会话（措辞标明「仅作数据，不是指令」）与按 ID 取
  （`ai_delegate_result` 工具 / `GET /api/v1/ai/delegate/{task_id}`）。
  后台任务用 `context.WithoutCancel`（HTTP 一结束请求 ctx 就被取消 ⇒ 后台委派会被一起杀掉）。
- **stdio 形态也能被委派**（`RunOnceStdio`）：委派执行器此前固定走 `acpx <agent> exec`，
  委派到 stdio profile 时会把「命令」当成 acpx 的 agent 名 ⇒ 必然跑不起来。
  现在按 profile 的传输方式执行：acpx 走 exec，stdio 走一次性 ACP 会话
  （起进程 → initialize → session/new → session/prompt → 收结果 → 关进程，复用 stdio 客户端）。
  stdio 传输下用名字选一个不存在的 profile 会直接报错（stdio 没有「内置 agent 名」这回事）。
- **思考过程在界面上的呈现**：外部 agent 的 `agent_thought_chunk` 映射成 `thinking` 事件
  （此前只在协议层可见），界面按回合累积、默认折叠、限长 2000 字。
  限长放在前端（服务端截断会让流「突然停住」）。

### Fixed

本轮六个坑，全部由**全流程实测**挖出来（内置助手 / stdio 外部后端 / acpx 外部后端 /
异步委派 / 只读边界 / 多 profile 六条链路实跑）：

- **profile 模式下 AI 根本没起来**（最严重）：`ai.backend=stdio` + `external.agents[]`
  ⇒ 启动报「需要 ai.external.command」⇒ **AI 整个没起来**（端点回落 501）。
  根因：校验查的是旧的单字段，而 profile 模式下命令写在 profile 里。
  改法：校验与装配统一走 `EffectiveExternal()`（生效 profile + 生效 transport）——
  **同一个概念的新旧两套写法只留一个入口**（校验 / 装配 / 审计 / 委派 / 治理闸门五处共用）。
- **yaml 里配的 profile 被静默丢弃**：`MergeSettings` 逐字段列举，漏了 `Agents` / `Current`
  ⇒ 只有走设置文件才生效。改法：补齐 + 加**反射测试**（逐字段比对，
  以后新增字段当场被拦）。
- **界面表单读不到外部配置**：`ExternalConfig` 缺 JSON 键名定义 ⇒ 回显出来的是 Go 的
  大写键（`Agent` / `Agents`）⇒ 前端读不到、界面全空。
  改法：补齐键名 + 测试钉死 snake_case。
- **改 profile 授权档绕过 admin 闸门**：治理闸门只比单字段的授权档 ⇒ profile 列表模式下
  改某个 profile 的授权档（approve_reads → approve_all）不被当成治理动作。
  授权档决定外部 agent 能读什么 / 写什么 ⇒ 这是安全边界。
  改法：逐 profile 比对（空值与默认档等价）。
- **profile 的传输方式靠猜**：老写法只填 `agent` 或只填 `command` 时无从判断该走哪条通道；
  主后端是 internal 时 profile 只服务委派、不能跟着主后端一起变。
  改法：显式 transport > 有 command ⇒ stdio > 有 agent 名 ⇒ acpx > 跟随 `ai.backend`。
- **委派到 stdio profile 必然失败**：见上面 Added 的对应条目。

另修复（自审轮）：

- **外部 agent 的边界洞**：沙箱软链逃逸、terminal 输出无上限、acpx 会话磁盘泄漏、
  同一会话并发回合互相截胡（第一轮事件流到第二轮通道）。
- **全仓 lint 清零**：CI 的质量闸门已连续 8 个版本失效（lint 从未真正跑起来）。
  `golangci-lint run` 从 34 条降到 0 条，CI 首次全绿。

### Changed

- **文档**：README（中英）/ `ARCHITECTURE.md` §7.5.1 / `docs/design/acp-external-agent-design.md`
  补齐本轮能力与取舍；`skills/gitferry-histsync/SKILL.md` 里「委派会占住确认请求」
  的旧描述与异步委派的实际行为不符，已按现状改写。
- **通道能力差异写进文档**（选型依据）：只有 `stdio` 会把本服务的 `/mcp` 挂给外部 agent
  ⇒ 只有 `stdio` 能让外部 agent 把领域工具调回本服务；`acpx` 不挂
  （外部 agent 在那条通道上会自己满环境找工具）、非交互（`ask` 授权档直接被拒）、
  且拿不到 agent 侧的会话 ID（会话在 acpx 自己的存储里）。
- **工具数 42 → 43**（新增 `ai_delegate_result`：按任务 ID 取回委派结果）。

## [v2.1.0] - 2026-10-26

外部 AI coding 工具接入（ACP）。**AI 助手的执行后端现在可插拔**：
除内置 eino 助手外，可把整段对话交给外部成熟的 AI coding 工具
（MiniMax Code / Claude Code / Codex / Gemini CLI / OpenCode…），
经 [ACP](https://agentclientprotocol.com)（Agent Client Protocol，JSON-RPC over stdio）驱动。

### Added

- **执行后端可插拔**：`ai.backend` 可选 `internal`（内置 eino）/ `acpx`（经 acpx CLI 驱动，
  一个命令面对 20+ 个 agent）/ `stdio`（原生 ACP，自己当 client）。
  抽象落在**回合层**（一轮对话进、一串与后端无关的事件出）而不是模型层 ——
  ACP 不是「另一个模型端点」，它是「编辑器 ↔ coding agent」的协议，agent 是**子进程**，
  硬塞进 `model.BaseModel` 只会得到一堆假动作。
  `internal/agent/engine/` 是中立契约，`internal/agent/engine/acp/` 是外部后端。
  **对外零破坏**：`/api/v1/ai/chat` 的 SSE 契约与 `Runner` 的公开方法不变。
- **领域工具不重写**：外部 agent 经 MCP 客户端挂回本服务自己的 `/mcp`，
  42 个工具（同步 / 镜像 / 运维 / 历史同步）直接可用。挂载清单支持**多源**
  （`ai.external.mcp_servers`），认证密钥只走环境变量（不进 yaml、不进设置文件）。
- **授权卡**：外部 agent 的 `session/request_permission` 接进界面的确认卡
  （复用危险工具那套交互，新增 `permission` 事件与 `permission_call` 回传）。
- **委派工具 `ai_code_delegate`**（危险）：内置助手可以把一段编码任务派给外部 agent，
  结果回到助手上下文。走既有确认链路（外部 agent 是本机子进程 ⇒ 必须让人知道）。
- **按 key 分角色的鉴权**：原先只有**单一全局 key + 单一全局角色**，给不出「只读 key」；
  补 `SetScopedAPIKey` / `ResolveAPIKeyRole`，外部 agent 的 MCP 专用 key 固定只读
  （MCP 路径上危险工具**不弹确认**，给 admin key 等于交出「无需确认的写操作」）。
- **外部 agent 会话 ID 透出**：事件流新增 `agent_session` 事件（带 `agent_session_id`
  + `agent_label`）。外部 agent 与本服务可能**共用同一个会话库**
  （如 MiniMax Code 的 CLI 与桌面版：ACP 建的会话落在 `~/.minimax/v2/sessions/…`，
  与桌面版同格式）⇒ 透出来，用户才能在那个界面上找到这次会话、接着聊。
- **AI 配置的治理闸门**：改 AI 配置要写角色；**换后端 / 改外部 agent 授权档仅 admin**
  （换后端 = 让另一个程序接管本机上的 AI 助手），且写操作日志。

### Fixed

- **外部 agent 子进程继承了服务密钥**（安全）：Go 的 `exec.Cmd.Env` 为 nil ⇒
  子进程继承父进程环境 ⇒ agent 能读到 `ENCRYPTION_KEY`（**凭据加密主密钥**，
  可解密库里所有 git 凭据）与 admin 主 key ⇒ **绕开 MCP 的只读边界**直调写接口。
  改法：给外部 agent 进程**净化环境**（精确摘 `ENCRYPTION_KEY` 与
  `GIT_SYNC_*` / `GITFERRY_*` 前缀；agent 自己的 provider 鉴权要留，不能一刀切清空）。
  实测复验：修完 agent 打印 env ⇒ 一无所获。
- **AI 配置页显示的不是生效配置**：`GET /api/v1/ai/config` 直接回设置文件，
  而真正生效的是「yaml 默认 + 设置文件覆盖」⇒ **用 yaml 部署时界面是空表单**，
  用户得把配置全部重敲一遍才能保存。改法：handler 侧注入 yaml 段并合并后回显；
  「有没有变更」的判定也改用合并后的值。
- **yaml 的 `mcp_enabled: true` 永远不生效**：布尔字段分不清「未设置」与「显式 false」，
  合并时被当成 false 覆盖掉。改法：设置文件还没写过之前，yaml 的布尔值说了算。
- **换后端热重建不关旧后端**：外部后端每会话一个**长驻 agent 子进程**，
  不关旧的就攒孤儿进程；会话回收 goroutine 也每次重建多留一个。
  改法：补 `Runner.Close()`（关后端 + 停回收 goroutine），重建与进程退出路径都调。
- **acpx 通道的 `ask` 档名不副实**：acpx 是**非交互**的（它的 stdin/stdout 被我们占着）
  ⇒ 弹不出授权卡，写操作按拒绝处理 ⇒ 配置写「每次都问」而实际是「拒绝」。
  改法：配置校验阶段直接拒绝该组合并给出出路；参数映射改成显式给 `--approve-reads`。
  同类的还有「存下一份后端起不来的配置」——校验放在**保存时**，不是只在启动时。

### Changed

- **文档重组**：历史过程文档（`docs/history`、`docs/superpowers`）与静态 HTML 稿已从
  仓库清出，规格文档移入 `docs/design/` 并加索引；README / ARCHITECTURE / CONTRIBUTING
  按「描述当前架构」重写。`docs/design/acp-external-agent-design.md` 是本特性的设计文档。
  （这属于文档侧整理，接口与行为均无变化 ⇒ 发 minor 版本。）
- **工具数 41 → 42**（新增 `ai_code_delegate`）。

## [v2.0.1] - 2026-10-26

### Fixed

- **CLI 的 401 提示不指路**：密钥存放在 macOS Keychain 时，配置文件里的
  `api_key` 是空的 ⇒ 非交互环境（keychain 锁住 / `security` 不可用 / 换了用户）
  取不到 ⇒ CLI 不带任何认证发请求 ⇒ 服务端 401，而提示写的是通用的
  「请设置 GITFERRY_TOKEN 或运行 gitferry auth login」
  ⇒ 用户以为自己没配过，反复重新设置 token，而真正的解法是
  「这个环境取不到 Keychain 里的密钥」。
  改法：本机**确实没有**可用密钥时，提示改成「本机没有可用的 API Key
  （可能已存在 macOS Keychain，但当前环境取不到）」；本机**有**密钥时
  保留原提示（那就是密钥不对 / 权限不足）。
- **密钥来源不可见**：密钥在 Keychain 模式下，从配置文件上根本看不出
  「本机到底有没有密钥」。改法：配置增加运行时字段 `KeySource`
  （`env:GITFERRY_TOKEN` / `env:GITFERRY_API_KEY` / `file` / `keychain` / `none`，
  **不落盘**）+ `KeychainErr`（取不到的具体原因），`auth status` 一并报出。
- **Keychain 写失败会把旧密钥删掉**：`saveSecret` 是「先删旧项、再 add」，
  而 `security add-generic-password` 在条目已存在时会失败 ⇒ 必须先删；
  但 add 失败（keychain 锁住 / `security` 不可用）时**旧密钥就没了**
  ⇒ 用户的凭据被静默删掉，而调用方以为写成功了。
  改法：删之前把旧值读出来，add 失败就写回去。
  同时把取密钥的失败分成两类：条目不存在（`security` 返回码 44）与
  「取不到」（其它错误）—— 两者的用户建议完全不同。

### Added

- **发版提交助手**（`scripts/release-commit.sh`）：多个 agent 会话共享同一个
  工作树 ⇒ 共享**同一个 `.git/index`** ⇒ `git commit`（不带 `--only`）
  提交的是**整个暂存区** ⇒ 别人 `git add` 的文件会被一起带进你的提交
  （本项目真实发生过：另一会话的 `docs/design/*.md` 进了发版提交，
  被我的 tag 一起发布出去）。
  改法：脚本用 `git commit --only <路径>`，只提交给定路径、忽略暂存区里的
  其它内容；提交后自检"本次提交的文件是否全在给定路径内"，发现越界文件就
  **自动撤销**这次提交（`git reset --soft HEAD~1`，不丢内容）并非 0 退出。
  `--check <路径>...` 模式可单独检查当前 HEAD。

## [v2.0.0] - 2026-10-26

GitFerry **v2**：契约语义摆正。

v1.36.0 ~ v1.38.0 三轮里的改动**包含破坏性契约变更**（查不到的 run 从
「200 + 空列表」变成「404」、`auth status` 退出码从 0 变 1、`--csv` 导出从
「CSV + JSON 信封」变成「纯 CSV」、MCP 工具的字段语义变更），却连发三个
minor ⇒ **版本号在说谎**：下游会按「兼容」假设来升级，升级时踩坑。
本版把这条线整体标为 **v2**，并给出逐条迁移指引；同时加一道机械门禁，
让「破坏性变更发成 minor」以后**不可能再发生**。

### 迁移指引

**从 v1.35.x 升到 v2.0.0**。下面每一条都是**调用方可见**的变更。

#### CLI

| 命令 | v1 行为 | v2 行为 | 调用方要改什么 |
|---|---|---|---|
| `repo +create` | 发 `platform` / `clone_url` / `key`，服务端全部丢弃 ⇒ **永远 400** | 发 `name` + `remote_url`；平台走 query `platform_id` | 手拼请求体的脚本要改成新字段（CLI 已自动适配） |
| `ops +drift --task` | 参数名与服务端不符 ⇒ 绑定失败被忽略 ⇒ **静默扫描全部任务** | 改发 `task_keys`；绑定失败即 **400** | 依赖「默认全量扫描」的调用要显式不传 `--task` |
| `task +list --all` | 只读第一页，且 `meta.total_count` 是**假的** | 发 `page_size`、按服务端 meta 结束、返回**真实总数** | 别再按 `meta.total_count` 推算条数 |
| `task +batch-run` | 部分失败时**退出码仍是 0** | 部分失败 ⇒ 退出码非 0 | 脚本必须检查退出码 |
| `ops +audit-report --csv` | stdout = CSV **+ JSON 信封** ⇒ 导出文件是坏的 | stdout **只有 CSV** | 解析器不用再剥 JSON 尾巴 |
| `auth status` | 服务端 401/5xx/连不上都报 `ok=true`、退出码 0 | 如实报 `ok=false` + 退出码 1 | 脚本必须检查退出码，不要只看 `ok` |
| `task +list --status` | 服务端**没有**这个字段 ⇒ 静默返回全量 | **真的**按 `last_status` 过滤，`total` 是过滤后的条数 | 依赖旧行为（拿全量）的改成不带 `--status` |
| `history +detail --run-id` | 服务端**没有** run_id 过滤 ⇒ 翻页找；找不到返回空列表 200 | **精确查**那一条；找不到 ⇒ **404**；`run_id` 可单独给（不再强制 `--task`） | 加 404 分支；去掉「找不到就是空列表」的假设 |

#### HTTP API

| 接口 | v1 行为 | v2 行为 |
|---|---|---|
| `GET /api/v1/sync/history?run_id=` | **忽略** `run_id`，返回该任务的全部历史 | 精确返回那一条；查不到 ⇒ **404**；给了 `run_id` 时不再强制 `task_key` |
| `GET /api/v1/sync/tasks?status=` | **忽略** `status` | 真的按 `last_status` 过滤，`total` 是过滤后的条数 |
| `POST /api/v1/ops/rebuild` | **受理前**先删工作目录（会抢正在跑的同步的目录） | 由 core 在**拿到执行权后**删（抢不到执行权就一步都不动）；响应 `message` 变更 |
| `GET /api/v1/ops/drift` | 绑定错误被丢掉 ⇒ 静默退化成全量扫描 | 绑定失败即 **400** |

#### MCP 工具

| 工具 | v1 行为 | v2 行为 |
|---|---|---|
| `list_histsync_runs` | 硬编码 50 条、不可调，而 `total` 却是**真实总数**（两个数字对不上） | 新增 `page` / `limit`（默认 20、上限 100）；输出新增 `page` / `limit` / `returned` / `truncated` / `notice` |
| `get_ops_todo` | 三路子查询失败被**静默吞掉**；`total` 是**截断后**的长度 | 新增 `partial` / `failures` / `warning` / `returned` / `truncated` / `notice` / `message`；`total` 改为**截断前**的真实条数 |
| `get_sync_health` | 历史查询失败被说成「从未执行」 | 新增顶层 `failures` + 任务级 `history_error` |
| `deep_analyze` | 同上 | 新增顶层 `query_failures` + 任务级 `history_error` |
| `diagnose_run` | 「查不到」被说成「未找到」 | 新增 `query_failed` 标记，两者区分开 |

#### core（影响壳层与下游集成）

- **rebuild 语义下沉到 core**：清工作目录的时机从「受理前」变成「拿到该任务
  的执行权之后」。依赖「rebuild 一调用目录就没了」的集成要改成读执行历史确认。

### 破坏性变更

1. `GET /api/v1/sync/history?run_id=` 查不到 ⇒ **404**（原先 200 + 空列表）。
2. `GET /api/v1/sync/history` 不再强制 `task_key`（给了 `run_id` 时）。
3. `GET /api/v1/sync/tasks?status=` 开始**真的过滤**（原先静默返回全量）。
4. `GET /api/v1/ops/drift` 绑定失败 ⇒ **400**（原先静默全量扫描）。
5. `POST /api/v1/ops/rebuild` 不再在受理前删工作目录；响应 `message` 变更。
6. CLI `ops +audit-report --csv` 的 stdout **只剩 CSV**（原先 CSV + JSON 信封）。
7. CLI `auth status` 失败时**退出码 1**、`ok=false`（原先恒 0 / 恒 true）。
8. CLI `task +batch-run` 部分失败时**退出码非 0**（原先恒 0）。
9. CLI `repo +create` 的请求体字段改为 `name` + `remote_url`，平台走 query
   `platform_id`（原先的字段名被服务端丢弃 ⇒ 命令恒 400）。
10. CLI `ops +drift --task` 改发 `task_keys`；绑定失败即 400（原先静默全量扫描）。
11. CLI `task +list --all` 返回**真实总数**（原先 `meta.total_count` 是假的）。
12. MCP `get_ops_todo` 的 `total` 语义变更（截断前 vs 截断后），并新增
    `partial` / `failures` / `warning` 等字段。
13. MCP `list_histsync_runs` 新增 `page` / `limit` 入参（原先硬编码 50 不可调）。
14. MCP `get_sync_health` / `deep_analyze` / `diagnose_run` 的失败表达方式变更
    （新增 `failures` / `query_failures` / `history_error` / `query_failed`）。

### Added

- **发版纪律门禁**（`internal/release`）：把「破坏性契约变更必须走 major」
  变成**机械检查**，而不是靠人记 ——
  - 破坏性变更必须显式标记（`### 破坏性变更`），标记了就必须是 major；
  - major 版本必须给出迁移指引（`### 迁移指引`）；
  - 公开契约（OpenAPI）删掉 path / 请求参数 ⇒ 必须 major。
  这条门禁正是本版存在的原因：v1.36.0~v1.38.0 连发三个 minor 就是漏了它。

## [v1.38.0] - 2026-10-26

### Fixed

同包内另外三处**同一形状**的错误吞掉（一并修掉）：

- **MCP `get_sync_health` / `deep_analyze` 把历史查询失败当成「从未执行」**：
  以前 `if herr == nil && len(runs) > 0` 的 else 分支同时覆盖「没有历史」与
  「查询失败」⇒ 报告写「从未执行」，真相是「查不到」⇒ 运维会去查一个
  根本不存在的配置问题。
  修法：分数仍按「没有历史」**保守**打分（fail-closed：宁可低不要高），
  但把失败本身报出来 —— 顶层 `failures`（`get_sync_health`）/
  `query_failures`（`deep_analyze`）+ 任务级 `history_error`。
- **MCP `diagnose_run` 把「查不到」说成「未找到」**：`findRunView` 对历史
  查询失败是 `continue` 直接跳过 ⇒ 搜完全部任务找不到就报 `run N not found`
  ⇒ 运维以为记录被删了，真相是「查不到」⇒ **无法断言它不存在**。
  修法：累计查询失败，搜完仍找不到时用 `historyQueryError` 明确报
  「无法断言它不存在」；输出加 `query_failed` 标记，与「确实不存在」区分开。
- **MCP `get_ops_todo` 继续吃健康维度的内部失败**：健康段逐任务的查询失败
  现在会一并传到待办队列的 `failures`（`source=health/history`）；
  且这类任务的「健康分偏低」会说明是**保守打分**、不是真实健康度。

## [v1.37.0] - 2026-10-26

### Fixed

- **MCP `get_ops_todo` 的三路子查询失败全部被静默吞掉**（报告与事实相反）：
  健康 / 孤儿两路走子工具，子工具失败时返回的是 `{"error": "..."}` 的 JSON 而
  **Go error 恒为 nil** ⇒ 以前 `json.Unmarshal` 那个负载不报错、Items 为空
  ⇒ **子系统一挂，这一整类待办无声消失**，调用方看到的是「没有待办」；
  RPO 一路连 Go error 都被直接丢掉。
  修法：新增 `parseToolPayload`（错误信封与解析失败都变成 Go error），
  三路失败全部记进 `failures`，输出带 `partial` + `warning`；一条都没查到时
  额外给 `message`，避免「空列表」被当成「一切正常」。
  顺带把截断也说出口：`total` 改为**截断前**的真实条数，并给出
  `returned` / `truncated`（以前 `total` 报的是截断后的长度）。
- **MCP `list_histsync_runs` 硬编码 50 条**：入参只有 `channel_id`
  ⇒ 模型想看更多也拿不到，而输出里的 `total` 却是**真实总数**
  ⇒ 两个数字对不上，模型无从判断自己看到的是不是全部
  （同一个坑在 `list_histsync_channels` 上已修过一遍，这里是漏网的同一个）。
  修法：给 `page` / `limit` 入参（默认 20、上限 100，与
  `list_histsync_channels` 一致），真分页取数，并给出 `returned` / `truncated` /
  `notice`。

## [v1.36.0] - 2026-10-26

### Changed

- **core pin → v0.30.0**：
  - **rebuild 语义下沉到 core**：清工作目录必须在**拿到执行权之后**做。
    ① 壳层 `POST /api/v1/ops/rebuild` 原来「先删目录、再受理」—— 任务正在跑时
    目录已经被删（跑着的工作树被删 ⇒ 那次同步必炸），受理又因 `ErrTaskRunning`
    失败 ⇒ 净效果是「把活着的同步搞死，重建还没排上」；② MCP 的 `rebuild_task`
    描述写「清空任务工作目录并全量重建」，实现却只是重新触发一次同步 ⇒
    **workdir 损坏 / 浅克隆永远修不了**（工具描述与行为相反）。
    修法：core 在拿到执行权后清目录，抢不到执行权就一步都不动；壳层不再自己删。
  - **卡在进行中的镜像执行永不回收 ⇒ 通道永久占用**：僵尸 run（进程被 SIGKILL /
    崩溃 / OOM 杀掉）状态永远停在 `pending|running` ⇒ 该通道**永久**报
    「该通道有执行中的任务」，再也无法触发任何发布 / 校验。
  - **版本矩阵取最旧一次执行 / tag 用字符串序 / verify 列永远 unverified** ——
    三处都让矩阵**报告与事实相反**。
  - **执行历史按保留期清理**：`CleanupOldData` 此前从未被调用 ⇒ 三张表无限增长。
- **SDK pin → v0.99.7**：跨 host 重定向**泄漏 URL query 里的凭据**
  （go-gitee 把令牌放 `access_token` 且每跳都重加 ⇒ 平台 302 到 CDN 时令牌
  发给第三方主机）；Gitee 归档下载 100% 失败；Gitee / GitCode 文件内容返回
  base64 串；`ListCRs` 零值 State 被当成「只看 open」。

### Fixed

- **CLI 的请求形状与服务端契约对齐**（hertz 静默忽略未知 query 参数 ⇒
  契约漂移的表现是「命令存在、参数也传了，但永远不生效」）：
  - `repo +create` 发的是 `platform` / `clone_url` / `key`，而服务端的
    CreateRepoReq 只有 **name + remote_url**（平台走 query 的 **platform_id**）
    ⇒ 三个字段全被丢弃 ⇒ 400 ⇒ **这个命令从来没能成功过**。
  - `ops +drift --task` 发的是 `task_key`，服务端要的是 `task_keys`
    ⇒ 绑定失败 ⇒ **静默退化成扫描全部任务**，调用方以为做了筛选。
  - `PaginateAll` 只读第一页且**谎报总数** ⇒ `--all` 拿到的不是全量，
    而信封里的总数看着又是对的。
  - `task +batch-run` 部分失败时**退出码仍是 0** ⇒ 脚本按退出码判断会以为全成功。
- **审计导出的 stdout 被 Envelope 污染**：`ops +audit-report --csv` 先打印 CSV，
  再让框架把 Envelope JSON 也打到 stdout ⇒ `> out.csv` 得到的文件尾部多一段
  JSON ⇒ 任何 CSV 解析器都读不了这个导出文件。
  改法：CSV 模式下命令自己输出后返回 nil Envelope，框架不再打信封。
- **`auth status` 对 4xx / 5xx / 连不上仍报 ok**：以前无论服务端返回什么都
  `output.Success(...)` ⇒ 信封里**永远 ok=true** ⇒ 脚本按 ok 判断就会把
  「密钥无效 / 服务不可达」当成「一切正常」。改法：失败如实报失败并非 0 退出。
- **漂移检测的绑定错误被丢弃**：`DetectDrift` 原来 `_ = c.BindAndValidate(&req)`
  ⇒ 绑定失败被当成「没传过滤条件」⇒ **静默退化成扫描全部任务**。
  改法：绑定失败即 400。

### Added

- **任务列表支持 `status` 过滤、执行历史支持按 `run_id` 精确查**：CLI 的
  `task +list --status` / `history +detail --run-id` 一直在发这两个参数，
  但契约里没有 ⇒ **用户以为筛过了、拿到的却是全量**（`history +detail`
  翻不到就是空列表）。IDL / model / handler 全链对齐，查不到的 run_id 报 404。

## [v1.35.0] - 2026-10-25

### Changed

- **core pin → v0.29.0**：`GetBranchSyncInfo` 在两侧历史无关（目标仓被强推改写过）时
  gogit 后端返回 `(0,0)` ⇒ 漂移检测**误判成「已同步」** ⇒ 目标仓被改写历史后漂移
  再也检测不到。native 走 `git rev-list --left-right --count A...B` 会给出两侧各自
  的提交数 ⇒ 补齐同一语义（这条路径在镜像漂移检测里被用到）。
- **SDK pin → v0.99.6**：同上修复 + v0.99.5 的日志脱敏加固（`redactURL` 补
  userinfo；新增 `redactPath` 接入三处日志与一处错误构造 —— `Request.Path` 可以
  自带 query，里面的 token 会随日志与错误信息外泄）。

## [v1.34.0] - 2026-10-25

### Fixed

- **Gotify 令牌落进服务端日志**：token 拼在 **query** 里 ⇒ 请求失败时
  `http.Client` 返回 `*url.Error`，其 `Error()` 含**完整 URL** ⇒ 错误进日志 ⇒
  令牌落到日志采集 / 告警平台 / 备份。healthchecks.io 的 ping UUID 在**路径**里
  （同样是秘密），同样会被打出去。
  修法：`redactSecretURL`（秘密类查询参数替换成 `REDACTED`、**路径整体掩掉**，
  保留 host 以便排障）+ `redactError`（把 `*url.Error` 里的 URL 换成脱敏版，
  保留错误类型与包装）+ 通知与心跳的日志点统一走脱敏。
- **AI 接口把 API key 放在 URL query**：`GET /api/v1/ai/models?…&api_key=…`
  ⇒ 密钥进浏览器历史 / Referer / 反向代理 access log。
  ⇒ 改 **POST + body**（key 不再出现在 URL 里）。路由、handler、apidoc 的路由表、
  前端调用同步改；OpenAPI 已重新生成（契约门禁盯着路由与 OpenAPI 是否一致）。

### Changed

- **core pin → v0.28.0**：webhook 提交被静默丢掉的两条路径（任务繁忙时的待补跑、
  卡在 `processing` 的事件回收）+ 抬 SDK 至 v0.99.4。
- **SDK pin → v0.99.4**：跨 host 重定向不再带走平台令牌（P0）+ gogit 后端六处
  会破坏仓库的差异。

## [v1.33.0] - 2026-10-25

### Fixed

- **已发布的 OpenAPI 契约在说谎**：`apidoc` 生成器不认识 thrift 的 `optional`
  关键字 ⇒ "optional bool enabled" 被解析成 typ="optional" ⇒ 类型落到未知分支
  ⇒ **OpenAPI 里所有 optional 字段都成了 `"type":"string"`**
  （`UpdateTaskReq` 的 enabled / git_force / git_lfs / git_bundle / sync_wiki /
  submodules / git_push_prune，以及 Create/UpdateRuleReq 的 enabled）
  ⇒ **按已发布契约生成的客户端会把布尔字段发成字符串**。
  修法：解析器先剥掉 `optional` 再取类型；补单测钉住。
  （双向契约门禁没抓到：IDL 与 openapi.json 同源，错的也一致。）
- **core pin → v0.27.0**：两处 P0（入站 webhook 第 2 条起被静默丢弃、历史同步
  目标树基线永不前进）、五处 P1（预检与推送闸门口径相反、显式 `enabled=false`
  被默认值吞掉、局部更新静默改规则、触发失败仍标 processed）、三处 P2（推送目标
  地址回显凭据、同名目标凭据串号、演练链检不出截断与坏行）。
- **SDK pin → v0.99.3**：入站 webhook 的事件 ID 永不为空 + 优先用平台投递 ID
  （GitHub `X-GitHub-Delivery` 等）⇒ GitHub 上配了入站同步不再「第一条之后就失效」。

### Changed

- **webhook 规则字段改 optional**（**破坏性**）：`CreateRuleReq.min_interval/enabled`、
  `UpdateRuleReq.min_interval/enabled/branch_pattern/description` 改成 optional
  （IDL、手维护 thrift model、handler 同步）。
  理由：**前端在规则列表里切换启用开关提交的是 `{id, enabled}`**，而旧 core
  `UpdateRule` 无条件回写 enabled/branchPattern/description ⇒ 每次切开关都会
  把分支模式清空（规则从「只跟 main」悄悄放宽成「任何分支都触发」）、把描述清空、
  把 enabled 写成 false（规则被静默禁用 ⇒ 之后所有 webhook 都不再触发同步）。
  ⇒ 改指针：nil = 不修改。
- **健康评分口径对齐**：`internal/agent/tools/{ops,analyze}.go` 改用 core 的
  `health.SnapshotsFromRuns`（唯一实现）⇒ 此前壳层另写一份事实采集，漏了
  `StepFails`/`RetryTotal`/`ErrorType` ⇒ **同一个任务，AI 看到的健康分比 API
  给运维看的乐观**。

## [v1.32.0] - 2026-10-25

### Changed

- **core pin 抬到 v0.26.1**（零 replace）：删除旧规则引擎（健康评分只留
  一套维度实现，`error_free` 信号由 reliability 维度接管）+ 抬 SDK 地板。
- **SDK pin 抬到 v0.99.2**：本仓用 `GetCommitsBetween` 算「目标侧独有提交」
  展示清单，而 gogit 后端的旧实现（SDK v0.99.0）**在分叉场景会把共同祖先
  也收进清单**（v0.99.1 已修）。⇒ 壳层与 core 此前跑的 SDK 版本不一致
  （依赖图取最高版本 ⇒ 壳层已是新版、core 单独构建仍是旧版）。

### Removed

- **删除前端死别名 `SystemStatusResp`**（`frontend/src/api/index.ts`）：
  注释声称"兼容旧导入"，实际全仓**无任何引用** ⇒ 纯死兼容导出。

### Fixed

- 健康评分测试改用维度引擎（旧规则引擎已删，`health.DefaultConfig()` 不复存在）。

### Changed（健康面板文案）

- 健康 `issues` 数组不再有旧规则引擎的措辞，改为维度格式（`<维度>: <结论>`）；
  **分数与 level 不变**。

## [v1.31.0] - 2026-10-25

### Removed（**破坏性**）

删除剩余三处**仍在生效**的旧版本兼容（core v0.25.0）。删除前核过现网数据：
300+ 仓库的 per-repo `access_token` **全为空**、9 个任务的 `keep_divergent`
**全为 1**（等价 block）且 `force_push_policy` 为空、配置**无 `backup_s3` 段**
⇒ 三处对现网均为**零行为变化**。

- **per-repo `accessToken`**：IDL（`idl/repo.thrift` 的 Create/Update 各一处）、
  手维护的 thrift model、handler 映射、前端类型与仓库表单、OpenAPI。
  凭据口径**只留平台一处**（含 GitHub App installation token）。
- **`keepDivergent`**：IDL 三处、手维护的 thrift model（三个结构体）、
  handler 映射、前端类型与两个任务页面、OpenAPI、README 与 skill 文档。
  它是 `force_push_policy` 的**旧拼写**；两套并存 ⇒ 前端一度**按 `keep_divergent`
  展示策略**，与实际生效的 `force_push_policy` 不一致。现在只认
  `force_push_policy`，空值按 `block`（最安全）回填。
- **legacy `backup_s3` 配置项**（`conf/config.example.yaml`），
  只留 `backup_destinations`（多目的地扇出）。

### Changed

- **core pin 抬到 v0.25.0**（零 replace）：策略解析收敛为唯一实现
  `model.ResolveForcePushPolicy`（`service` 与 `executor` 此前各有一份平行实现），
  空值 ⇒ `block`、非法值同样按 `block`（不静默放行）。
- `internal/agent/tools/*` 同步去掉 health 事实里的 `keep_divergent`
  （该字段引用 core model，不清理则编译不过）。

## [v1.30.0] - 2026-10-25

### Removed（**破坏性**）

- **删除所有旧版本兼容层**（core v0.24.0）：`gitPrune` 字段（旧版本兼容用的
  死开关，v0.23.0 起已不生效）、旧哈希算法（旧记录哪怕把 `kind` /
  `pushed_run_id` 改成任何值，链都照样报「完好」）、旧审批文件导入、旧模板文件迁移。
  壳层同步清理：IDL、手维护的 thrift model、handler 映射、CLI 的 `--git-prune`
  开关、前端（类型 / 表单 / 复选框）、文档与 skill 文档。

## [v1.29.0] - 2026-10-25

### Changed

- core pin 抬到 **v0.23.0**（零 replace）：五条修复 ——
  **源端 fetch 恒剪枝**（此前跟 `task.GitPrune` 走，而 gogit 后端（生产用的）
  只在 `Prune=true` 时才剪 ⇒ 同一份配置两后端行为不同：源端删了分支 ⇒
  本地留着陈旧跟踪 ref ⇒ **源端已删的分支又被推回目标**，且
  **镜像删除（`git_push_prune`）从来没触发过**）、
  **多分支的镜像删除落地**（以前只告警不实现）、**wiki 改逐分支推**
  （整批推时一个分支非快进会拖垮全部）、align 跳过 `origin/HEAD` 伪 ref、
  漂移检测的 clone 套用 partial-clone。

### Fixed

- IDL 的 `gitPrune` 标注为**不再生效**（源端 fetch 恒剪枝是镜像语义要求），
  保留字段只为兼容旧客户端与旧配置。

## [v1.28.9] - 2026-10-25

### Changed

- core pin 抬到 **v0.22.0**、SDK pin 抬到 **v0.99.0**（零 replace）：一轮**自查复查**
  揪出上一轮改动里的 1 个 P0 + 3 个 P1，另修完此前清单剩下的九条 ——
  **P0 多分支路径的 fail-closed 是死代码**（目标端状态未知 ⇒ 放行覆盖目标侧独有提交）、
  **P1 多分支 allow 策略回归**（显式允许覆盖的任务反被拒）、
  **P1 多分支的人工放行永远不被消费**（"一次性"退化成常驻后门）、
  增量 `since` 漏老条目、PR 剔除仍占 issue 预算、manifest 写失败仍报成功、
  DR 演练不解密、bundle 文件名不校验、SampleMetadataVerify 无快照报"通过"、
  UpdateTask 三个字段无法清空、回灌历史不清理、RevParse 失败被当成"未绑定"、
  wiki 关键词误判、rotateBundles 吞错、漂移仓库不自愈、漂移 glob 必失败。

### Fixed

- **契约门禁的路由源改成 glob**：此前写死两个文件 ⇒ 新增第三个注册文件时，
  那个文件里的接口**两个方向都测不到** ⇒ 门禁静默失去覆盖。删掉死代码。
- **update 的 `force_push_policy` / `include_branches` / `exclude_ref_patterns`
  改为传指针**（nil=不修改、**空串=清空**），与 IDL 的 `optional string` 对齐 ——
  之前用 `Deref` 成普通 string，空串与"没传"不可区分 ⇒ 一旦设过就再也改不回去。

## [v1.28.8] - 2026-10-25

### Changed

- core pin 抬到 **v0.21.0**、SDK pin 抬到 **v0.98.0**（零 replace）：九条修复 ——
  **wiki 同步推的是旧内容 + 无条件强推 + 非默认分支不同步**（P0）、
  **漂移检测比错两侧且不带凭证**（私有仓永远检不出漂移）、
  `exclude_ref_patterns` 被 align 重新造回、`glob` 传给 `git bundle create`
  导致多分支任务永远产不出冷备、`backup_keep` 没有任何消费者、
  保留策略删除失败仍计入 removed、附件/Gists 截断无告警、
  回灌历史同秒覆盖 + 写失败被吞、issue 快照混进 PR（GitHub 的 issue 列表含 PR，
  同一 PR 两份快照且占掉 issue 预算）。

## [v1.28.7] - 2026-10-25

### Changed

- core pin 抬到 **v0.20.1**（零 replace）：回灌去重只取第 1 页 ⇒ 目标仓 issue 超过
  100 条后灾备演练**重跑会重复创建同名 issue**（幂等性被破坏，DR 重演变脏数据）；
  冷备完整性清单的 `task_key` 仍按文件名第一段反推 ⇒ key 带 `-` 时对不上任何任务。

## [v1.28.6] - 2026-10-25

### Fixed

- **OpenAPI 契约与实际路由对不上**（`cmd/apidoc/main.go`）：
  - 三个端点实际注册的是 **POST**，契约写成 **GET**（`resolve-org-target` /
    `import-public-org` / `push-backup`）⇒ 照契约调用的客户端拿到 404/405；
  - `/api/v1/mirror/runs` 与 `/api/v1/mirror/targets` **根本没注册**，契约却给它们
    挂了 get+post；
  - 17 个已注册端点不在 OpenAPI 里（force-push 审批三个、AI 配置四个、
    templates delete/preview、import-starred、org-mirror、dr-drill/export…）
    ⇒ spec 驱动的客户端（swagger UI / `gitferry schema`）看不见它们；
  - 同名路径被后一个循环**整条覆盖**（`GET /api/v1/ops/templates` 曾从契约里消失）。

### Added

- **全 API 契约门禁**（`internal/pkg/swagger/api_contract_test.go`）：双向校验 ——
  已注册的路由必须在 OpenAPI 里，**OpenAPI 声明的端点必须真的注册了**
  （此前只有 histsync / mirror 两个前缀有门禁，ops / ai 等路由无人管）。

### Changed

- core pin 抬到 **v0.20.0**（零 replace）：六条数据安全缺陷 ——
  强推放行的「消费」没绑定 target_ref（同一次放行可反复授权覆盖）、
  `git_push_prune` 推完就删目标分支、多分支（glob）任务分歧保护整体 fail-open、
  多分支首次执行不对齐分支、冷备完整性清单零校验却报「通过」、
  RPO 报告把不同任务合并成一条（停更的任务被「洗白」）；
  外加手动触发绕过同 taskKey 互斥、post_exec 脚本无超时。

## [v1.28.5] - 2026-10-25

### Changed

- core pin 抬到 **v0.19.0**（零 replace）：元数据备份**只采集 open PR**，
  且「上限 500」实际只拿到第 1 页的 100 条 ⇒ 已合并/已关闭的 PR 被静默丢弃
  （真实仓里那是绝大多数），快照不完整、灾备回灌缺历史，且回灌侧的
  「PR 状态跟着回灌」根本无从触发（快照里没有 closed/merged 的 PR）。

## [v1.28.4] - 2026-10-25

### Changed

- core pin 抬到 **v0.18.0**（零 replace）：四条并发/回灌缺陷 ——
  同通道并发发布没有真互斥（通道锁只在克隆/fetch 阶段持有，
  **推 tag + 推 main 全程在锁外**）、并发发布把 main 快照回退到旧版本
  （公开仓默认分支回退）、已合并/已关闭的 PR 回灌成"开着的 issue"、
  overwrite 覆盖 label 颜色时不记账不留痕（报告说"新建 N 条"实际是"覆盖 N 条"，
  且人工改过的颜色被无声改回）。
- `RestoreKindStat` 追加 `updated` 字段（**追加**而非重排既有编号，
  不打断已有 thrift 消费者）：覆盖更新与新建分开计数。

## [v1.28.3] - 2026-10-25

### Changed

- core pin 抬到 **v0.17.0**（零 replace）：三条「公开化静默失败」缺陷修复 ——
  同一行多处命中时痕迹替换错位（残留/改坏/panic）、二进制 blob 完全跳过痕迹清理
  （密钥进公开 module）、merged 裁决绕过身份改写（内网 module path 进公开仓）。

## [v1.28.2] - 2026-10-25

### Changed

- core pin 抬到 **v0.16.2**（零 replace）：漂移检测整轮加 30s 总预算 ——
  每个漂移任务要 fetch 一次源端才能算出 ahead/behind,上游慢、任务多时
  整轮能跑到几十分钟，而调用方早已超时（服务端还在空转）。预算耗尽后
  剩余任务如实标「未检查」。

## [v1.28.1] - 2026-10-25

### Fixed

- **已存在的 pending 审批刷新到当前目标状态**（core v0.16.1）：`RequestApproval` 见到同
  task+branch 已有 pending 就跳过 ⇒ 目标侧此后又变了（独有提交从 1 个变成 50 个）时，
  旧 pending 仍挂着、绑的还是**旧状态** ⇒ 运维批的是一份过期的描述。
  修法：已有 pending 时刷新它（reason / target_ref / created_at）。

### Changed

- core pin 抬到 **v0.16.1**（零 replace）。

## [v1.28.0] - 2026-10-25

### Fixed

- **backup_on_demand 兑现不了时 fail-fast，且不能被审批绕过**（core v0.16.0）：该策略要求
  覆盖前先打回滚快照，但没配 `sync.backup_dir` 就打不了。此前**静默按 block 处理**并落一条
  pending 审批 ⇒ 运维一次"放行"就会在**没有备份**的情况下覆盖目标侧独有提交 —— 那正是这个
  策略要防的数据丢失。修法：直接报配置错并**不产生任何审批申请**。
- **强推放行绑定到"被批准时的目标状态"**（core v0.16.0）：审批是"针对这一次分歧"的授权。
  目标侧若在审批之后又前进（独有提交从 1 个变成 50 个），旧审批不该继续适用 ——
  否则一次"批 1 个独有提交"会变成"授权覆盖此后的一切"。修法：审批记录加 `target_ref`
  （申请时目标分支的 HEAD SHA）；判定时目标 SHA 变了 ⇒ 旧放行不再适用，必须重新审批。
  手工申请端点 `POST /api/v1/ops/force-push-approvals` 相应支持 `target_ref`。
- **漂移检测给出真实的 ahead/behind**（core v0.16.0）：此前远端那个 sha 没被 fetch 过 ⇒
  `rev-list` 直接失败，旧实现静默给 0/0 ⇒ "漂移了多少"这个最该有的信息永远是空的。
  修法：先把源端分支 fetch 下来（带凭证，私有仓必须）再算；fetch 失败则明说原因。
  比对分支同时从 `TargetBranch` 改成 `SourceBranch`（工作区里的本地分支是克隆**源端**时
  检出的那个分支；此前源/目标分支不同的任务恒报 "cannot resolve refs"）。

### Changed

- core pin 抬到 **v0.16.0**（零 replace）。

## [v1.27.0] - 2026-10-24

### Added

- **强推审批可撤销**（core v0.15.0）：新增 `POST /api/v1/ops/force-push-approvals/revoke`
  与 CLI `ops +force-push-approvals --revoke <id>`（另有 `--approve <id>` 与列表）。
  放行是"人工授权覆盖目标侧独有提交"，批错了要在**兑现之前**收回；没有撤销口子的话，
  唯一的"取消"办法是等它被用掉 —— 而那正是最不该覆盖的时刻。

### Fixed

- **backup_on_demand 兑现不了时说清楚为什么**（core v0.15.0）：该策略要求覆盖前先打回滚快照，
  但没配 `sync.backup_dir` 就打不了。此前静默按 `block` 处理 ⇒ 策略与实际行为不符
  （操作者以为配了"先快照再覆盖"，实际被拦下），且看不出原因，像是工具随机拦的。
  修法：错误文案点名 `sync.backup_dir` 并说明"本次按 block 处理"。

### Changed

- 依赖抬版：core → **v0.15.0**，go-git-platform → **v0.97.0**（零 replace）。
  SDK v0.97.0 给 gogit 后端的 http/https 传输加了**拨号超时 30s / 响应头超时 60s /
  TLS 握手超时 15s** —— 此前 go-git 的默认传输（`http.DefaultTransport`）**没有任何超时**，
  上游"连不上/连上不回"时 git 操作会一直挂着，调用方只能等自己 context 的超时兜底
  （实测：GitHub 间歇性不可达时一次克隆卡到操作系统默认的 TCP 重试超时，约 2 分钟）。

## [v1.26.0] - 2026-10-24

### Fixed

- **强推放行改为一次性**（core v0.14.0）：`force_push_policy=block` 的审批，
  契约是「Admin **一次性**放行后允许**本次** force」，此前实现是**永久有效** ——
  一次人工放行变成该任务+分支上的常驻后门，之后每次覆盖目标侧独有提交都无人复核。
  现在一次成功的强推之后放行即被用掉，下次同任务同分支再出现分歧必须重新审批。
- **冷备完整性校验没清单时不再 500**：`ops +integrity` 在还没做过冷备（没有
  `backup-manifest.json`）时返回 500 `internal server error`，看不出是「还没做冷备」
  还是「真出事了」。现在返回可读结果并说明该先跑一次冷备。
- **冷备未启用时给可读提示**：`ops +integrity` / 冷备相关端点在 `sync.backup_dir`
  没配时回 400 并说明怎么开，而不是笼统的 500。
- **仓库迁移失败时带出平台原因**：此前只回 `github migration create failed: status 403`，
  看不出是自己没权限、组织开了迁移限制、还是 token 缺 scope；现在带上平台错误串。
- **漂移检测的判词与计数**（core v0.14.0）：两端 ref 不等不再一律报 `diverged`，
  区分 `local ahead` / `remote ahead` / 真 `diverged`；算不出 ahead/behind 时
  明说 unknown，不再静默给 0/0（看着像「没有差异」）。

### Changed

- 失败时不再一律删掉任务工作区（core v0.14.0）：失败常与本地状态无关
  （分歧保护拦下、鉴权失败、网络抖动），此前每次失败都把工作区删掉 ⇒ 下一次被迫全量克隆，
  增量 fetch 的收益归零。现在工作区可用就保留（下次增量），坏了才删。
- `make generate` 的范围说明写进 Makefile：`biz/model/{platform,repo,sync_task,git_sync,base,
  system,webhook,operation_log}` 是**手工维护**的（文件头虽写 DO NOT EDIT，但
  PlatformInfo 的 SSH 主机密钥钉扎 / GitHub App 字段是手工加的，Go 字段名与 optional
  语义都和 IDL 不一致 —— IDL 重新生成会把它们冲掉）。注释说明免得有人「顺手修正」它。

## [v1.25.0] - 2026-10-24

### Fixed

本轮在「GitHub ↔ 自建 Forgejo 双向同步」实测中定位并修掉的缺陷。前三条是
**报成功但数据错**级别：任务状态显示成功，目标侧实际没前进 / 目标侧独有提交被覆盖。

- **增量同步永不前进**（core `executor.fetchRepo`）：`git fetch` 只更新
  `refs/remotes/origin/<branch>`，**本地分支仍停在旧提交**；而 push 推的是
  `refs/heads/<branch>`（本地）。于是目标侧永不更新，git 回 `already up-to-date`
  被当成成功 —— **首次克隆正常，第二次起每次都空转**。
  修法：fetch 之后把本地分支对齐到源端 ref（HEAD 在该分支上用 `reset --hard`，
  否则用 `update-ref` 只动 ref，不牵动别的分支）。
- **分歧保护自我失效**（core `executor.checkDivergencePolicy`）：检查目标分叉时
  fetch **没带凭证**（自建实例/私有仓/过期证书连不上），而 fetch 失败被当作
  「无分歧风险」直接放行 ⇒ 目标侧独有提交被 force push 覆盖，**任务仍报成功**。
  修法：fetch 带上平台凭证；连不上时**fail-closed**（本地若已有该目标分支的
  远端跟踪 ref，说明目标确实有这个分支，必须报错让人工介入）。
- **部分更新静默禁用任务**（`idl/sync_task.thrift` + 壳层 handler）：
  `UpdateTaskReq` 的 bool 字段曾是非 optional，壳层用 `&req.Enabled` 取地址传给
  core ⇒ 指针恒非 nil，core 的「nil 即不修改」形同虚设。
  `gitferry task +update --cron "0 3 * * *"` 会把任务**禁用**并清空所有布尔开关。
  修法：IDL 改 `optional bool`，壳层直传指针；core 侧语义不变。
- **cron 锁 TTL 与续期间隔冲突**（core `service/concurrency.go`）：续期间隔是
  **锁级固定值**，而 cron 锁 TTL 曾等于续期间隔 ⇒ 第一次续期就撞上 key 过期，
  锁从未被续上；锁只在前 10 秒有效，之后多实例可并发跑同一任务。
  修法：cron 锁 TTL 提到 `3×renewInterval`，**并在本次执行结束后显式释放**
  （不释放 = 续期成永久锁 ⇒ 之后每次 tick 都拿不到，cron 静默停摆）。
- **执行结束回写状态抹掉并发修改**（core `service/task_service.go`）：
  回写用整行 `Save(task)`，而 task 是**执行开始时**的快照 ⇒ 执行期间运维改的
  `force_push_policy` / `cron` / 开关会被旧快照悄悄改回去（实测：一次 cron 执行
  把刚改的 `force_push_policy=allow` 还原成 `block`，表现为「改了没生效」）。
  修法：回写只碰自己拥有的两列（`last_run_at` / `last_status`）。
- **已设的 cron 清不掉**（`idl/sync_task.thrift` + core）：`cron` 曾是普通 string，
  空串与「没传」不可区分 ⇒ 一旦设了 cron 就再也改不掉（只能删任务重建）。
  修法：改 `optional string`（nil=不修改 / 空串=清空），core 侧同步改指针语义。
- **`git_tags=true` 从来没把 tag 推到目标端**（core `executor.push`）：主推送只带
  分支 refspec，tag 只在 fetch / bundle / backup-remote 里被用到。修法：tag 单独推
  （不覆盖已存在的 tag，等同 `git push --tags`）；批量失败时退化成**逐个推** ——
  git 的批量 push 是全有全无，一个冲突的 tag 会把其它本来没问题的 tag 一起挡住。
- **分歧错误文案里的「独有提交数」是错的**（core `executor.divergentError`）：
  用的是**有上限截断的展示列表长度**（maxDivergentLog=5），目标侧只有 1 个独有提交时
  文案写「has 5 commit(s)」。修法：改用 rev-list 的真实 behind 计数。
- **`GetAPIURL` 把 github.com 拼成 `github.com/api/v3`**（core `model/platform.go`）：
  那是 **GitHub Enterprise** 的路径，github.com 的 REST API 在 `api.github.com`
  ⇒ 按旧逻辑建出来的 GitHub 平台必坏（连通性检测直接 404）。修法：已知 SaaS 实例
  用整域名基址表（与 SDK 的 `knownHostPlatforms` 对齐），并加交叉校验测试。
- **glob 多分支两处非法 refspec**（core `executor`）：`git clone --branch 'release/*'`
  与 `git fetch origin 'release/*'` 都不是合法用法 ⇒ 多分支任务从来跑不起来。
  修法：多分支走全量 clone / 全量 fetch，再按 glob 建出本地分支。
- **镜像引擎增量 fetch 不更新工作区**（core `service/mirror_service.go`）：
  镜像发布从**工作区**取材，fetch 后工作区还是旧的 ⇒ 每次发旧快照。
  修法：fetch 之后把工作区对齐到源端默认分支。
- **镜像发布没把平台 TLS 设置传给 go-git 传输**（core `mirror` + `service`）：
  `TransportAuth()` 只产出 `transport.AuthMethod`，不带 TLS 开关 ⇒ 自建实例
  （自签/过期证书）的「远端一致性检查」必然失败、发布直接中止。
  修法：显式传 `InsecureSkipTLS`，并按 host 找回目标所属平台补上它的 TLS 策略。
- **webhook 接收路由接不住 `owner/repo`**（`biz/router/custom.go`）：`:repoKey`
  只匹配**单段**路径，而仓库 key 的标准形态含斜杠 ⇒ 平台推过来的 webhook 直接 404。
  修法：改用 catch-all `*repoKey`。
- **备份推送把父目录当仓库**（`biz/handler/git_sync/backup_remote_service.go`）：
  传的是 `<tempDir>/<taskKey>`，真正的 git 仓库在 `.../repo` ⇒ 必然
  「repository does not exist」。修法：core 新增 `GetRepoDir`，并把「repo」子目录名
  收成单源（`model.RepoDirName`）。
- **CLI 六个命令的参数/flag 对不上服务端**（`internal/cli/commands/domains.go`）：
  `ops +drift` 用 GET 而路由只注册 POST（永远 404）；`ops +push-backup` /
  `ops +repo-files` 的必填 flag 没注册（命令永远跑不了，`--refspec`/`--path` 被
  `flagStr` 静默吞掉）；`repo +list --keyword` 传 `keyword` 而服务端要 `search`
  （静默不过滤）；`task +list --keyword` 同样传错；`webhook +rules`/`+events` 不传
  服务端必填的 `repo_key`（永远 400）。

### Added

- **GitHub ↔ 自建 Forgejo 双向同步的完整实测**：两条对向任务 + 镜像发布通道 +
  webhook 规则 + 强推审批流 + cron 定时同步，全部在真实两端跑通并留证。

### Added

- **MCP 端的写工具恢复可用**（`internal/agent/tools/scope_client.go`、
  `internal/mcp/server.go`）：新增 `ClientConfirmedScope` —— **确认交给客户端**，
  服务端只管角色与强推授权。
  - 起因：MCP 是**无状态**协议，服务端「登记待确认」的令牌无处可去 ——
    于是 `sync_history_channel` / `set_history_override` /
    `rotate_history_target_credential` 三个写工具在 MCP 上**一律不可用**
    （缺会话 scope → 直接拒绝）。而 MCP 客户端（Claude Code / Cursor）在把工具调用
    发过来之前**自己就已经向用户展示并征求过同意**了 —— 确认本来就发生在客户端那一侧。
  - **不绕过什么**：角色闸门照旧（`dangerGuard` 的角色检查在 scope 检查**之前**，
    fail-closed）；`force_push` 的 admin 闸门照旧（core 层的批准者校验）；审计链照旧。
  - 口径对齐：MCP 路径与 HTTP 路径一致（客户端负责确认、服务端负责角色）；
    CLI 与内置 AI 助手是本进程 agent，服务端必须自己加确认闸 —— 那条差异有理由。
- **`internal/pkg/authz`**：访问角色（admin / operator / readonly）与它在 Go context 里的
  传递。角色原本只存在 hertz 的 `RequestContext` 上，而**工具层要判角色** ——
  MCP 端点与 AI 助手都是进程内直调 core service，不过 HTTP 路由。
- **历史同步的 HTTP 层端到端测试**（`biz/router/git_sync/histsync_e2e_test.go`）：
  真实 hertz 引擎 + API Key 鉴权 + 全部路由 —— 建通道 → 配两个推送目标 → 预检 →
  移植 → 推送 → 公开仓真的前进 → 审计链 `"ok":true`。
- **历史同步的 RBAC 与强推测试**（`biz/router/git_sync/histsync_rbac_test.go`、
  `biz/handler/git_sync/histsync_force_test.go`、`internal/mcp/rbac_test.go`）：
  只读/operator/admin 三档角色 × 写工具 × 强推授权的边界。
- **CLI 的历史同步测试**（`internal/cli/commands/histsync_test.go`）与
  **histsync 技能文档的确认口径表**（`skills/gitferry-histsync/SKILL.md`）：
  逐入口写清「确认在哪一侧做、谁负责角色」。

- **镜像通道支持痕迹清理规则**（`CreateMirrorChannelReq.scrubRules`）与
  **镜像目标支持 SSH 主机密钥钉扎**（`insecureSkipTLS` / `hostKeyFingerprint` /
  `knownHostsPath`）：镜像发布的产物是**对外可见的公开 module** ——
  身份改写只管 module path，内容里的内网 registry / 部署代号 / 令牌原样进公开仓就是泄漏；
  而目标侧之前没法钉主机密钥，于是同一次发布里「检查路径」fail-closed、
  「推送路径」TOFU，两套策略相反。两项都在 core 落地后经 thrift 契约暴露
  （`idl/mirror.thrift` → `hz model` 重新生成 `biz/model/mirror`，字段 ID 只追加不重排 ⇒ 向后兼容）。
- **histsync 与镜像中心的 37 条 HTTP 端点进入公开 API 契约**（`cmd/apidoc/histsync.go`、
  `cmd/apidoc/mirror.go`）：此前这 37 条路由是**手写**的、不在任何契约里
  ⇒ `gitferry schema`（Agent 自 introspect 的入口）完全看不到它们。
  契约按路由表**逐条声明**（路径/方法/入参/出参），请求与响应字段与 core 的
  json tag 对齐；两份产物（`docs/openapi.json` 与内嵌副本）都由 `make apidoc` 生成。
  - **没有新建 thrift 文件**：thriftgo 只编译 `git_sync.thrift`（含其 include），
    新文件既不生成代码也不受校验，只是一份没人验证的副本；而且请求/响应结构体
    的真相在 core 的 Go struct 里，thrift 再抄一份 = 第二份真相，core 改字段时
    契约反而开始骗人。
  - **防漂移门禁**（`internal/pkg/swagger/{histsync,mirror}_contract_test.go`）：
    解析路由注册源文件的 AST（还原 hertz 分组前缀）拿到**实际注册的路由集合**，
    断言契约覆盖它 —— 以后加/改路由忘了更新契约，测试立刻红。
    不用运行时 `hertz.Register` + `engine.Routes()`：那会把整个 handler 依赖链
    拖进底层包的测试里，别人改坏一个包就会让这条门禁跟 histsync 无关地红。
- **危险工具改成单一事实源**（`internal/agent/tools/danger.go`、`repo.go`）：
  危险标注写在**注册处**（`addDanger`），原先那是**另一份手写清单**
  （`DangerTools` map）—— 两份清单漂移时，执行层安全不受影响
  （`dangerGuard` 自己有角色闸），但 **tools/list 过滤**与
  **system prompt 的危险工具清单**会说谎。
  - `Policy.Classify(ctx, tool, danger bool)` 改由调用方传入「是否危险」，
    策略层不再引用全局集合。
  - 包级 `DangerToolNames()` / `IsDangerTool()` 从同一份注册代码派生
    （提示词是包级变量、init 阶段就要成形，那时还没有 Registry 实例）。
  - **防漂移门禁**（`internal/agent/tools/danger_drift_test.go`）：静态扫描本包源码里
    所有 `dangerGuard(ctx, "<名字>", ...)` 调用，断言它们都在危险集合里 ——
    新增危险工具忘了标注就红。


### Changed

- **MCP 的历史同步工具统一加 `histsync_` 前缀**（`internal/agent/tools/repo.go`）：
  旧名 `list_history_*` 与既有的 `list_sync_history`（「查询任务最近同步执行历史」）
  **前缀相反、语义打架** —— 模型看到 `list_history_runs` 会合理地以为它是同步执行历史,
  实际是 histsync 的执行记录。CLI 侧早就为同一件事绕开了(`histsync` 而非 `history`),
  工具层没跟上。现在两边命名一致:
  `list_histsync_channels` / `plan_histsync_channel` / `resolve_histsync_channel` /
  `list_histsync_runs` / `verify_histsync_channel_audit` / `list_histsync_overrides` /
  `set_histsync_override` / `preview_histsync_channel` / `sync_histsync_channel`。
  （仓外引用面已核实为**零**：npm-pkg / skills 都没有引用这些工具名。）
- **`list_histsync_channels` 的入参形状修正**：它此前复用 `historyChannelRef`,
  而那个结构的 `channel_id` 没有 `omitempty` ⇒ schema 把它标成**必填** ——
  列全部通道根本用不上它,模型只能被迫随便填一个 id。
  现在它有自己的入参(分页),并去掉了硬编码的 100 条上限(通道多了以后,
  模型看到的是被静默截断的列表,而它并不知道还有下一页)。
- **`/push` 的 `force` 改名为 `force_push`**（`biz/handler/git_sync/histsync_service.go`）：
  同一个键 `force` 在 `/sync` 表示「忽略阻塞项继续」、在 `/push` 却表示「强制推送」——
  **语义相反的同名键**迟早被用错,CLI 被迫写一层手工转译(`--force-push` 传过去叫 `force`),
  传错键还会被服务端静默丢弃。现在两个端点都用 `force_push`;
  旧键 `force` **明确报错并指路**(而不是静默忽略 —— 旧调用方以为强推了、实际没推,
  比推不动更危险)。
- **审计明细不再落本机绝对路径**（通道创建的 audit detail）：
  明细进**不可篡改的哈希链** —— 写进去就等于永久提交(事后想改会让整条链报「被篡改」),
  而绝对路径把执行机的目录结构与操作者用户名永久写进了审计记录。
  现在降级为「目录名 + 短哈希」:能与通道配置对照,又不泄露本机布局。
  （上一轮已把元数据快照的绝对路径改成相对路径,这条是同类的最后一处。）

- **CLI 的 histsync 子命令不再静默丢弃参数**（`internal/cli/commands/histsync.go`）：
  CLI 过去把 29 个 flag 撒在**每个**子命令上，于是 `+plan --force-push`、
  `+channel-list --verdict merged` 这类无效参数**被静默接受**（实测 21 个命令
  共 **578 处静默丢弃**）—— 用户以为设上了，服务端根本没收到。
  静默丢弃比报错危险：它让人以为操作生效了。
  现在每个命令声明自己**真正会读**的 flag 集，cobra 的 `PreRunE` 统一拦：
  任何「不属于本命令」的参数一旦被显式设置就报错退出。
  （上一轮只给 `+push --force` 单独补了报错，同一个病在另外 18 个命令上还在。）
- **MCP 的工具结果同时给出结构化内容**（`internal/mcp/server.go`）：
  工具内部返回的本来就是结构化 JSON，但在 MCP 边界上只包成了文本块 ⇒
  客户端（Claude Code / Cursor）只能自己解析字符串。
  现在按规范补上 `structuredContent`（与 `content` 并列）；
  老客户端不认识会忽略（行为退化成现状，无破坏）。

- **工具权限模式改为会话级**（`internal/agent/tools/permission.go`、
  `internal/agent/session.go`）：之前档位对象是**进程级单例** —— 一个会话把全局切成
  readonly，会连带把**其他所有会话**的写操作全拒（跨会话影响）。现在档位落在会话上。
- **`set_permission_mode` 补齐三道闸门**：参数与角色**先判、再进确认链路**（否则
  operator 会先看到一个确认弹窗，确认完才被告知「你没权限」）；**只有 admin 能拨**
  （拨档是治理动作）；需用户确认（走 `dangerGuard`）。
  该工具**不受只读档拦截** —— 否则成了一条单行道（切到只读之后再也切不回来）。
- **`remember` / `forget_memory` 按危险工具处理**（`internal/agent/tools/memory.go`）：
  记忆会拼进**每次会话的 system prompt** ⇒ 跨会话持久化的**提示注入通道**；
  之前是裸工具，只读角色也能写。现在需用户确认 + 只读角色一律拒绝。
- **元数据快照只报相对路径**（`internal/agent/tools/backup_dr.go`）：快照的绝对目录是
  服务器本机布局，对使用者没有价值，却会一路流进模型上下文。
- **`internal/converter` 的手工字段搬运清零**（77 → 0 逐字段拷贝）：
  改成一个可复用的反射式字段搬运器（`mapper.go`）+ 每个家族一份「例外契约」
  （`fieldSpec`：名字不同的构、该跳过的字段、要算出来的派生字段）。
  core 加字段时理想情况只改 core 与 thrift 两处，转换层不动。
  - **JSON 形状逐字不变**：新增黄金 JSON 测试（18 个样本，期望值取自重构前的实现）+
    字段覆盖度测试（每个 DTO 字段要么被自动搬走、要么显式声明），两个安全网都做了
    反向验证。
  - 仍然必须逐字段的只有 `ApplyPlatformUpdate`（那是**更新策略**不是字段搬运：
    哪些字段允许改、各字段的「未更新」哨兵不统一，强行统一会改行为）。
  - core ↔ thrift ↔ agent 三层的字段名对照（含 `source_repo_key` vs agent 的
    `source_repo` 这类差异）记在 `internal/converter/doc.go`；
    **agent 侧的字段名统一**未做（会对模型看到的 JSON 形状是破坏性变更，需上层拍板）。
- **`UpdateTaskReq` 的 bool 字段改为 optional**（`idl/sync_task.thrift`）：
  原来它们是必填 ⇒ **部分更新必须把所有 bool 都重传一遍**，漏一个就 400。
  改成 optional 后，未传的字段表示「不更新」。
  HTTP 侧是**宽松化**（老客户端照常传即可，向后兼容）；Go 侧是破坏性的
  （生成类型 `bool` → `*bool`），仓内已适配。


### Fixed

- **确认流不再依赖「危险工具清单」**（`internal/agent/decorator.go`）：
  工具没通过确认时返回的就是 `{"status":"confirmation_required",…}` ——
  那才是「需要确认」的事实。装饰器此前查的是**另一份事实源**(危险工具清单):
  清单漏了这个工具时,前端收不到确认事件 ⇒ 模型等在那里、工具永远不执行,
  而用户界面上一片安静。现在以工具自己的回答为准。
  （顺带澄清:执行层的角色闸在 `dangerGuard` 里,**独立于**那份清单 ——
  清单漏了不会让只读角色执行写工具,只影响展示层与提示词层。）

- **MCP 路径上危险工具被「会话上下文缺失」整体挡死**（第四轮复核发现）：确认机制与
  角色闸门混在一起，导致 MCP 上三个写工具全不可用（见上「Added」）。
- **CLI 的 `histsync +push --force` 是哑弹**：它的语义其实是 sync 的「忽略阻塞项」，
  与「覆盖远端已有提交」不是一回事 —— 静默丢弃会让用户以为覆盖了分叉检查。
  现在直接报错并指向 `--force-push`。
- **凭据参数只挂 `target-*` 子命令**（`internal/cli/commands/histsync.go`）：
  避免凭据出现在无关命令的 flag 列表里。

## [v1.24.0] - 2026-10-23

> 依赖 git-ferry-core **v0.12.0**（新增 `mirror/history` 包：历史同步引擎；
> minor 版无破坏性变更）+ go-git-platform **v0.96.0** +
> oekit `core` **v0.12.0** / `pkg` **v0.22.0** / `concurrency` **v0.20.1** /
> `logx` **v0.7.1** / `redisx` **v0.10.0** / `bootstrap` **v0.6.4** /
> `hertz` **v0.8.1** / `metrics` **v0.1.9** +
> oagentkit `guardx` **v0.14.0**（私有仓，构建需 `GOPRIVATE=github.com/oagentkit/*`）+
> eino **v0.9.22**。

### Added

- **历史同步（histsync）：双仓内容差异发布**——把内网源树的内容按差异移植到公开目标树。
  与镜像中心的「身份镜像」并存，语义不同：那边是同一仓库的多个身份，
  这边是两个各自有完整历史的仓库之间按内容差异同步。
  - HTTP API（`biz/handler/git_sync/histsync_service.go`）：通道 CRUD、规则配置、
    生成同步计划、裁决写入、预检、内容移植与推送。
  - CLI（`internal/cli/commands/histsync.go`）：`gitferry histsync` 子命令。
  - Agent 工具（`internal/agent/tools/histsync.go`）：生成同步计划、裁决冲突、
    内容移植与推送、预检。**裁决写入按「危险工具」处理**——模型给出判定与依据 →
    弹出确认 → 用户确认后才写入，这是「AI 建议 + 人工确认」的闭环落点。
  - Agent Skill（`skills/gitferry-histsync/`）：触发「两棵树怎么同步 / 公开仓少了什么 /
    哪些文件该带过去」这类场景。
  - 审计日志补 `Detail` 明细（`recordAuditDetail`）：结构化补充信息同样进入哈希链
    payload，明细被改也会被检出。

### Changed

- **eino v0.9.19 → v0.9.22**：本仓 `internal/agent/**` 直接消费 eino
  （`adk` / `compose` / `schema` / `components/tool` / `components/model`）。
  同时与 oagentkit `guardx v0.14.0` 的 eino 下限（v0.9.21）对齐——
  后续若引入 oagentkit/agent 不会再因 eino 版本被顶回。
  `eino-ext/components/model/openai` v0.1.13、`eino-ext/libs/acl/openai` v0.1.17、
  `eino-contrib/jsonschema` v1.0.3 均已是各自最新。

## [v1.23.0] - 2026-10-22

> 依赖 git-ferry-core **v0.11.0**（含 BREAKING：`lock.Semaphore.Renew` 改 `(bool, error)`、
> `Cleanup` 改 `(int64, error)`、删 `LocalLock`）+ go-git-platform **v0.96.0** +
> oekit `core` **v0.12.0** / `pkg` **v0.22.0** / `concurrency` **v0.20.1** /
> `logx` **v0.7.1** / `redisx` **v0.10.0** / `bootstrap` **v0.6.4** /
> `hertz` **v0.8.1** / `metrics` **v0.1.9** +
> oagentkit `guardx` **v0.14.0**（私有仓，构建需 `GOPRIVATE=github.com/oagentkit/*`）。

### Changed

- **依赖地板再抬一档**：oekit `pkg` v0.21.0→**v0.22.0**、`concurrency` v0.20.0→**v0.20.1**、
  `oagentkit/guardx` v0.13.1→**v0.14.0**。三个版本正是上一轮向上游提的 issue 的落地：
  - `pkg v0.22.0` 补齐 `pathx.MatchGlob`（#64）、`pathx.IsSafeFileName`（#63）、
    `stringx.SplitCSVNonEmpty`（#65）三个原语；
  - `concurrency v0.20.1` 修 `syncx.KeyedGate` 的 release 竞态（#70）——改用
    `atomic.Bool` CAS，与同包 `Gate` 对齐，不再有「双重归还名额突破 cap 上限」；
  - `guardx v0.14.0` 让 permgate **原生支持**「只读档对写入类工具是禁不是问」（#24）。
- **收编上游新原语，删除本地第二份口径**：
  - 本轮曾为消费方缺口在本地实现过 `MatchGlob` / `IsSafeFileName` /
    `SplitCSVNonEmpty` 三个原语——上游已补齐同名原语，**本地实现全部删除**，
    调用点改为直接调 oekit（`pathx.MatchGlob` / `pathx.IsSafeFileName` /
    `stringx.SplitCSVNonEmpty`）。上游实现比本地更正确：
    `pathx.MatchGlob` 把「`*` 是通配符」的判定**前置于**字面量比较
    （本地实现顺序反了，模式里的 `*` 会被当字面量：`MatchGlob("a*b","a*b")`
    本地返回 false、上游返回 true）；`stringx.SplitCSVNonEmpty` 全空白输入返回
    **nil**（本地返回空切片，会把「没配」与「配了空」混同）。
  - `internal/pkg/textutil` 的两个转发函数（`IsSafeFileName` / `SplitCSVNonEmpty`）
    随之删除——本仓直接调 oekit。
- **工具权限判定不再有本地只读兜底**（`internal/agent/tools/permission.go`）：
  「只读档对危险工具是拒不是问」此前由本地一行判定兜底（因为 permgate 的
  `AlwaysAsk` 段会先于模式判定短路返回 Ask）。`guardx v0.14.0` 已原生支持
  （只读拦截段插在 project ask 之前、以及 `checkAlwaysAsk` 的会话授权之后，
  RuleID/Reason 与原只读段逐字相同、公共 API 零变更），本地兜底删除。
  保留一处本地判定：显式 override 最高优先（permgate 的规则集排在只读档之下，
  表达不了「把某工具显式改成只读/放行」）。
  新增测试钉住只读档对危险工具=拒的口径——上游若改动判定顺序，测试会先红。
- **依赖地板抬到真最新**：oekit `core` v0.9.0→**v0.12.0**、`pkg` v0.14.0→**v0.21.0**、
  `concurrency` v0.16.0→**v0.20.0**、`logx` v0.7.0→**v0.7.1**、`redisx` v0.9.0→**v0.10.0**、
  `bootstrap` v0.6.2→**v0.6.4**、`hertz` v0.6.4→**v0.8.1**、`metrics` v0.1.7→**v0.1.9**、
  go-git-platform v0.90.1→**v0.96.0**；go 指令 1.27.2→**1.27.3**（与 go-git-platform 对齐，
  go1.27.3 修复 crypto/x509 GO-2026-6809）。`github.com/oekit/pkg` 由间接依赖升为直接依赖。
  版本一律用真实发布版本，仓内无 `replace`。
- **新引入 oagentkit 家族**（`github.com/oagentkit/guardx` v0.13.1，私有仓需
  `GOPRIVATE=github.com/oagentkit/*`）：见下「新增 oagentkit 落点」一节。
  刻意**不**引入 `oagentkit/agentkit` 门面（其自身标注已废弃、只保留兼容转发，
  且会拖入全部 12 个子模块含 milvus SDK / mcp-go），**不**引入 `toolgate/policy`
  （其判定模型是「变异工具一律 Deny」，与本仓「危险工具走确认」的形态正面冲突）。
- **手写周期循环收编 oekit `loop`**(heartbeat / AI 会话 janitor / runwatch poll 三处):
  立即首跑与 FirstDelay 语义与旧 ticker 逐一对应;增强:单轮 panic 经 RunSafeLabeled
  收编(此前 panic 会杀死整个循环,现在循环存活且告警带任务名)、忙时丢 tick 不重叠。
- **SystemStatus 双查询并行改 `syncx.FanInErr`**:错误/panic 统一进 Slot.Err
  (async.PanicError,全局 PanicHandler 观测),删除 channel + RecoverToErr 样板。
- **手写通用工具收编 oekit `pkg`**：
  - `biz/handler/git_sync/optbind.go` 整个文件删除，三个手写可选指针解包
    (`optStr`/`optBool`/`optBoolDefault`) 换 `ptrx.Deref`（16 个文件 50 处调用点）。
  - 8 处 `strconv.Atoi(c.Query(...))` 丢 error → `convert.Str2Int` / `convert.Str2Int64`；
    `trends_service.go` 手写 `parseIntQuery` 删除。
  - `forcepush_approval.go` 手写 `minInt` → Go 内建 `min`。
  - 两份重复的逗号串切分（`splitCSV` / `parseTaskKeys`）合并为 `splitCommaList`，
    用 `stringx.SplitByComma` + `slicex.Filter`（**空结果返回 nil**——core 的
    `UpdateRule` 有 `if req.TaskKeys != nil` 门，返回空切片会把「保留原列表」变成「清空」）。
- **手写 CSV 拼装收编 `hertz/render.CSV`**（审计报告 / 问题列表两处）：
  改走标准库 `encoding/csv`（RFC 4180）。与原 `textutil.CSVEscape` 的普通行输出逐字节
  一致；差异是 `encoding/csv` 还会给**首字符是空白**的字段加引号（RFC 正确性提升）。
- **手写路径穿越防线收编 `pkg/pathx.SanitizeRelStrict`**（`backup_remote_service.go`）：
  严格档显式 `..` 段一律拒绝，并额外拒绝 Windows 盘符 `C:\foo`、UNC `//host/share`
  与绝对路径——**原实现这些在 Linux 上全部漏过**。`.`（列仓库根目录）单独放行；
  原有 `Abs` + `HasPrefix` 双保险保留作纵深防御。附带修掉旧实现按**子串** `..`
  拒绝、误杀 `foo..bar.git` 这类合法文件名的过度拦截。
- **webhook per-IP 限流器收编 oekit `concurrency/ratelimit`**：手写令牌桶 map + 惰性清理
  → `NewKeyedLimiterByFunc` + `TokenBucket` + `WithIdleTTL(10min)`，桶的空闲回收由引擎负责。
  **刻意不设 `MaxEntries` 硬上限**：key 来自 `ClientIP()`，而 hertz 默认可信网段是
  `0.0.0.0/0`（信任所有对端的 `X-Forwarded-For`），key 空间可被伪造——硬上限会让伪造流量
  占满表后反过来把合法 webhook 来源全拒成 429。测试用的假时钟改用
  `ratelimit.WithTokenClock` 注入（复用同一个桶，与原 `AllowN(at,n)` 等价）。
  限流器的后台回收 goroutine 由 `CloseWebhookRateLimiter()` 收口，已接进 `main.go` 的退出路径。
- **手写轮询收编 `concurrency/retry.Poll`**（GitHub migration 导出）：
  原实现先睡后查且**完全忽略 ctx 取消**；改为先查后睡（首次查更早，无损），
  超时（`retry.ErrTimeout`）静默收束并保留旧 state（对外行为不变），ctx 取消则提前结束。
- **AI 助手并发闸门收编 `concurrency/syncx.Gate`**：手写 `chan struct{}` + `select/default`
  → `Gate.TryAcquire()`（满额即拒，语义逐位一致；release 幂等）。
- **`internal/corebridge/tokenenv.go` 改用 `exec.CommandContext`**：原实现是
  goroutine + `time.After`，超时后只是放弃等待、**子进程仍在后台跑**（泄漏）；
  现在超时即杀子进程，墙钟上限不变。
- **删 `internal/pkg/safe`**：该包注释声称「oekit concurrency/async 没有覆盖的 panic
  处理原语」——**这个前提从未成立**（`async.RunSafeErr`/`RunSafeRetLabeled` 自
  concurrency v0.10.0 起就在）。唯一调用点改用 `async.RunSafeRetLabeled`，
  panic 收编为带 payload 与栈的 error 回传（此前若直接删掉 defer 辅助，
  「统计 panic」会被当成「统计成功且全为 0」返回 200）。
- **AI 会话 run 排序收编标准库 `slices.SortFunc`**：手写插入排序 O(n²) → O(n log n)。
- **bundle 文件名校验收敛为单一事实源**（`strutil.IsSafeFileName`，core 侧实现）：
  壳层 verify / restore bundle 与 core 的 DR 演练原本各写一份
  `filepath.Base(name) != name`——**该判定漏了 `".."`**（`filepath.Base("..") == ".."`），
  而 name 随后被 `filepath.Join(backupDir, name)` 拼进路径，能读到 backup 目录的**父目录**。
  改用 core 的 `strutil.IsSafeFileName`（`pathx.SanitizeRelStrict` 严格档 + 单段检查），
  壳层经 `internal/pkg/textutil` 转发层调用。
- **git smart HTTP 路径穿越防线收编 `pkg/pathx.SanitizeRelStrict`**（`internal/gitserve`）：
  迁移前是 `path.Clean("/"+p)` + `strings.Contains(p,"..")`——后者是**子串**判定，
  既让 Windows 风格分隔符的口径与 Linux 不一致，又误杀 `foo..bar.git` 这类合法仓库名。
  现在统一分隔符口径（`/` 与 `\` 同为分隔符，跨平台同判）、拒显式 `..` 段与盘符/UNC，
  并补专项测试：穿越用例（含编码形态 `%2e%2e%2f`）与合法名（`foo..bar.git`、重复斜杠）两侧都钉住。
- **`internal/notify` 手写去尾斜杠** → 标准库 `strings.TrimRight`。
- **JSON recovery 中间件收编 oekit `core/chain` + `hertz/chainmw`**（`biz/serve`）：
  手写的 defer/recover 换成 `chain.Recovery`（panic 收编为带 payload 与栈的
  `*chain.PanicError`）+ `chainmw.Wrap`（错误出口经 `WithOnError` 完全接管——
  chainmw 默认出口是 `middleware.Fail` 的统一响应格式，本仓要自己的 JSON 体）。
  此前这条链路**零测试覆盖**，现补 `biz/serve` 测试钉住「panic → JSON 500
  （非 Hertz 默认的 HTML）且后续请求仍可用」。
- **AI 配置默认值收敛为单一事实源**：`0.3 / 2048 / 60 / 4` 这组默认值此前在四处
  各写一遍（配置校验 / 设置更新 / 设置转运行时 / runner 兜底），改一个要同步四处。
  现在值只在 `config.go` 的常量里定义一次。
- **`internal/converter/safe_convert.go` 边界钳制收编 oekit `pkg/mathx.Clamp`**：
  三个手写钳位函数（int→int32 / int64→int32 / uint→int32）塌成一行，语义逐字相同。
- **分页缺省统一为 50**：镜像通道列表此前独立实现了一套钳制且缺省值是 20，
  于是同一次请求在任务/仓库/操作日志列表返回 50 条、在镜像通道列表返回 20 条。
  现与其它列表端点共用 `converter.PageToOffset`（缺省与上限取 core dao 的口径）。
  **对外行为变化**：镜像通道列表每页条数由 20 变为 50。
  实现上只依赖 `corebridge.DefaultPagination`（已发布的转发入口）而**不直接引用
  core 的新增常量**——core 尚未发版时，壳层仍能按已发布的 core 编译
  （`GOWORK=off go build ./...` 可过）。钳制顺序有讲究：先用它把 pageSize 钳好、
  **再**用钳后的值算 offset，否则「页大小超上限」时 offset 会算在未钳的页大小上、
  翻页跳号；已补测试钉住这一顺序。
- **CLI 的 CSV 拆分与 int→string 收编到 core 统一入口**：本地 `splitCSV` 与
  `itoa` 分别改为调用 core 的 `strutil.SplitCSVNonEmpty` / `strutil.Itoa`。
- **`internal/metrics` 的 `sortedKeys` → 标准库 `slices.Sorted(maps.Keys(m))`**
  （与 core `executor/drdrill.go` 的同名实现逐字同构，两份合成一份口径）。

### Fixed

- **`internal/corebridge` 密钥命令超时不再泄漏子进程**：见上 `exec.CommandContext` 一条。
- **bundle 名可逃逸到 backup 目录的父目录**：见上 `IsSafeFileName` 一条（壳层与 core 两处同款）。
- **system prompt 里的危险工具清单与代码脱节**：提示词手写了一份只列 3 个
  （`run_task` / `test_repo_connection` / `test_platform_connection`），而
  `tools.DangerTools` 实际有 6 个——`retry_sync_run` / `rebuild_task` / `run_dr_drill`
  同样要走确认链路，模型却不知道，会直接调用而不提示等待确认。
  清单改为从 `tools.DangerTools` 派生（排序后填入模板），加危险工具只改那一处；
  新增测试钉住「每个危险工具都出现在 system prompt 中」。
- **平台仓库过滤的 glob 模式此前等于没配**（core 侧修复，行为影响到本仓的
  平台自动发现与仓库过滤）：core 内三份 glob 实现语义各异，其中平台仓库过滤用的
  `filepath.Match` 不让 `*` 跨 `/`，而匹配对象是仓库全名 `owner/repo` ——
  于是 `exclude_globs: ["*"]` 这类模式永远匹配不到，拦不住任何仓库。
  现统一到 core 的 `strutil.MatchGlob`（`*` 跨 `/`），详见 core CHANGELOG。

### Added

- **oagentkit 落点（两处，均在 `internal/agent`）**：
  - **`guardx/permgate` 接管工具权限判定**（`internal/agent/tools/permission.go`）：
    权限判定从一段本地 switch 变成 oagentkit 的规则引擎裁决——本仓三档动作
    (`readonly`/`confirm`/`denied`) 映射到 permgate 的 `Behavior{Allow,Ask,Deny}`，
    危险工具集合映射为 `Capability{AlwaysAsk}`，模式映射为 `Mode{Default,ReadOnly}`，
    裁决结果带 `RuleID`/`Reason`。**两处判定留在本地**（permgate 的判定顺序表达不了）：
    ① 显式 override 最高优先（permgate 的规则集排在只读档之下）；
    ② 只读模式下危险工具一律**拒**（permgate 的 `AlwaysAsk` 段会先于模式判定短路
    返回 `Ask`＝「问一句」，而本仓只读档的语义是「拒」）。
    新增测试钉住这两条与审计档行为。
  - **`guardx/injectband` 给记忆注入加数据围栏**（`internal/agent/prompt.go`）：
    记忆摘要是「用户/agent 曾经写下的内容」——属于数据不是指令。此前是裸拼进
    system prompt，一段记忆正文里写个伪造的 `<system-reminder>` 标签就能把自己
    抬升成系统注入（散文规则挡不住结构化标签）。现在包成
    `<system-reminder source="memory">…</system-reminder>`，且 body 内伪造标签
    被转义。新增测试钉住这一点。

## [v1.22.1] - 2026-10-17

> 依赖 git-ferry-core **v0.10.1** + go-git-platform **v0.90.0** + oekit logx **v0.5.1** /
> bootstrap **v0.5.4**。

### Changed

- **依赖补丁批**:go-git-platform v0.89.0 → v0.90.0(transport debug 日志按级别守卫,
  被过滤时不求值响应体拷贝)、logx v0.5.0 → v0.5.1(修复 std 后端默认级别 DEBUG 的
  iota 陷阱,未设 LOG_LEVEL 的进程不再全量打 Debug)、bootstrap v0.5.3 → v0.5.4
  (`WithBeforeServe` 启动钩子,本仓暂无使用点)。生产代码零改动,`go test ./...`
  19 包 + `make lint` 全绿。

## [v1.22.0] - 2026-10-17

> 依赖 git-ferry-core **v0.10.0**(oekit 接入 + go-git-platform v0.89.0)+ go-git-platform
> **v0.89.0**。

### Changed

- **依赖升 git-ferry-core v0.9.1 → v0.10.0**(core 侧 oekit 接入与行为变更见 core
  CHANGELOG:webhook 空密钥改拒绝、锁族迁移 distlock、日志接线 logx)。
- **依赖升 go-git-platform v0.76.0 → v0.89.0**,随之 **Go 版本线 1.26 → 1.27.2**(SDK go.mod
  要求):`go.mod`、`Dockerfile`(`golang:1.27-alpine`)、`release.yml` 的 setup-go、README /
  CONTRIBUTING 的环境要求同步;`ci.yml`/`codeql.yml` 本就跟随 `go-version-file`。壳层代码零改动,
  与已发布 core v0.9.1 组合编译通过(core 源码与新 SDK API 兼容),可独立过 CI。
  实测:GitCode 平台连通性测试 0.49s 通过且能力探测齐全,仓库分页列表正常。
  **随 SDK 带入的行为变更**见 core CHANGELOG(webhook 空密钥改拒绝——壳层 `IsWebhookValidation`
  哨兵路径返 401 不变;分页触顶改报错;SDK 错误文本形状变化,壳层无文本匹配受影响)。
- **接入 oekit 基础库**（替换自写基础设施，依据 <https://oekit.github.io/llms.txt>）：
  - **启动骨架 → `oekit/bootstrap` + `oekit/hertz.RunServer`**：`serve.Run` 不再用 `oklog/run`
    手拼 run.Group，改由 bootstrap 统一信号监听、资源逆序关闭与超时预算；关闭顺序不变
    （先排空 HTTP，再执行 `cleanup` 停 watcher/core）。实测 SIGTERM 后 1s 内退出、端口释放。
  - **裸 `go func` → `oekit/concurrency/async.GoSafe`**：通知派发、心跳、AI 会话 janitor、
    AI 流式回复、run watcher（事件/轮询两路）、token 命令执行、测试假模型共 9 处统一经
    GoSafe 启动；`internal/pkg/safe.Go` 删除，`safe.RecoverToErr`（panic 转 error 回传）保留。
  - **指标 → `prometheus/client_golang` + `oekit/metrics` + `oekit/hertz/metrics`**：
    `internal/metrics` 不再手写 Prometheus 文本渲染（253 行 → 注册表薄封装），默认注册表复用
    oekit `metrics.Registry`，`/metrics` 由 `hertzmetrics.MetricsHandler()` 暴露，因而新增
    Go runtime / process 采集器与 `app_build_info`、`app_start_time_seconds`；对外 API
    （`IncCounter/AddLabeled/SetGauge/ObserveSyncRun*/ObserveHTTP/WritePrometheus`）不变。
    **抓取面两处格式差异**：`sync_runs_total` 无 task 的打点现落在 `task=""` 序列
    （Prometheus 摄入时空标签等价于不存在，PromQL 查询不受影响）；gauge 按最短浮点输出
    （如 `1.7e+09`）。
- 依赖：`cloudwego/hertz` v0.10.5 → v0.10.6（oekit/hertz 要求）；oekit 升至结构重置后的新基线
  **bootstrap v0.5.0 / hertz v0.5.3**（包提升到模块根：import `oekit/bootstrap/bootstrap`→`oekit/bootstrap`、
  `oekit/hertz/hertz`→`oekit/hertz`、`oekit/hertz/hertz/metrics`→`oekit/hertz/metrics`；hertz v0.5.1
  的测试仍引用旧 bootstrap 路径导致 `go mod tidy` 报错，v0.5.2 已修；v0.5.3 再修 v0.5.2 tag
  被重指向导致的 sumdb 校验和不一致，直接使用 v0.5.3）；concurrency v0.5.2（纯增量）；
  go-git-platform **v0.89.0**（适配 logx v0.4+ 的 `Delegating` 单参签名,间接依赖随之
  core v0.6.0 / logx v0.4.0 / pkg v0.9.3）。

### Removed

- `github.com/oklog/run`（被 oekit bootstrap 取代）。

## [v1.21.0] - 2026-10-03

> 依赖 git-ferry-core **v0.9.1** + go-git-platform **v0.76.0**。

### Changed

- **业务引擎全面下沉 core**（分层整改批次 1–3，详见根目录 `DESIGN.md`）：
  - metadata 备份/回灌引擎 → core `BackupMetadata`/`RestoreMetadata`；
    壳 `metadata_service.go` 327→110、`metadata_restore.go` 505→77、`metabackup.go` 删除。
  - health 评分引擎整包迁 core（`internal/health` → `git-ferry-core/health`），
    评分走 `Service.HealthSnapshot`。
  - trends / todo / 资产盘点 → core `OpsTrends`/`OpsTodo`/`RepoInventory`。
  - 模板库与预览套用 → core（`Service.Templates/PreviewTemplate/ApplyTemplate`）。
  - force-push 审批存储 → core `ForcePushStore`，执行器默认回调 core 存储，
    壳只留 HTTP 端点与操作者身份。
  - 组织导入/组织镜像编排 → core `ImportPublicOrg`/`BulkMirrorOrg`/`ListStarredRepos`。
  - 密钥生成 → core `pkg/deploykey`。
  - 壳 `biz/handler/git_sync` **8263 → 6609 行**（-1654），`internal/` 下
    `health`/`orgmap`/`tpl` 三包迁出。
- **runwatch 改事件驱动**：默认订阅 core `SubscribeRuns` 完成事件（`runwatch.mode:
  event`），轮询保留为兜底（`mode: poll`）；失败自动重跑次数/冷却判定下沉 core
  `RetryTracker`。
- **错误映射统一**：新增 `response.FromError`（按 core `Classify` 选状态码），
  7 处 `errors.Is(Err*NotFound)` 分支收敛。
- **装配时序**：`main.go` 改 `corebridge.NewService(cfg, WithProviderHooks(...))`，
  不再依赖「先调 InstallProviderHooks 再 NewService」的隐式顺序。
- **依赖**：go-git-platform v0.75.0 → **v0.76.0**（本地与发布态对齐）。
- 旧名清洗：文档/注释/脚本中 `git-sync-core`/`git-sync-service`/`git-sync-intranet`
  → `git-ferry*`；`script/bootstrap.sh` 并入 `scripts/` 并修正失效的二进制名。

### Removed

- `internal/health`、`internal/orgmap`、`internal/tpl`（业务规则迁 core）。
- 壳内 `ShellForcePushApprover` 的文件读写实现（改调 core 存储）。
- `metabackup.go` / `org_mirror_helpers_test.go`（逻辑与断言随迁 core）。

### Fixed

- **组织导入/组织镜像「建任务」恒失败**：core `CreateTask` 的分支名校验拒绝 `*`，
  而编排与 executor 都按 glob 语义使用 `SourceBranch: "*"`（core 侧已放行 `*?[]`）。
- 健康评分/盘点等 N+1 查询改由 core 批量化接口承接（`HealthSnapshot` 保留原有
  8 并发 fan-out 语义）。

### Changed（续）

- **`biz/serve` 日志与失败退出转发 core `pkg/proclog`**：消除与内网壳的逐字重复
  （`SetupLogger`/`ExitOnFail` 保持原导出名，调用方零改动）。

### Tests

- 新增 `metadata_http_test`、`org_bulk_http_test`（响应逐字 JSON + 错误文案锁定）；
  壳全量 `go test ./...` 与 `make lint` 全绿；`cmd/apidoc` 重新生成后
  `docs/openapi.json` **零差异**。

## [v1.20.4] - 2026-10-02

> 依赖 git-ferry-core **v0.8.1** + go-git-platform **v0.75.0**。

### Changed

- **分页迁移到平台 v0.73 收敛后的唯一分页面**：issues 导出、gists 备份改
  `provider.EachBounded` + 到上限即 `ErrStopIteration`（`ListAllPages` 已被
  平台删除）。空页终止语义：短页≠末页（防服务端压缩页大小时提前停），
  页预算 +1 页用于观测空页。
- **CI**：`actions/setup-node` v4→v7（消除 Node 20 运行时弃用警告），
  构建 Node 20（EOL）→ 22。

## [v1.20.3] - 2026-10-02

> 依赖 git-ferry-core **v0.8.0**（全面收口 go-git-platform + `PushTaskBackup` 新增）
> + go-git-platform **v0.72.0**（可选能力 Gists/Starred/Migrations/ReleaseAssets、
> 泛型分页 `ListAllPages`、`githubapp`、403 限流重试）。

### Changed

- **provider 收口**：`newIssueProvider` 改经 core `ProviderForPlatform`（Manager 缓存 +
  GitHub App installation token 解析，repo token 非空优先）；删除 metadata 附件/gists
  处手写 token 回退与 `sdkprov.Config+NewProvider` 样板——GitHub App 平台此前拿
  `plat.AccessToken`（空）调 API 的错误行为随之修正。
- **限流指标 hook 化**：`internal/githubapi.Throttler` 的 `gitferry_api_ratelimit_total`
  打点迁到 `service.SetProviderHooks` 响应钩子（main 启动早期装配），429 或
  403+`X-RateLimit-Remaining:0` 即计数（含重试各次尝试）；退避本身由平台 transport
  接管。
- **删除 `internal/githubapi` 包**（手写 HTTP/退避/分页已被平台覆盖）；`IsGitHub`
  （含死掉的 "ghe" 分支）删除，调用点改 `plat.Type == corebridge.PlatformTypeGitHub`
  或 `Capabilities()` 能力门控。
- **分页收口**：gists 备份、starred 导入、组织公开仓列取、issues 导出全部改
  `ListAllPages`/页循环；gists 备份**修复只拉 1 页（≤100）却标 maxGists=200** 的
  bug，现分页拉到上限为止；附件下载复用已收集的 `ReleaseInfo.Assets` 不再重复请求。
- **migration 平台化**：CreateMigration/GetMigration 迁到 `MigrationManager`
  （org=="" 用户级），wait 轮询与响应字段/审计文案不变；请求经平台 transport 获得
  429/5xx/限流 403 重试（原 `http.DefaultClient` 无超时无重试）。
- **backup push 带凭证**：`POST /ops/push-backup` 删裸 `exec git push`，改
  core `PushTaskBackup`（临时 remote + gitbackend + `BuildRepoAuth` 凭证），
  https 私有仓可推送；dry-run/参数校验/审计/响应 JSON 不变，错误文案近似保留。

### Fixed

- 组织公开仓导入：`RepoManager.ListRepos` 客户端过滤 `!Private`（原 `type=public`
  只拉公开），拉满 max 条公开仓为止；无 token 时仍可匿名列（ProviderForPlatform
  空 token 语义不变）。

## [v1.20.2] - 2026-10-01

> 依赖 git-ferry-core **v0.7.2**（认证路径重构）+ go-git-platform v0.68.2。

### Changed

- **认证路径重构**（依赖 go-git-platform credential helper 改造 + core `executor/auth.go`）：
  HTTPS 令牌经临时 credential helper / GIT_ASKPASS 注入 git，**不进 argv / environ 明文**；
  临时凭证目录 RAII 清理。SSH 密钥内容同样经 0600 临时文件 + `GIT_SSH_COMMAND`。
- **README 改版**：安装/快速开始重写为 npm 分发优先，补 Prerequisites 与
  AI Agent Quick Start，中英文同步。

### Fixed

- **服务启动必现 panic**：`conf/config.yaml` 缺省 `notify` 段时 `shellCfg.Notify`
  为 nil，取 `Heartbeat` 字段前未判空（`notify.New`/`NewHeartbeat` 本身已 nil-safe）。
- **CLI 全部命令 panic**：`opsMetadataRestoreCmd` 重复定义 `sc()` 已挂的
  `target-repo`，pflag `flag redefined` 使 `config init`/`auth status` 等全不可用。
- **gosec G703 误报**：gitserve 取仓路径处 taint 分析未识别前缀校验（该处已有
  `path.Clean` + 拒 `..` + `absRoot` 前缀三层防护）。

## [v1.20.1] - 2026-10-01

> 依赖 git-ferry-core v0.7.1（ForcePushApprover 注入）。

### Added

- **core v0.7.0 接入**：分支过滤 `include_branches`/`exclude_ref_patterns`
  （converter/create/update + 任务向导表单）。
- **备份远端推送** `POST /ops/push-backup` + CLI `ops +push-backup`（github/gitlab git remote）。
- **workdir 浏览** `GET /ops/repo-files` + CLI `ops +repo-files`（防路径穿越）。
- 首启向导平台链接修正；goreleaser brews/scoops；OpenAPI 85 paths。


### Added

- **force-push 审批闭环**：executor 注入 `ForcePushApprover`；`block` 策略分歧时
  自动登记 pending，Admin 放行后放行覆盖（core `SetForcePushApprover` + 壳层适配器）。
- **备份远端 push**（P2.2）：`sync.backup_remotes[]` 同步成功后向 GitHub/GitLab 等
  push 镜像副本（`{owner}/{repo}` 占位）；与开源发布区分，不改写 module 身份。
- **同步趋势** `GET /ops/trends?days=N` + CLI `ops +trends`；前端可接小图。
- **metrics 按任务标签** `sync_runs_total{status,task}`。
- **DR 演练验元数据**：`dr-drill?with_metadata=true` 抽样比对快照清单/issues 分片。
- **AI 工具** `get_metadata_snapshots`（只读；回灌仍走 CLI）。
- **CLI** `ops +git-url` 打印 Git Smart HTTP clone URL。
- **配置启动校验**：`backup_format` 枚举 / `backup_keep` 范围（对齐 schema 子集）。
- **首启向导**：仪表盘无仓库无任务时展示三步引导。

## [v1.19.4] - 2026-10-01

### Changed

- **README 全面重写**（中英双语对齐）：能力总览、快速开始、运维与灾备矩阵、
  Agent 三接入面（CLI/MCP/Skills）、元数据恢复闭环、Git Smart HTTP 示例、
  配置样例与架构图；链接指向规划/调研/示例模板。

### Fixed

- 再次移除 `org-mirror` 路径中对未发布 core 字段 `IncludeBranches` 的引用，修复 CI。

## [v1.19.3] - 2026-10-01

### Added

- **org 映射预览** `POST /ops/resolve-org-target` + CLI `ops +org-map`
  （preserve/single/flat/mixed，`internal/orgmap` 纯逻辑+单测）。
- **公共组织导入** `POST /ops/import-public-org`（可选建任务）；
  与已有 `org-mirror` / `import-starred` 路由对齐。
- **force-push 审批流** `ops/force-push-approvals`（申请/列表/Admin 放行）。
- **post-exec 钩子** `sync.post_exec_script`（GITFERRY_TASK/RESULT/RUN_ID/TRIGGER）。
- **冷备 zip 形态** `sync.backup_format: bundle|zip`（keep 轮转两者皆支持）。
- **配置 JSON Schema** `conf/config.schema.json`。
- **分发与 CI 模板** `examples/packaging/{homebrew,scoop,nix}`、
  `examples/ci/github-actions-mirror.yml`。
- OpenAPI 收录 org 相关端点（81 paths）。



> 对标开源（gickup / ghorg / gitea-mirror / github-backup-rust）缺口批次。
> 规划：`docs/superpowers/plans/2026-10-01-competitor-gap-roadmap.md`
> （该过程文档已随历史文档归档移除，此处保留出处）。

### Added

- **元数据 Restore（P0）**：`POST /api/v1/ops/metadata-restore`（Admin，**默认 dry-run**）
  - labels → milestones → issues（含评论/关闭）→ PRs（issue 形态标注）→ releases
  - CLI `ops +metadata-restore --key <repo> [--kinds …] [--execute] [--overwrite]`
  - 前端冷备页「回灌」入口；历史写 `backup_dir/metadata-restore/<repo>/`
- **平台 API 限流退避（P0.2）**：403/429 + Retry-After / X-RateLimit 指数退避
  （1s→60s，最多 5 次）；指标 `gitferry_api_ratelimit_total`
- **分支过滤 + 忽略 PR refs（P1.3）**：任务字段 `include_branches` / `exclude_ref_patterns`
  （默认排除 `refs/pull/*`、`refs/merge-requests/*`）；CLI 对应 flags
- **MCP Server（P1.1）**：`POST /mcp` Streamable HTTP，与 eino 工具同表
  （initialize / tools/list / tools/call）；`GET /mcp` 探测
- **Git Smart HTTP 只读（P2.1）**：`git_serve` 配置段，局域网/灾备 `git clone`
- **元数据增量（P3）**：`metadata-backup?since=RFC3339`

## [v1.19.2] - 2026-10-01

### Fixed

- 去掉对未发布 `git-ferry-core` 字段（`IncludeBranches`/`ExcludeRefPatterns`）的引用，
  恢复基于 `git-ferry-core v0.6.1` 的 CI 构建。

## [v1.19.1] - 2026-10-01

### Added

- **只读 Git Smart HTTP**（`internal/gitserve`）：内网/灾备场景 `git clone` 冷备或 workdir 的 bare 仓。
- **MCP 端点**（`internal/mcp`）：把 agent 工具表暴露给 Claude Code / Cursor 等外部客户端。
- **GitHub API 节流**（`internal/githubapi/throttle`）：速率控制 + metrics。
- Playwright 截图回归脚本 `scripts/e2e-screenshots.js`（`make e2e-shots`）+ 关键页截图
  （login/dashboard/ops/devhub/health/templates）。
- 测试补齐：`OpsTodo`/`HealthScore` 响应结构与空态、模板 `extends` 链、
  CLI client 分页、config 读写与 env 覆盖、批量 CSV 转义。

### Fixed

- 批量结果 CSV 对含逗号/引号的 error 字段做转义（原先可能破坏 CSV 结构）。
- `prBody` 改用 `fmt.Fprintf`，清掉 staticcheck QF1012。
- 前端统一走 `notify*` toast（静态 `message` 在 AntD 4 不可靠）：仪表盘复制、DeveloperHub。
- HealthScorePanel 样式对齐设计 token；待办/建议动作支持复制命令、危险标记。
- 健康 attention 按分升序；模板 `extends` 成环错误信息带上完整链路。
- DeveloperHub Release 安装命令改为变量版本号 + Releases 链接，不再写死旧版本。

### Changed

- `goreleaser` 明确为可选路径（默认发版仍走 `release.yml`）；Makefile 增加
  `e2e-shots` / `goreleaser-snapshot`。

## [v1.19.0] - 2026-09-30

> 对照 gitlink-cli 的 Agent-Native / 开源工程化批次 + Scorecards/Renovate 业务深化。
> 依赖 `git-ferry-core v0.6.1`。新增 `github.com/spf13/cobra v1.10.2`（CLI）。

### Added

- **`gitferry` CLI**（`cmd/gitferry` + `internal/cli`）：
  - Shortcuts：`repo/task/history/ops/platform/webhook` 域（`+list/+info/+run/+batch-run/...`）
  - Raw API `gitferry api METHOD PATH`、Schema 自省 `schema list/show`（内嵌 OpenAPI）
  - 统一 Envelope `{ok,data,error,meta}`，`--format json|table|yaml`
  - `--all` 全量分页、`--dry-run` 批量预览、`--with-drift` 健康折入漂移、`--csv` 导出
  - 危险操作 409 确认门（脚本 `--yes`）；`config init/get/set/list/path`
  - macOS Keychain 存 API Key（文件 0600 回退）
- **Agent Skills**（`skills/gitferry-*`）：shared/repo/task/history/ops/workflow，
  含 troubleshooting、flags 参考；`make install-skills` / npm `gitferry-install-skills`
- **Web「CLI / Agent 入口」**（`/settings/dev`）：安装/认证/命令速查/实时连通
- **健康评分维度化（Scorecards）**：reliability/freshness/schedule/safety/completeness
  五维 + reason + 可复制动作（`health.RouteActions`）；`/ops/health-score` 返回
  `dimensions/action_items/attention/top_actions`；`with_drift` 折入实测漂移
- **统一待办**：`GET /api/v1/ops/todo` + CLI `ops +todo` + AI `get_ops_todo`；
  仪表盘运维待办卡片（P1/P2 + 动作一键复制）
- **策略模板继承**：`Template.Extends` 链式合并（子覆盖父）、环检测，
  apply 返回 `effective_spec`/`extends_chain`；Templates UI 可选继承
- **token_cmd 密钥注入**：`GIT_SYNC_TOKEN_CMD_<KEY>`（secrets manager）→ env → 配置
- **审计 CSV**：`ops +audit-report --csv`、`task +batch-run --csv`
- **开源工程化**：双语 README、CONTRIBUTING/SECURITY/Issue&PR 模板、
  npm 分发包（`gitferry-cli`）、`.goreleaser.yaml`、Release 产出 CLI 六平台压缩包、
  验收清单与相邻工具调研（`designs/research/ops-ux-patterns.md`）
- **AI 工具**：`get_ops_todo`；`deep_analyze` 输出 `weak_dims` 与维度驱动 next_action
  （工具总数 30）

### Changed

- 健康/历史采集：成功率、连续失败、步骤级失败、重试次数、主导错误类型
- 健康计算并行拉历史（消除 N+1）；仪表盘露出薄弱维度
- OpenAPI 收录 `/ops/todo` 等（78 paths）；lint/tidy/gofmt 收口

## [v1.18.0] - 2026-09-29

> 依赖 `git-ferry-core v0.6.0`。对标 gickup / gitea-mirror / ghorg 的 P0–P5 演进批次。

### Added

- **P0 灾备闭环**:
  - DR 演练 `POST /ops/dr-drill`(单个/批量):恢复到临时目录 → `git fsck` → refs 比对 → RTO 观测;
    历史 JSONL 哈希链(`GET /ops/dr-drill/history`、`/dr-drill/chain/verify`)。
  - 备份完整性证明:SHA256 + Merkle Root 清单(`POST /ops/backup-manifest`、`GET /ops/backup-manifest/verify`)。
  - RPO/RTO 观测 `GET /ops/rpo`:按任务暴露最近备份时长、超标标记、估算恢复时间。
- **P1 元数据资产**: `POST /ops/metadata-backup` 快照 issues(+评论)/PR/labels/milestones/releases,
  可选下载各 tag 的 source archive;`GET /ops/metadata-backups` 列表。
- **P2 多目的地扇出**: `sync.backup_destinations`(s3/webdav/azure/local),与 `backup_s3` 并存;
  同步成功后自动加密(可选)并逐目的地上传,失败互不阻断。
- **P3 生命周期**:
  - `POST /ops/auto-discover` 平台新仓库发现/导入。
  - `POST /ops/drift` 本地与目标远端分支漂移检测。
  - 任务字段 `force_push_policy`:`allow | block | backup_on_demand`(覆盖前自动打回滚快照)。
  - `POST /ops/backup-cleanup` 按 `backup_retention_days` 清理;`legal_hold` 时拒绝。
- **P4 认证治理**:
  - RBAC `admin|operator|readonly`;写操作 `WriteGuard`、治理操作 `AdminGuard`。
  - OIDC/JWT Bearer(HS256)可选鉴权 `auth.oidc`。
  - 审计哈希链 `prev_hash/entry_hash` + `GET /ops/audit-chain/verify`。
  - 平台表新增 GitHub App 字段(app_id / installation_id / private_key)。
- **P5 存储效率**: 冷备 AES-256-GCM(`sync.backup_encrypt_key`),`.bundle.enc` 恢复自动解密;
  retention + legal_hold;前端运维中心新增「灾备演练」页签。
- **GitHub App 签发**: RS256 JWT + installation access token(缓存 55min),配置了
  `github_app_id/installation_id/private_key` 时优先于 PAT;私钥加密入库。
- **Release 二进制附件**: `metadata-backup` 可下载 GitHub Release assets(`with_assets`);
  **Gists 备份** `POST /ops/gists-backup` 或快照 `with_gists`。
- **前端**: 任务表单「强制推送保护」三策略;冷备页「元数据快照」;灾备页「漂移检测」。

- **AI 模型发现**: `GET /api/v1/ai/models` + 设置页「获取模型」，从端点拉取真实模型列表。
- **仪表盘运维健康**: 平均分、金银铜/基础分布、待改进项数，直达运维中心。
- Webhook 列表表格式内容卡；清理残留旧页头样式。

## [2026-09-29] - 前端重设计与 AI 配置

### Added

- **前端全面重设计**（GitHub/Linear 式冷静运维台）:设计 tokens + AntD 主题、
  侧栏分组 IA、统一 `PageHeader` / `MetricStrip`、仪表盘失败任务「需要关注」。
- **AI 助手配置界面**（系统 → AI 助手）:服务预设（OpenAI/DashScope/DeepSeek/Ollama/vLLM）、
  Base URL、模型选择、API Key、温度/Max Tokens/超时；保存写 `data/ai-settings.json` 并热重建 Runner。
- **AI 配置 API**: `GET/POST /api/v1/ai/config`、`POST /api/v1/ai/config/test`（密钥脱敏回传）。
- **AI 面板重设计**:空态建议问题、工具 chip、危险操作确认卡、composer 输入区。
- **文档与截图更新**: `docs/screenshots/` 新版 UI 全套;README / frontend README / designs 对齐现状。

### Changed

- 登录页品牌统一为 GitFerry（去掉紫色渐变）。
- `getAIStatus` 兼容 `{code,data}` 与裸 JSON，避免误判「未启用」。
- AI 工具事件文案改为「调用中… → 完成/失败摘要」。

## [2026-09-27] - 运维与备份能力批次

### Added

- **运维中心**(侧栏): 概览 / 健康评分 / 资产盘点 / 策略模板 / 部署密钥。
- **可观测**: `GET /metrics` Prometheus;HTTP 低基数路径指标。
- **通知矩阵**: ntfy / gotify / success-fail heartbeat / 通用 HMAC 签名 webhook。
- **失败补偿**: runwatch 轮询 + 自动重跑;`POST /ops/retry`、`/ops/retry-batch`。
- **健康评分** `GET /ops/health-score` (gold/silver/bronze/basic,可配置规则 DSL)。
- **资产盘点** `GET /ops/inventory`(孤儿仓库)。
- **策略模板** CRUD + preview + apply(dry-run);`data/templates.json`。
- **审计导出** `GET /ops/audit-report?format=csv`。
- **过滤导入** `POST /ops/sync-platform`(archived/fork/star/language/glob)。
- **GitHub Migration 全量归档** `POST /ops/migration`。
- **Issues 导出** `GET /ops/issues-export`。
- **部署密钥** `POST /ops/deploy-key`(Ed25519,私钥仅返回一次) + UI。
- **一键重建** `POST /ops/rebuild`。
- **任务高级选项**: `git_lfs` / `sync_wiki` / `git_bundle` / `submodules` /
  `git_push_prune` / `keep_divergent`。
- **密钥注入** `GIT_SYNC_TOKEN_<NAME>`。
- **配置** `sync.backup_dir/backup_keep/partial_clone/backup_s3`、`runwatch`、`notify`。

### Security

- 确认令牌 crypto/rand 128bit;危险工具放行标记一次性消费。
- 工具输出 json.Marshal,消除 `%s` 拼 JSON。
- `conf/config.yaml` 移出 git;弱密钥黑名单。
- 默认 API Key 鉴权写入 `api-key:<指纹>`,杜绝 X-User 冒充。
- SSH 主机指纹钉扎 UI。

### Fixed

- webhook 按 IP 限流;GetSyncService 判空;GetRepo 未命中 404。
- Dashboard 不再污染共享 store;仓库搜索竞态;静默失败提示。
- PlatformSettings 947 行拆分;any/catch 类型收敛。
