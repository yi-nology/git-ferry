# 开源双仓同步:完整历史镜像发布 需求与设计

> 2026-10-22 · 状态:待评审
> 形态:git-ferry 新增「双仓同步」通道模式(git-ferry-core 扩展 + 壳层 API + CLI/MCP + Agent Skills)
> 关联:`2026-09-16-mirror-publishing-design.md`(镜像中心前端设计)、`git-ferry-core/mirror`(身份改写基座)
> 首个落地仓:`git.enjoye.top/enjoydream/code-review-service` → `github.com/yi-nology/code-review-service`

---

## 0. 背景与现状(实测)

### 0.1 问题的真实形态

内网与公网双树不是「同一个仓库加两个 remote」,而是**两个各自有完整历史的独立仓库**:

| | 内部树 | 公开树 |
|---|---|---|
| 路径 | `~/my_project/review-service` | `~/my_project/code-review-service` |
| module path | `git.enjoye.top/enjoydream/code-review-service` | `github.com/yi-nology/code-review-service` |
| remote | `ssh://git@git.enjoye.top/enjoydream/code-review-service.git` | `origin` → 本地裸镜像<br>`github` → GitHub 真上游 |
| commit 数 | 408 | 406 |
| tag | `v0.2.0` | `v0.1.0` `v0.2.0` `v0.2.1` |
| 独有提交 | readiness 探测 / swagger 重生成 | Gerrit 移植 / release CI / 开源目录卫生 |

两侧提交 **subject 序列逐条对应**(最近 15 条几乎一一对上),仅在「开源仓目录卫生」这类条目上措辞不同——证明同源,但已双向分叉。

### 0.2 路径改写的真实规模

以 `review-service` 实测:

| 项 | 数量 |
|---|---|
| `go.mod` 的 `module` 行 | 1 |
| 唯一 import 目录 | 98 |
| import 引用总数 | 688 |
| 涉及手写 `.go` 文件 | 30+ |
| `biz/model/*` 生成物 | **0**(不含模块路径) |
| `idl/*.thrift` | **0**(不自引用) |

最后两行是关键:**thriftgo 生成物与 IDL 不含模块路径,改写不需要重新生成**。改动只落在手写代码的 import 上。

### 0.3 现有手工作法的三个缺陷

1. **改写用 `sed`/`ReplaceAll`,无边界保护**。朴素替换会把 `github.com/a/b-extra` 中的 `github.com/a/b` 也换掉,产出语义错误的 module path。
2. **改写后校验靠人肉**。模块路径前缀变化会打乱 import 分组字典序,`gofmt` 报 diff——公开树 CHANGELOG 已明确记过这个坑。
3. **内网痕迹清理靠人工扫**。AGENTS.md 里的 `harbor.enjoye.top`、215 环境、内部网关名必须从公开树剥除,无门禁。

### 0.4 已具备的基座能力(可复用,不必重写)

`git-ferry-core/mirror` 已解决「身份改写」这块最硬的骨头:

- **边界安全改写**(`mirror/gitobj.go:23`,边界正则在 `:21`):正则 `([^a-zA-Z0-9_.\-]|$)` 限定替换边界,子包路径正确改写、兄弟路径不误伤。
- **类型检查门禁**(`mirror/gate.go:18`):`golang.org/x/tools/go/packages` 全量类型检查(`GOWORK=off`),语义级而非文本级。
- **全链路编排**(`core/service/mirror_service.go`):通道 CRUD → 克隆 → 改写 → 物化 → 门禁 → 远端 tree 一致性比对 → 合成 → 强推 → 验证。
- **确定性保证**:合成 commit 继承源 commit 作者/时间戳,同 tag 重跑 hash 完全一致。

---

## 1. 目标与非目标

### 1.1 目标

