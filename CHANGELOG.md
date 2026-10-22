# Changelog

GitFerry — 自托管 Git 同步/镜像/备份中枢。本文件记录壳层发版变化。

## [Unreleased]

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
> 规划：`docs/superpowers/plans/2026-10-01-competitor-gap-roadmap.md`。

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