1. 把「内部树 → 公开树」的路径改写、门禁校验、痕迹清理**从人工流程变成可执行门禁**。
2. 支持**完整历史**同步(公开树保留自己的 commit 序列),而非仅 tag 快照。
3. 公开树的**任何一次推送都必须可审计**:哪些改动被同步、哪些被剥离、哪些待人工确认。
4. AI 承担**语义判断**(改动能否公开 / 痕迹要不要脱敏),确定性代码承担**机械变换**(路径改写 / 类型门禁)。

### 1.2 非目标

1. **不做通用双向同步**。本模式是「内部树单向发布到公开树」,公开树可以有自己的提交(Gerrit 移植等),但不会反向流回内部树。跨平台 remote↔remote 同步由既有 sync 引擎负责,不在此范围。
2. **不替代 `git-ferry-core/mirror` 的 publish 模式**。publish 模式解决「同一模块两个身份都可 `go get`」,本模式解决「两个各自有完整历史的仓库」。两者语义不同、并存。
3. **不做代码内容翻译**。只改身份标识与配置引用,不改业务逻辑、不做 i18n。
4. **不在本需求内做 10 个仓的全量推广**。首期只做 `review-service` 跑通,推广是后续独立事项(见 §7)。

---

## 2. 与既有 mirror publish 模式的关系(边界澄清)

两者容易混淆,必须先划清:

| | 既有 publish 模式 | **本需求:双仓同步模式** |
|---|---|---|
| 目的 | 同一模块两个身份都可 `go get` | 两个各自有完整历史的仓库 |
| 目标形态 | 每 tag 一个**孤儿快照**(无父提交) | **完整 commit 序列**,保留公开树历史 |
| 历史关系 | 各版本互不依赖,可重跑任一历史 tag | 公开树保留自己的提交序列 |
| tree 一致性 | 同 tag 重跑 tree 必须逐字节一致 | 按提交推进,允许公开树有额外提交 |
| tag | 源 tag 全量映射 | 两树 tag 可不一致(实测:公开树多 v0.2.1) |
| 目录差异 | 不涉及 | **需要**(实测:`docs/internal/` ↔ `docs/archive/`) |
| 典型用途 | 依赖方免改 import | 开源发布 + 保留发布历史 |

**实现取向**:复用 `core/mirror` 的**改写器**(`rewriteTree`/`rewriteModulePath`)与**门禁**(`typeCheckGate`),但**不复用其"合成孤儿 commit"的发布模型**。双仓同步需要的是「在目标树上叠加提交」,而非「在目标树上放快照」。

---

## 3. 核心数据模型

### 3.1 双仓通道(DualRepoChannel)

一条「内部源仓 → 公开目标仓」的同步配置:

```
DualRepoChannel
  ├─ source        内部仓(remote + 本地克隆路径)
  ├─ target        公开仓(remote,支持多目标:GitHub / 裸镜像留底)
  ├─ identityMap   身份映射(模块路径 / 仓路径 / 域名)
  ├─ rewriteRules  改写规则集(有序,见 §3.3)
  ├─ pathRules     路径规则(重命名 / 排除 / 必改)
  ├─ scrubRules    痕迹清理规则(正则 + 替换)
  └─ aiPolicy      AI 判定策略(见 §5)
```

### 3.2 身份映射(IdentityMap)

需要声明式表达的映射类型:

| 类型 | 源 | 目标 | 示例 |
|---|---|---|---|
| `module` | `git.enjoye.top/enjoydream/code-review-service` | `github.com/yi-nology/code-review-service` | Go module path |
| `repoPath` | `enjoydream/code-review-service` | `yi-nology/code-review-service` | 仓内路径引用 |
| `host` | `git.enjoye.top` | `github.com` | 仓地址 |
| `internalHost` | `harbor.enjoye.top` | *(剥离)* | 内网 registry,公开树不出现 |
| `literal` | `215` | *(剥离或替换)* | 环境标识 |

**注意**:`internalHost` 的目标是「不出现」而非「替换为某值」。这类规则与前四类语义不同,必须区分建模——混在一起会导致「剥不掉」和「剥成占位符」两种错误混同。

### 3.3 改写规则集(RewriteRules)

有序规则列表,按类型分派到不同执行器:

```
RewriteRules
  ├─ moduleRewrite[]    → 复用 core/mirror 的边界安全改写器
  ├─ pathRewrite[]      → 文件路径映射(docs/internal → docs/archive)
  ├─ contentRewrite[]   → 字面量/正则内容替换
  └─ scrubRules[]       → 痕迹检测与剔除(检测为主,替换为辅)
```

**排序要求**:module 改写必须在 content 改写之前完成。否则先替换了内网域名,模块路径的前缀已被破坏,边界改写失效。

---

## 4. 功能需求

### F1 同步计划生成(Plan)

**输入**:内部树的一个提交范围(或一个 tag)。
**输出**:一份**可审阅的同步计划**,描述每个提交的处置方式。

每个提交标注为以下之一:

| 处置 | 含义 |
|---|---|
| `port` | 直接移植(仅需身份改写) |
| `port_with_scrub` | 移植但需内容清理(命中有 scrub 规则) |
| `exclude` | 不移植(命中排除规则,如仅内网部署文件) |
| `hold` | **待人工确认**(AI 判定不确定,或 diff 复杂度超阈值) |

**验收标准**:
- 计划生成是**纯读操作**,不写目标仓、不改工作区。
- 同一输入两次生成的计划**完全一致**(确定性)。
- 每个 `hold` 必须附带**触发原因**(哪条规则 / AI 的哪条理由)。

### F2 差异判定(Delta)

判定内部树相对公开树的领先提交,并按 subject 匹配历史中已同步的提交。

**验收标准**:
- 能正确识别「两边都有但 hash 不同」的同源提交(路径改写必然导致 hash 变化,不能靠 hash 匹配)。
- 匹配键为 `(subject, author, authorDate)` 三元组;冲突时降级为 `(subject)` 并标记歧义。
- 输出 `已同步 / 领先 / 公开树独有` 三类。

### F3 身份改写(Rewrite)

复用 `core/mirror` 的 `rewriteTree` + `rewriteModulePath`,**不做二次实现**。

**验收标准**:
- 边界用例:`github.com/a/b/pkg` 正确改写;`github.com/a/b-extra` **不被误改**。
- `biz/model/*` 与 `idl/*.thrift` 不被改动(实测不含模块路径,应为零命中)。
- 改写后自动执行 `gofmt`,import 分组必须符合 gofmt 规范(**这是实测踩过的坑**)。
- 8MB 以上 blob 与二进制文件原样保留。

### F4 类型门禁(TypeCheckGate)

复用 `core/mirror/gate.go` 的 `typeCheckGate`。

**验收标准**:
- 门禁在**推送前**执行,不通过则中止且**不产生任何推送**。
- 门禁在 `GOWORK=off` 下运行。
- 错误信息定位到 `file:line:col`。

### F5 痕迹检测与清理(Scrub)

对改写后的待推送内容做检测,命中即标记该提交为 `port_with_scrub` 或 `hold`。

**默认识别目标**(可配置):

| 类别 | 模式示例 |
|---|---|
| 内网域名 | `*.enjoye.top`、`git.enjoye.top`、`harbor.enjoye.top` |
| 密钥形态 | `Bearer `、`sk-`、`ghp_`、`api_key=` |
| 环境标识 | 内部环境名、`215` 环境 |
| 内网地址 | `10.x` / `100.64.x` / `192.168.x` 段 |
| 客户标识 | 客户仓名 / 客户域名(需配置) |

**验收标准**:
- 检测在**推送前**完成,产出**逐行定位**的报告(文件:行号:命中内容)。
- 命中密钥类 → **强制 hold**,不自动替换(自动替换密钥可能留下半吊子状态,比留痕更危险)。
- 命中内网域名类 → 可按规则自动替换或剔除,但**必须在计划里列出**。
- 公开树的 `AGENTS.md` 等文档文件纳入检测范围(实测公开树 AGENTS.md 不得含内部主机名)。

### F6 提交移植(Apply)

把计划落到目标树。

**验收标准**:
- 默认**逐提交移植**,保持提交序列与作者身份/时间戳。
- 公开树独有提交**原样保留**(实测 Gerrit 移植、release CI 属此类)。
- 移植后重算新 hash(路径改写必然改 hash),新 commit message **保留原 subject** 并追加改写说明。
- 冲突时**不自动解**,进入 `hold` 并产出冲突报告。

### F7 双远端推送(Push)

支持目标侧多远端:GitHub 真上游 + 本地裸镜像留底。

**验收标准**:
- 推送顺序可配置,默认**先留底后发布**。
- 任一远端失败时**不推送其余远端**(避免半发布状态)。
- 推送使用 `gitbackend`,不执行动态参数子进程(与 core/mirror 的安全约束一致)。
- 推送前**必须**通过 F4 门禁与 F5 检测。

### F8 预检与确认(DryRun / Confirm)

**验收标准**:
- 预检执行到「远端 tree 比对」为止,不推送。
- 预检产出**完整差异报告**:将推送的提交数、改写文件数、清理命中数、待人工确认数。
- 危险操作(强推、覆盖已有 tag)需显式确认,确认前展示影响面。

### F9 审计与回滚(Audit / Rollback)

**验收标准**:
- 每次同步留存:计划快照、门禁结果、检测报告、推送前后 ref 对照、commit 映射表。
- 支持按同步记录**回滚目标树到指定 ref**(裸镜像留底提供保障)。
- 审计记录含**哈希链**,与 git-ferry 既有审计机制一致。

### F10 Agent 接入(CLI / MCP / Skills)

沿用 git-ferry 既有接入面,不另造。

**验收标准**:
- CLI 有 `dual-repo` 命令组:`plan` / `preview` / `apply` / `status` / `rollback`。
- MCP 暴露对应工具,输出走既有 Envelope 格式。
- Agent Skill 提供操作手册,含「预检→审计划→执行」的标准流程。

---

## 5. AI 辅助的定位(边界必须写死)

**AI 只做语义判断,不做机械变换。**

| 环节 | 执行者 | 理由 |
|---|---|---|
| 路径改写 | 确定性代码(复用 core/mirror) | 文本变换,LLM 不可靠且不可复现 |
| 类型门禁 | 确定性代码(go/packages) | 语义级检查,LLM 无法替代 |
| 痕迹检测 | 规则优先 + AI 补充 | 正则能抓的确定性抓,AI 补语义型 |
| **改动能否公开** | **AI** | 需理解 diff 意图,规则做不到 |
| **痕迹要不要脱敏** | **AI** | 需判断上下文语义 |
| **commit message** | AI 生成 + 人工确认 | 既有能力,直接复用 |
| **冲突解** | AI 辅助 + 人工确认 | 既有「AI 冲突解决」能力可复用 |

**AI 判定的输出必须可解释**:每条 `hold` 要给出理由与依据的 diff 片段,不能只给结论。审计要求可追溯。

**AI 不参与**:路径改写结果、门禁结论、检测命中项。这三类的正确性由确定性代码保证,LLM 参与只会引入不确定性。

---

## 6. 验收标准(端到端)

以 `review-service` 为首个验证对象:

| # | 验收项 | 判定方式 |
|---|---|---|
| A1 | 内部树 408 与公开树 406 的提交差异被正确分类 | 计划输出与人工核对一致 |
| A2 | 98 个 import 目录全部改写,零误伤 | 与 `git-enjoye-top` 反查,兄弟路径不被改 |
| A3 | 改写后 `gofmt -l` 输出为空 | 命令行直接判定 |
| A4 | 类型门禁通过 | `make test` + 门禁日志 |
| A5 | 公开树无 `*.enjoye.top` 残留(除历史说明) | 扫描报告 |
| A6 | 公开树独有提交(Gerrit 移植等)原样保留 | commit 映射表核对 |
| A7 | 目录改名 `docs/internal/` → `docs/archive/` 生效 | 文件树比对 |
| A8 | 预检不产生任何推送 | 远端 ref 对照无变化 |
| A9 | 同一输入两次生成的计划完全一致 | 计划 diff 为空 |
| A10 | 公开树 `go.mod` 的 module 行为 `github.com/yi-nology/code-review-service` | 文件核对 |

---

## 7. 待决策项(评审时拍板)

### D1 程序形态:扩展 git-ferry 还是独立程序?

| | 扩展 git-ferry(推荐) | 独立新程序 |
|---|---|---|
| 改写/门禁复用 | 直接复用 core/mirror | 需重写或依赖 core |
| MCP/CLI/UI/审计 | 全白拿 | 全部重造 |
| 代价 | 动一个已不小的产品 | 多一套基础设施 |

**推荐**:扩展 git-ferry。改写边界正则这类坑不值得踩两遍。

### D2 AI 介入深度?

**推荐**:语义判断层(§5)。机械变换层交给确定性代码。

### D3 推广范围?

本地共 10 个内部路径 go 仓:

```
code-review-service(已公开)  bianque       crawler        domain-manage
moyuoctopus    openclash-service            user-service  wechat
review-service-export
```

- **首期只做 `review-service` 跑通**(推荐),推广是后续独立事项。
- 若首期即做通用化,路径/配置/部署文件差异都要配置化,工作量差一个量级。

### D4 公开树独有提交如何回流?

公开树的 Gerrit 移植、release CI 等提交**不回流**到内部树(本需求定义为单向)。若将来需要双向,应另立需求——双向合并的冲突语义比路径改写复杂得多,不塞进本需求。

---

## 8. 里程碑

| 阶段 | 内容 | 出口条件 |
|---|---|---|
| **M1** | core 扩展:双仓通道模型 + 差异判定 + 计划生成(F1/F2) | 对 review-service 产出正确计划 |
| **M2** | 改写 + 门禁 + 格式修复(F3/F4) | A2/A3/A4 通过 |
| **M3** | 痕迹检测 + 移植 + 推送(F5/F6/F7) | A1/A5/A6/A7 通过 |
| **M4** | 预检 / 审计回滚 / Agent 接入(F8/F9/F10) | A8/A9 通过,CLI+MCP 可用 |

**M1–M2 是最小可用闭环**:能生成正确的计划并产出改写后可通过门禁的提交,此时人工已能完成推送。M3 起才自动化推送。

---

## 9. 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| 公开树与内部树历史分叉加深,匹配歧义增加 | 计划错误 | 歧义即 `hold`,不猜;定期人工确认基线 |
| 痕迹清理误伤(把正常文本当内网信息删) | 内容损坏 | 检测与清理分离;清理命中项全部进计划供审 |
| 强推覆盖公开树已有内容 | 不可逆 | 裸镜像留底 + F9 回滚;覆盖需显式确认 |
| `core/mirror` 若演进,双仓模式被绑定 | 维护成本 | 依赖 core 的**公开 API**(如 `ReadModulePath`),不依赖内部实现 |
| 依赖 `go-git-platform` / `oekit` 家族版本 | 构建失败 | 走真实发布版本,不引 `replace`(与仓内既有约束一致) |

---

## 10. 参考

- `git-ferry-core/mirror/gitobj.go:23` — 边界安全改写实现
- `git-ferry-core/mirror/gate.go:16` — 类型检查门禁
- `git-ferry-core/service/mirror_service.go` — 通道编排与执行
- `docs/superpowers/specs/2026-09-16-mirror-publishing-design.md` — 镜像中心前端设计(本需求不改其页面结构)
- `git-enjoye-top` 内部树 `AGENTS.md`「Repos: internal mirror and the public one」— 双树现状的权威描述
